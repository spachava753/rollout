package modal

import (
	"strings"
	"testing"
)

func TestParseDockerfile(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantBase    string
		wantCmds    int
		wantErr     bool
		errContains string
	}{
		{
			name: "basic dockerfile",
			content: `
FROM ubuntu:22.04
RUN apt-get update
ENV MY_VAR=test
`,
			wantBase: "ubuntu:22.04",
			wantCmds: 2,
			wantErr:  false,
		},
		{
			name: "dockerfile with COPY",
			content: `
FROM python:3.10
COPY . /app
RUN pip install -r requirements.txt
`,
			wantErr:     true,
			errContains: "COPY and ADD instructions are not supported",
		},
		{
			name: "dockerfile with ADD",
			content: `
FROM alpine:latest
ADD https://example.com/file.tar.gz /tmp/
`,
			wantErr:     true,
			errContains: "COPY and ADD instructions are not supported",
		},
		{
			name: "dockerfile with line continuations",
			content: `
FROM node:18
RUN npm install \
    react \
    react-dom
`,
			wantBase: "node:18",
			wantCmds: 1,
			wantErr:  false,
		},
		{
			name: "missing FROM",
			content: `
RUN echo "hello"
`,
			wantErr:     true,
			errContains: "no FROM instruction found",
		},
		{
			name: "multiple FROM - uses last",
			content: `
FROM golang:1.21
RUN go version
FROM alpine:latest
`,
			wantBase: "alpine:latest",
			wantCmds: 1,
			wantErr:  false,
		},
		{
			name: "comments and empty lines",
			content: `
# This is a comment

FROM python:3.9

# Another comment
RUN python --version
`,
			wantBase: "python:3.9",
			wantCmds: 1,
			wantErr:  false,
		},
		{
			name: "case insensitive instructions",
			content: `
from node:20
run node -v
workdir /app
`,
			wantBase: "node:20",
			wantCmds: 2,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, cmds, err := parseDockerfile(tt.content)

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
					return
				}
				if base != tt.wantBase {
					t.Errorf("expected base %q, got %q", tt.wantBase, base)
				}
				if len(cmds) != tt.wantCmds {
					t.Errorf("expected %d commands, got %d", tt.wantCmds, len(cmds))
				}
			}
		})
	}
}
