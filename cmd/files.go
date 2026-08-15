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
	FileTypeActions
	FileTypeGitLab
)

func DetectFileType(path string) FileType {
	normalized := filepath.ToSlash(path)
	base := filepath.Base(path)
	lower := strings.ToLower(base)

	// GitHub Actions workflow files
	if strings.Contains(normalized, ".github/workflows/") &&
		(strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml")) {
		return FileTypeActions
	}

	// GitHub Actions action metadata files
	if lower == "action.yml" || lower == "action.yaml" {
		return FileTypeActions
	}

	// GitLab CI files. The suffix names the format, so a pipeline split into
	// parts keeps it wherever the parts are kept.
	if strings.HasSuffix(lower, ".gitlab-ci.yml") {
		return FileTypeGitLab
	}

	// The compose filenames are claimed before the GitLab layouts below, which
	// key on directory names that belong to no one in particular.
	if isComposeName(lower) {
		return FileTypeCompose
	}

	if isComponentTemplate(normalized) {
		return FileTypeGitLab
	}

	// GitLab CI files split out and pulled back in with `include: local:`.
	// The directory is a convention rather than something GitLab defines, but
	// `.gitlab` itself holds features of its own, so only `ci` counts.
	if strings.Contains(normalized, ".gitlab/ci/") &&
		(strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml")) {
		return FileTypeGitLab
	}

	// Compose files (any other YAML)
	if strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml") {
		return FileTypeCompose
	}

	return FileTypeDockerfile
}

// isComponentTemplate reports whether path is a CI component template. GitLab
// publishes a component from templates/<name>.yml or from
// templates/<name>/template.yml, and accepts no other layout.
// https://docs.gitlab.com/ci/components/
// isComposeName reports whether base is one of the compose filenames, which are
// the names the default search looks for.
func isComposeName(base string) bool {
	if base == "compose.yml" || base == "compose.yaml" {
		return true
	}
	return strings.HasPrefix(base, "docker-compose") &&
		(strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml"))
}

func isComponentTemplate(normalized string) bool {
	for _, pattern := range []string{"**/templates/*.yml", "**/templates/*/template.yml"} {
		if ok, err := doublestar.Match(pattern, normalized); err == nil && ok {
			return true
		}
	}
	return false
}

// defaultGlob is used when neither -f nor --glob is specified.
const defaultGlob = "**/{Dockerfile,Dockerfile.*,docker-compose*.yml,docker-compose*.yaml,compose.yml,compose.yaml,action.yml,action.yaml,.gitlab-ci.yml,.github/workflows/*.yml,.github/workflows/*.yaml}"

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
		return nil, fmt.Errorf("no Dockerfiles, compose files, GitHub Actions files, or GitLab CI files found")
	}
	return matches, nil
}

// skipDirs are directories skipped during glob fallback (outside git repos).
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
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
