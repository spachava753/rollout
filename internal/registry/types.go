package registry

// RegistryTask represents a single task entry in a registry dataset.
type RegistryTask struct {
	Name        string `json:"name"`
	GitURL      string `json:"git_url"`
	GitCommitID string `json:"git_commit_id,omitempty"` // empty = HEAD
	Path        string `json:"path,omitempty"`          // empty = repo root
}

// RegistryDataset represents a dataset defined in a registry.json file.
type RegistryDataset struct {
	Name        string         `json:"name"`
	Version     string         `json:"version"`
	Description string         `json:"description,omitempty"`
	Tasks       []RegistryTask `json:"tasks"`
}

// cloneKey uniquely identifies a git repository at a specific commit.
type cloneKey struct {
	GitURL      string
	GitCommitID string // empty means HEAD
}
