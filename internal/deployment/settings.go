package deployment

// Settings is deployment-owned bootstrap metadata, never invocation input.
type Settings struct {
	TenancyID            string `json:"tenancyId"`
	Region               string `json:"region"`
	Namespace            string `json:"namespace"`
	Bucket               string `json:"bucket"`
	ConfigurationVersion string `json:"configurationVersion"`
	CatalogVersion       string `json:"catalogVersion"`
	ConfigurationSHA256  string `json:"configurationSha256"`
	CatalogSHA256        string `json:"catalogSha256"`
	ReleaseSHA256        string `json:"releaseSha256"`
}
