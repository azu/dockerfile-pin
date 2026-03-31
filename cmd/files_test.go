package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestFindFiles_SingleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(path, []byte("FROM node:20"), 0644); err != nil {
		t.Fatal(err)
	}
	files, err := FindFiles(path, "")
	if err != nil {
		t.Fatalf("FindFiles() error = %v", err)
	}
	if len(files) != 1 || files[0] != path {
		t.Errorf("FindFiles() = %v, want [%s]", files, path)
	}
}

func TestFindFiles_GlobPattern(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "services", "api")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(dir, "Dockerfile"),
		filepath.Join(sub, "Dockerfile"),
		filepath.Join(sub, "Dockerfile.dev"),
	} {
		if err := os.WriteFile(p, []byte("FROM node:20"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := FindFiles("", filepath.Join(dir, "**", "Dockerfile*"))
	if err != nil {
		t.Fatalf("FindFiles() error = %v", err)
	}
	sort.Strings(files)
	if len(files) != 3 {
		t.Errorf("FindFiles() returned %d files, want 3: %v", len(files), files)
	}
}

func TestFindFiles_DefaultGlob(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "services", "api")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	targets := []string{
		filepath.Join(dir, "Dockerfile"),
		filepath.Join(sub, "Dockerfile"),
		filepath.Join(dir, "docker-compose.yml"),
	}
	for _, p := range targets {
		if err := os.WriteFile(p, []byte("FROM node:20"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()
	files, err := FindFiles("", "")
	if err != nil {
		t.Fatalf("FindFiles() error = %v", err)
	}
	if len(files) != 3 {
		t.Errorf("FindFiles() returned %d files, want 3: %v", len(files), files)
	}
}

func TestFindFiles_DefaultGlob_MatchesExpectedPatterns(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "docker")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	// Files that should match the default glob
	shouldMatch := []string{
		filepath.Join(dir, "Dockerfile"),
		filepath.Join(sub, "Dockerfile.dev"),
		filepath.Join(dir, "docker-compose.yml"),
		filepath.Join(dir, "docker-compose.yaml"),
		filepath.Join(dir, "compose.yml"),
		filepath.Join(dir, "compose.yaml"),
	}
	// Files that should NOT match
	shouldNotMatch := []string{
		filepath.Join(dir, "README.md"),
		filepath.Join(dir, "main.go"),
	}
	for _, p := range append(shouldMatch, shouldNotMatch...) {
		if err := os.WriteFile(p, []byte("FROM node:20"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()
	files, err := FindFiles("", "")
	if err != nil {
		t.Fatalf("FindFiles() error = %v", err)
	}
	if len(files) != len(shouldMatch) {
		t.Errorf("FindFiles() returned %d files, want %d: %v", len(files), len(shouldMatch), files)
	}
}
