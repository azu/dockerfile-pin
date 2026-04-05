package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azu/dockerfile-pin/internal/actions"
	"github.com/azu/dockerfile-pin/internal/compose"
	"github.com/azu/dockerfile-pin/internal/dockerfile"
	"github.com/azu/dockerfile-pin/internal/resolver"
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

// TestResolveParallel_ShortDigestNoPanic verifies that resolveParallel does not panic
// when the resolved digest is shorter than 19 characters.
func TestResolveParallel_ShortDigestNoPanic(t *testing.T) {
	// A very short digest (fewer than 19 chars) must not cause an index-out-of-range panic.
	short := &resolver.MockResolver{
		Digests: map[string]string{
			"tiny:1": "sha256:ab",  // only 10 chars — well under 19
			"tiny:2": "",           // zero-length edge case
		},
	}
	ctx := context.Background()
	// Should complete without panicking.
	results := resolveParallel(ctx, short, []string{"tiny:1", "tiny:2"})
	if results["tiny:1"] != "sha256:ab" {
		t.Errorf("expected sha256:ab, got %q", results["tiny:1"])
	}
	// Empty digest is still stored (no error from the mock).
	if _, ok := results["tiny:2"]; !ok {
		t.Error("expected empty digest to be stored in results")
	}
}

// TestWriteFilePreservingPerms verifies that writeFilePreservingPerms keeps
// the original file permissions intact after writing.
func TestWriteFilePreservingPerms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")

	for _, mode := range []os.FileMode{0600, 0640, 0755} {
		if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
			t.Fatal(err)
		}
		// Explicitly set the desired permissions (WriteFile only applies mode on create).
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if err := writeFilePreservingPerms(path, []byte("updated")); err != nil {
			t.Fatalf("writeFilePreservingPerms() error = %v", err)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != mode {
			t.Errorf("mode %04o: after write got %04o, want %04o", mode, got, mode)
		}
		content, _ := os.ReadFile(path)
		if string(content) != "updated" {
			t.Errorf("content mismatch: got %q", string(content))
		}
	}
}

// TestApplyDockerfile_PreservesFilePermissions verifies that applyDockerfile
// does not overwrite the original file permissions with a hardcoded value.
func TestApplyDockerfile_PreservesFilePermissions(t *testing.T) {
	content := "FROM golang:1.22\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
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
	applyDockerfile(pf, map[string]string{"golang:1.22": "sha256:abc"}, false, false)

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0600 {
		t.Errorf("applyDockerfile changed permissions: got %04o, want 0600", got)
	}
}

// TestApplyActions_PreservesFilePermissions verifies that applyActions
// does not overwrite the original file permissions.
func TestApplyActions_PreservesFilePermissions(t *testing.T) {
	content := `runs:
  using: docker
  image: docker://node:20
`
	dir := t.TempDir()
	path := filepath.Join(dir, "action.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}

	refs, err := actions.Parse([]byte(content))
	if err != nil {
		t.Fatalf("actions.Parse() error = %v", err)
	}

	pf := parsedFile{
		path:        path,
		fileType:    FileTypeActions,
		actionsRefs: refs,
		content:     []byte(content),
	}
	applyActions(pf, map[string]string{"node:20": "sha256:abc"}, false, false)

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0600 {
		t.Errorf("applyActions changed permissions: got %04o, want 0600", got)
	}
}

// TestApplyCompose_PreservesFilePermissions verifies that applyCompose
// does not overwrite the original file permissions.
func TestApplyCompose_PreservesFilePermissions(t *testing.T) {
	content := `services:
  app:
    image: node:20
`
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}

	refs, err := compose.Parse([]byte(content))
	if err != nil {
		t.Fatalf("compose.Parse() error = %v", err)
	}

	pf := parsedFile{
		path:        path,
		fileType:    FileTypeCompose,
		composeRefs: refs,
		content:     []byte(content),
	}
	applyCompose(pf, map[string]string{"node:20": "sha256:abc"}, false, false)

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0600 {
		t.Errorf("applyCompose changed permissions: got %04o, want 0600", got)
	}
}
