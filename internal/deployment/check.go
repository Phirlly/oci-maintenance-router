// Package deployment verifies a PREVIEW installation without processing notices
// or sending notifications. Its result is not a baseline or delivery readiness.
package deployment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocistorage"
)

type Store interface {
	GetVersion(context.Context, string, string) (ocistorage.Object, error)
	Create(context.Context, string, []byte) (string, error)
	Get(context.Context, string) (ocistorage.Object, error)
}

type Result struct {
	Status              string `json:"status"`
	Code                string `json:"code,omitempty"`
	Field               string `json:"field,omitempty"`
	SendingEnabled      bool   `json:"sendingEnabled"`
	CheckAttemptID      string `json:"checkAttemptId,omitempty"`
	ConfigurationSHA256 string `json:"configurationSha256,omitempty"`
	ReleaseSHA256       string `json:"releaseSha256,omitempty"`
	RoutingGeneration   string `json:"routingGeneration,omitempty"`
}

func failed(code string) Result { return Result{Status: "not-ready", Code: code} }

func check(ctx context.Context, s Settings, store Store, attemptID string) Result {
	if ctx.Err() != nil {
		return failed("deadline")
	}
	catalogBytes, err := readPinned(ctx, store, "configuration/catalog.json", s.CatalogVersion, s.CatalogSHA256)
	if err != nil {
		return failed("catalog_read")
	}
	var catalog struct {
		Regions       map[string]string `json:"regions"`
		MySQLServices []string          `json:"mysqlServices"`
	}
	if decodeJSON(catalogBytes, &catalog) != nil {
		return failed("catalog_invalid")
	}
	configBytes, err := readPinned(ctx, store, "configuration/router.json", s.ConfigurationVersion, s.ConfigurationSHA256)
	if err != nil {
		return failed("configuration_read")
	}
	config, err := configuration.Decode(configBytes, configuration.Catalog{Regions: catalog.Regions, MySQLServices: catalog.MySQLServices})
	if err != nil {
		result := failed("configuration_invalid")
		var detail *configuration.Error
		if errors.As(err, &detail) {
			result.Field = detail.Field
		}
		return result
	}
	if config.TenancyID != s.TenancyID || config.DeploymentRegion != s.Region {
		return failed("configuration_identity")
	}
	// This legacy installation check cannot fulfil operational preparation or
	// diagnostic intent. Adoption must replace this gate with guarded coordination.
	if config.SchemaVersion != 1 {
		return failed("configuration_unsupported")
	}
	if config.Mode != "PREVIEW" || config.ActiveBaselineID != "" || len(config.Replay) != 0 {
		return failed("preview_required")
	}
	environment, err := config.EnvironmentID(configuration.LedgerLocation{Namespace: s.Namespace, Bucket: s.Bucket})
	if err != nil {
		return failed("storage_configuration")
	}
	generation, err := config.RoutingGeneration()
	if err != nil {
		return failed("configuration_invalid")
	}
	// A fresh attempt must prove its own write; retries reuse the same marker.
	marker, _ := json.Marshal([]string{"deployment-check-v2", environment, s.ConfigurationSHA256, s.CatalogSHA256, s.ReleaseSHA256, attemptID})
	name := "deployment-checks/" + digest(marker) + ".json"
	if ctx.Err() != nil {
		return failed("deadline")
	}
	_, err = store.Create(ctx, name, marker)
	if err != nil {
		var detail *ocistorage.Error
		if !errors.As(err, &detail) || (detail.Code != ocistorage.Conflict && !detail.WriteUncertain) {
			return failed("storage_write")
		}
		// A historical marker cannot override an explicit rejection of this
		// deployment check's write. Only conflicts or ambiguous outcomes may
		// recover by checking the exact saved bytes.
		if detail.Code == ocistorage.Service && detail.Status >= 400 && detail.Status < 500 {
			return failed("storage_write")
		}
	}
	saved, err := store.Get(ctx, name)
	if err != nil || !bytes.Equal(saved.Body, marker) {
		return failed("storage_verify")
	}
	if ctx.Err() != nil {
		return failed("deadline")
	}
	return Result{Status: "deployment-verified", ConfigurationSHA256: s.ConfigurationSHA256, ReleaseSHA256: s.ReleaseSHA256, RoutingGeneration: generation}
}

func readPinned(ctx context.Context, store Store, name, version, expected string) ([]byte, error) {
	obj, err := store.GetVersion(ctx, name, version)
	if err != nil {
		return nil, err
	}
	if len(obj.Body) > 1<<20 || digest(obj.Body) != expected {
		return nil, errors.New("pinned content mismatch")
	}
	return obj.Body, nil
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
