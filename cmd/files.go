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

// defaultGlob is used when neither -f nor --glob is specified.
const defaultGlob = "**/{Dockerfile,Dockerfile.*,dockerfile_*.tmpl,docker-compose*.yml,docker-compose*.yaml,compose.yml,compose.yaml}"

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
	// Default: use git ls-files filtered by defaultGlob to respect .gitignore
	files, err := findFilesWithGit()
	if err == nil && len(files) > 0 {
		return files, nil
	}
	// Fallback: glob without git, skip common dependency dirs
	matches, err := findFilesWithGlob(defaultGlob)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no Dockerfiles or compose files found")
	}
	return matches, nil
}

// skipDirs are directories skipped during glob fallback (outside git repos).
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	".git":         true,
}

func findFilesWithGlob(pattern string) ([]string, error) {
	var matches []string
	err := doublestar.GlobWalk(os.DirFS("."), pattern, func(path string, d os.DirEntry) error {
		matches = append(matches, path)
		return nil
	}, doublestar.WithFilesOnly(), doublestar.WithFailOnPatternNotExist())
	if err != nil {
		return nil, err
	}
	// Filter out paths under skip dirs
	var filtered []string
	for _, p := range matches {
		if !inSkipDir(p) {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

func inSkipDir(path string) bool {
	for part := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if skipDirs[part] {
			return true
		}
	}
	return false
}

func findFilesWithGit() ([]string, error) {
	out, err := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return nil, err
	}
	var matches []string
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		matched, err := doublestar.PathMatch(defaultGlob, line)
		if err != nil {
			continue
		}
		if matched {
			matches = append(matches, line)
		}
	}
	return matches, nil
}
