package cmd

import (
	"fmt"
	"os"
	"os/exec"
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

func isTargetFile(name string) bool {
	lower := strings.ToLower(name)
	if lower == "dockerfile" {
		return true
	}
	if strings.HasPrefix(lower, "dockerfile.") && !strings.HasSuffix(lower, ".go") && !strings.HasSuffix(lower, ".md") {
		return true
	}
	if strings.HasPrefix(lower, "docker-compose") && (strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml")) {
		return true
	}
	if lower == "compose.yml" || lower == "compose.yaml" {
		return true
	}
	return false
}

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
	// Default: use git ls-files to respect .gitignore
	files, err := findFilesWithGit()
	if err == nil && len(files) > 0 {
		return files, nil
	}
	return nil, fmt.Errorf("no Dockerfiles or compose files found")
}

func findFilesWithGit() ([]string, error) {
	out, err := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return nil, err
	}
	var matches []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isTargetFile(filepath.Base(line)) {
			matches = append(matches, line)
		}
	}
	return matches, nil
}
