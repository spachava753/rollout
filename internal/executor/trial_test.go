package executor

import "testing"

func TestSanitizeEnvName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "my-app",
			expected: "my-app",
		},
		{
			name:     "uppercase to lowercase",
			input:    "My-App",
			expected: "my-app",
		},
		{
			name:     "special chars to hyphens",
			input:    "my_app.name",
			expected: "my-app-name",
		},
		{
			name:     "consecutive special chars",
			input:    "my___app",
			expected: "my-app",
		},
		{
			name:     "leading/trailing special chars",
			input:    "_my-app_",
			expected: "my-app",
		},
		{
			name:     "long name truncated",
			input:    "terminal-bench-llm-inference-batching-scheduler-oracle-1-1734567890",
			expected: "terminal-bench-llm-inference-batching-scheduler-oracle-1-1734567",
		},
		{
			name:     "truncation removes trailing hyphen",
			input:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-b",
			expected: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeEnvName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeEnvName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
			if len(result) > maxAppNameLength {
				t.Errorf("sanitizeEnvName(%q) length %d exceeds max %d", tt.input, len(result), maxAppNameLength)
			}
		})
	}
}
