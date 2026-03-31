package cmd

import (
	"os"
	"os/exec"
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

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s", args, out)
		}
	}
}

func gitAdd(t *testing.T, dir string, files ...string) {
	t.Helper()
	args := append([]string{"add"}, files...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %s", out)
	}
}

func TestFindFiles_DefaultRecursive(t *testing.T) {
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
	initGitRepo(t, dir)
	gitAdd(t, dir, ".")
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

func TestFindFiles_RespectsGitignore(t *testing.T) {
	dir := t.TempDir()
	ignored := filepath.Join(dir, "node_modules", "pkg")
	if err := os.MkdirAll(ignored, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM node:20"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ignored, "Dockerfile"), []byte("FROM node:20"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, dir)
	gitAdd(t, dir, ".")
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
		t.Errorf("FindFiles() returned %d files, want 1 (node_modules should be excluded): %v", len(files), files)
	}
}
