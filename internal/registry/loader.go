package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// LoadFromPath loads a registry.json from a local filesystem path.
func LoadFromPath(path string) ([]RegistryDataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading registry file: %w", err)
	}

	var datasets []RegistryDataset
	if err := json.Unmarshal(data, &datasets); err != nil {
		return nil, fmt.Errorf("parsing registry JSON: %w", err)
	}

	return datasets, nil
}

// LoadFromURL loads a registry.json from a remote URL.
func LoadFromURL(ctx context.Context, url string) ([]RegistryDataset, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching registry: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var datasets []RegistryDataset
	if err := json.Unmarshal(data, &datasets); err != nil {
		return nil, fmt.Errorf("parsing registry JSON: %w", err)
	}

	return datasets, nil
}

// FindDataset searches for a dataset by name and version in a list of registry datasets.
// If version is empty, returns the first dataset with the matching name.
func FindDataset(datasets []RegistryDataset, name, version string) (*RegistryDataset, error) {
	for i := range datasets {
		if datasets[i].Name == name {
			if version == "" || datasets[i].Version == version {
				return &datasets[i], nil
			}
		}
	}

	if version != "" {
		return nil, fmt.Errorf("dataset %q version %q not found in registry", name, version)
	}
	return nil, fmt.Errorf("dataset %q not found in registry", name)
}
