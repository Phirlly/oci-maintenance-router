package ocisecrets

import (
	"encoding/base64"
	"encoding/json"
	"slices"
	"unicode/utf8"
)

// Application content budget; not a claim about OCI's encoded bundle accounting.
const secretLimit = 32 << 10

func decodeBundle(body []byte, secretID string, version int64) (Bundle, error) {
	invalid := &Error{Code: InvalidResponse, Status: 200}
	if !utf8.Valid(body) {
		return Bundle{}, invalid
	}
	fields, ok := bundleObject(body)
	if !ok {
		return Bundle{}, invalid
	}
	var id string
	var returnedVersion int64
	if json.Unmarshal(fields["secretId"], &id) != nil || id != secretID ||
		json.Unmarshal(fields["versionNumber"], &returnedVersion) != nil || returnedVersion <= 0 ||
		(version != 0 && returnedVersion != version) {
		return Bundle{}, invalid
	}
	content, ok := bundleObject(fields["secretBundleContent"])
	if !ok {
		return Bundle{}, invalid
	}
	var kind, encoded string
	if json.Unmarshal(content["contentType"], &kind) != nil || kind != "BASE64" ||
		json.Unmarshal(content["content"], &encoded) != nil {
		return Bundle{}, invalid
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(decoded) == 0 {
		return Bundle{}, invalid
	}
	if len(decoded) > secretLimit {
		return Bundle{}, &Error{Code: TooLarge, Status: 200}
	}
	return Bundle{version: returnedVersion, content: func() []byte { return slices.Clone(decoded) }}, nil
}
