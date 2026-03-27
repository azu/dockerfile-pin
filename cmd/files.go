package cmd

import (
	"fmt"
	"os"

	"github.com/bmatcuk/doublestar/v4"
)

func FindFiles(filePath string, globPattern string) ([]string, error) {
	if filePath != "" {
		if _, err := os.Stat(filePath); err != nil {
			return nil, fmt.Errorf("file not found: %s", filePath)
		}
		return []string{filePath}, nil
	}
	if globPattern != "" {
		matches, err := doublestar.FilepathGlob(globPattern)
		if err != nil {
			return nil, fmt.Errorf("invalid glob pattern: %w", err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no files matched pattern: %s", globPattern)
		}
		return matches, nil
	}
	defaultPath := "Dockerfile"
	if _, err := os.Stat(defaultPath); err != nil {
		return nil, fmt.Errorf("no Dockerfile found in current directory")
	}
	return []string{defaultPath}, nil
}
