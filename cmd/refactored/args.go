package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
)

// Args represents parsed command-line arguments as a key-value map
type Args map[string]string

// ParseArgs converts command-line arguments into a key-value map
// Handles both -key=value and -key formats
// For -key without value, sets value to empty string (can be checked with hasKey)
// Positional arguments are stored with keys _pos_0, _pos_1, etc.
func ParseArgs(args []string) Args {
	result := make(Args)
	posIndex := 0

	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			// Positional argument - store with index as key
			key := fmt.Sprintf("_pos_%d", posIndex)
			result[key] = arg
			posIndex++
			continue
		}

		// Remove leading dash(es)
		arg = strings.TrimLeft(arg, "-")

		// Check for key=value format
		if idx := strings.Index(arg, "="); idx != -1 {
			key := arg[:idx]
			value := arg[idx+1:]
			result[key] = value
		} else {
			// Just a key, no value
			result[arg] = ""
		}
	}

	return result
}

// GetString extracts a string value from args map with a default value
func (a Args) GetString(key string, defaultValue string) string {
	if val, ok := a[key]; ok && val != "" {
		return val
	}
	return defaultValue
}

// GetInt extracts an integer value from args map with a default value
func (a Args) GetInt(key string, defaultValue int) int {
	if val, ok := a[key]; ok && val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// RequireString extracts a required string value from args map, fatals if missing
func (a Args) RequireString(key string, usage string) string {
	val := a.GetString(key, "")
	if val == "" {
		log.Fatalf("Missing required flag: -%s\n%s", key, usage)
	}
	return val
}

// GetPositional extracts a positional argument by index
func (a Args) GetPositional(index int) string {
	key := fmt.Sprintf("_pos_%d", index)
	return a.GetString(key, "")
}
