// Package deployment binds managed identity and pinned configuration to guarded
// Function operations. Legacy schema1 retains its nonsending installation check.
package deployment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"oci-maintenance-router/internal/ocistorage"
)

type Store interface {
	GetVersion(context.Context, string, string) (ocistorage.Object, error)
	Create(context.Context, string, []byte) (string, error)
	Get(context.Context, string) (ocistorage.Object, error)
}

type Result struct {
	Status              string                `json:"status"`
	Code                string                `json:"code,omitempty"`
	Field               string                `json:"field,omitempty"`
	SendingEnabled      bool                  `json:"sendingEnabled"`
	CheckAttemptID      string                `json:"checkAttemptId,omitempty"`
	ConfigurationSHA256 string                `json:"configurationSha256,omitempty"`
	ReleaseSHA256       string                `json:"releaseSha256,omitempty"`
	RoutingGeneration   string                `json:"routingGeneration,omitempty"`
	pinnedRead          *pinnedReadDiagnostic // Logging only; never part of the response JSON.
}

func failed(code string) Result { return Result{Status: "not-ready", Code: code} }

func check(ctx context.Context, s Settings, store Store, attemptID string) Result {
	p, result := loadPinned(ctx, s, store)
	if result.Status != "" {
		return result
	}
	return checkLegacy(ctx, s, store, attemptID, p)
}

func checkLegacy(ctx context.Context, s Settings, store Store, attemptID string, p pinnedInputs) Result {
	// This legacy installation check cannot fulfil operational preparation or
	// diagnostic intent. Adoption must replace this gate with guarded coordination.
	if p.config.SchemaVersion != 1 {
		return failed("configuration_unsupported")
	}
	if p.config.Mode != "PREVIEW" || p.config.ActiveBaselineID != "" || len(p.config.Replay) != 0 {
		return failed("preview_required")
	}
	// A fresh attempt must prove its own write; retries reuse the same marker.
	marker, _ := json.Marshal([]string{"deployment-check-v2", p.environment, s.ConfigurationSHA256, s.CatalogSHA256, s.ReleaseSHA256, attemptID})
	name := "deployment-checks/" + digest(marker) + ".json"
	if ctx.Err() != nil {
		return failed("deadline")
	}
	_, err := store.Create(ctx, name, marker)
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
	return Result{Status: "deployment-verified", ConfigurationSHA256: s.ConfigurationSHA256, ReleaseSHA256: s.ReleaseSHA256, RoutingGeneration: p.generation}
}

func readPinned(ctx context.Context, store Store, name, version, expected string) ([]byte, error) {
	obj, err := store.GetVersion(ctx, name, version)
	if err != nil {
		return nil, err
	}
	if len(obj.Body) > 1<<20 {
		return nil, &ocistorage.Error{Code: ocistorage.TooLarge}
	}
	if digest(obj.Body) != expected {
		return nil, errPinnedContentMismatch
	}
	return obj.Body, nil
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
