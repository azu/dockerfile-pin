package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type FileType int

const (
	FileTypeDockerfile FileType = iota
	FileTypeCompose
)

func DetectFileType(path string) FileType {
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	if strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml") {
		return FileTypeCompose
	}
	return FileTypeDockerfile
}

// defaultGlob is used when neither -f nor --glob is specified.
const defaultGlob = "**/{Dockerfile,Dockerfile.*,dockerfile_*.tmpl,docker-compose.yml,docker-compose.yaml,compose.yml,compose.yaml}"

func FindFiles(filePath string, globPattern string) ([]string, error) {
	if filePath != "" {
		if _, err := os.Stat(filePath); err != nil {
			return nil, fmt.Errorf("file not found: %s", filePath)
		}
		return []string{filePath}, nil
	}
	if globPattern == "" {
		globPattern = defaultGlob
	}
	matches, err := doublestar.FilepathGlob(globPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid glob pattern: %w", err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no files matched pattern: %s", globPattern)
	}
	return matches, nil
}
