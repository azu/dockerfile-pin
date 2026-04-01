package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azu/dockerfile-pin/internal/dockerfile"
)

func TestApplyDockerfile_UpdateExistingDigest(t *testing.T) {
	content := "FROM node:20.11.1@sha256:olddigest111\nFROM python:3.12-slim@sha256:olddigest222\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	instructions, err := dockerfile.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	pf := parsedFile{
		path:        path,
		fileType:    FileTypeDockerfile,
		dockerInsts: instructions,
		content:     []byte(content),
	}

	digestMap := map[string]string{
		"node:20.11.1":     "sha256:newdigest111",
		"python:3.12-slim": "sha256:newdigest222",
	}

	// Apply with write (not dry-run)
	applyDockerfile(pf, digestMap, false)

	result, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(result), "FROM node:20.11.1@sha256:newdigest111") {
		t.Errorf("expected node digest to be updated, got: %s", string(result))
	}
	if !strings.Contains(string(result), "FROM python:3.12-slim@sha256:newdigest222") {
		t.Errorf("expected python digest to be updated, got: %s", string(result))
	}
}
