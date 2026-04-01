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

	applyDockerfile(pf, digestMap, false, true)

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

func TestApplyDockerfile_SkipExistingDigestWithoutUpdate(t *testing.T) {
	content := "FROM node:20.11.1@sha256:olddigest111\nFROM golang:1.22\n"
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
		"node:20.11.1": "sha256:newdigest111",
		"golang:1.22":  "sha256:ccc333",
	}

	applyDockerfile(pf, digestMap, false, false)

	result, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Without --update, existing digest should NOT be changed
	if !strings.Contains(string(result), "FROM node:20.11.1@sha256:olddigest111") {
		t.Errorf("expected node digest to be preserved, got: %s", string(result))
	}
	// Unpinned image should still be pinned
	if !strings.Contains(string(result), "FROM golang:1.22@sha256:ccc333") {
		t.Errorf("expected golang to be pinned, got: %s", string(result))
	}
}
