package modal

import (
	"errors"
	"strings"
	"testing"
)

// mockConfigReader is a test double for ConfigReader.
type mockConfigReader struct {
	output []byte
	err    error
}

func (m *mockConfigReader) ReadConfig() ([]byte, error) {
	return m.output, m.err
}

func ptr(s string) *string {
	return &s
}

func TestCheckImageBuilderVersion(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		readErr     error
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid version",
			output:  `{"image_builder_version": "2025.06"}`,
			wantErr: false,
		},
		{
			name:    "newer version",
			output:  `{"image_builder_version": "2025.12"}`,
			wantErr: false,
		},
		{
			name:        "version not set - null",
			output:      `{"image_builder_version": null}`,
			wantErr:     true,
			errContains: "image_builder_version is not set",
		},
		{
			name:        "version not set - empty string",
			output:      `{"image_builder_version": ""}`,
			wantErr:     true,
			errContains: "image_builder_version is not set",
		},
		{
			name:        "version too old",
			output:      `{"image_builder_version": "2024.10"}`,
			wantErr:     true,
			errContains: "is too old",
		},
		{
			name:        "cli error",
			readErr:     errors.New("modal CLI not found"),
			wantErr:     true,
			errContains: "failed to get modal config",
		},
		{
			name:        "invalid json",
			output:      `not valid json`,
			wantErr:     true,
			errContains: "failed to parse modal config",
		},
		{
			name:    "missing field defaults to null",
			output:  `{}`,
			wantErr: true,
			errContains: "image_builder_version is not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &mockConfigReader{
				output: []byte(tt.output),
				err:    tt.readErr,
			}

			err := checkImageBuilderVersionWith(reader)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
