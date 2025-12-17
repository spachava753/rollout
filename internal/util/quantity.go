package util

import (
	"fmt"
	"strings"
)

// ParseMemory converts a memory string (e.g., "2G", "512M") to MiB.
// If the string is empty, it returns 0.
func ParseMemory(memory string) (int, error) {
	memory = strings.TrimSpace(memory)
	if memory == "" {
		return 0, nil
	}

	var value float64
	var unit string

	// Try to parse number and unit
	n, err := fmt.Sscanf(memory, "%f%s", &value, &unit)
	
	if err != nil && n == 0 {
		return 0, fmt.Errorf("invalid memory value: %s", memory)
	}

	if n == 1 {
		// No unit found, assuming bytes (as per K8s/Docker convention often, but Modal impl assumed bytes too)
		return int(value / (1024 * 1024)), nil
	}

	unit = strings.ToUpper(strings.TrimSpace(unit))
	switch unit {
	case "B":
		return int(value / (1024 * 1024)), nil
	case "K", "KB", "KI", "KIB":
		return int(value / 1024), nil
	case "M", "MB", "MI", "MIB":
		return int(value), nil
	case "G", "GB", "GI", "GIB":
		return int(value * 1024), nil
	case "T", "TB", "TI", "TIB":
		return int(value * 1024 * 1024), nil
	default:
		return 0, fmt.Errorf("unknown memory unit: %s", unit)
	}
}
