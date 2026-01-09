package apple

// ProviderConfig holds Apple Container-specific configuration.
type ProviderConfig struct {
	// RuntimeUser overrides the auto-detected UID for exec operations.
	RuntimeUser string
	// RuntimeGroup overrides the auto-detected GID for exec operations.
	RuntimeGroup string
}

// ParseProviderConfig extracts Apple Container-specific config from the generic config map.
func ParseProviderConfig(config map[string]any) ProviderConfig {
	pc := ProviderConfig{}
	if config == nil {
		return pc
	}
	if v, ok := config["runtime_user"].(string); ok {
		pc.RuntimeUser = v
	}
	if v, ok := config["runtime_group"].(string); ok {
		pc.RuntimeGroup = v
	}
	return pc
}
