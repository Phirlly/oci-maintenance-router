package deployment

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"unicode/utf8"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocistorage"
)

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func parseSettings(raw string) (Settings, error) {
	var s Settings
	if len(raw) > 4096 || decodeJSON([]byte(raw), &s) != nil {
		return s, errors.New("invalid bootstrap")
	}
	for _, v := range []string{s.TenancyID, s.Region, s.Namespace, s.Bucket, s.ConfigurationVersion, s.CatalogVersion} {
		if v == "" || len(v) > 1024 || !utf8.ValidString(v) {
			return s, errors.New("invalid bootstrap field")
		}
	}
	for _, v := range []string{s.ConfigurationSHA256, s.CatalogSHA256, s.ReleaseSHA256} {
		if !hashPattern.MatchString(v) {
			return s, errors.New("invalid bootstrap digest")
		}
	}
	return s, nil
}

func decodeJSON(data []byte, into any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(into); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func connect(s Settings) (Store, error) {
	return connectPrincipal(s, os.Getenv, func() (common.ConfigurationProvider, error) {
		return auth.ResourcePrincipalConfigurationProvider()
	})
}

func connectPrincipal(s Settings, getenv func(string) string, principal func() (common.ConfigurationProvider, error)) (Store, error) {
	// Use only the managed file-based RP2.2 identity. No user profile fallback,
	// alternate resource-principal protocol or invocation-provided credentials.
	if getenv(auth.ResourcePrincipalVersionEnvVar) != "2.2" ||
		!filepath.IsAbs(getenv(auth.ResourcePrincipalRPSTEnvVar)) ||
		!filepath.IsAbs(getenv(auth.ResourcePrincipalPrivatePEMEnvVar)) {
		return nil, errors.New("managed identity required")
	}
	p, err := principal()
	if err != nil {
		return nil, err
	}
	tenant, err := p.TenancyOCID()
	if err != nil || tenant != s.TenancyID {
		return nil, errors.New("identity tenancy mismatch")
	}
	region, err := p.Region()
	if err != nil || region != s.Region {
		return nil, errors.New("identity region mismatch")
	}
	client, err := ocistorage.New(s.Region, configuration.LedgerLocation{Namespace: s.Namespace, Bucket: s.Bucket}, p)
	if err != nil {
		return nil, err
	}
	return client, nil
}
