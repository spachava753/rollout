package models

// Agent represents an agent definition from job.yaml.
type Agent struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Install     string            `yaml:"install,omitempty" json:"install,omitempty"`
	Execute     string            `yaml:"execute,omitempty" json:"execute,omitempty"`
	Env         map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
}

// IsOracle returns true if this is the special oracle agent.
func (a Agent) IsOracle() bool {
	return a.Name == "oracle"
}
