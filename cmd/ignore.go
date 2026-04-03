package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

// Config represents .dockerfile-pin.yaml configuration.
type Config struct {
	IgnoreImages []string `yaml:"ignore-images"`
}

// configFileNames is the list of config file names to search for.
var configFileNames = []string{".dockerfile-pin.yaml", ".dockerfile-pin.yml"}

// LoadConfig reads .dockerfile-pin.yaml or .dockerfile-pin.yml from the current directory.
// Returns empty Config if neither file exists.
func LoadConfig() (Config, error) {
	for _, name := range configFileNames {
		data, err := os.ReadFile(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return Config{}, fmt.Errorf("reading %s: %w", name, err)
		}
		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("parsing %s: %w", name, err)
		}
		return cfg, nil
	}
	return Config{}, nil
}

// MergeIgnorePatterns merges config file patterns with CLI flag patterns.
// CLI patterns are appended after config patterns so they take precedence under last-match-wins.
func MergeIgnorePatterns(configPatterns, cliPatterns []string) []string {
	result := make([]string, 0, len(configPatterns)+len(cliPatterns))
	result = append(result, configPatterns...)
	result = append(result, cliPatterns...)
	return result
}

// ValidatePatterns checks that all patterns are valid glob patterns.
func ValidatePatterns(patterns []string) error {
	for _, raw := range patterns {
		pattern := strings.TrimPrefix(raw, "!")
		if _, err := doublestar.Match(pattern, ""); err != nil {
			return fmt.Errorf("invalid ignore pattern %q: %w", raw, err)
		}
	}
	return nil
}

// IsIgnored checks if imageRef matches any ignore pattern using glob matching.
// Supports negation patterns prefixed with "!".
// Evaluation uses last-match-wins semantics (like .gitignore).
func IsIgnored(imageRef string, patterns []string) bool {
	ignored := false
	for _, raw := range patterns {
		negated := false
		pattern := raw
		if strings.HasPrefix(pattern, "!") {
			negated = true
			pattern = pattern[1:]
		}
		matched, err := doublestar.Match(pattern, imageRef)
		if err != nil {
			continue
		}
		if matched {
			ignored = !negated
		}
	}
	return ignored
}
