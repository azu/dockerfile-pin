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

func TestFindFiles_DefaultFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM node:20"), 0644); err != nil {
		t.Fatal(err)
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
	if len(files) != 1 {
		t.Errorf("FindFiles() = %v, want 1 file", files)
	}
}
