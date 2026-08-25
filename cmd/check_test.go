package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGitLabForCheck(t *testing.T) {
	content := `default:
  image: node:24@sha256:aaa111
test:
  image: alpine:3.20
  services:
    - $CI_REGISTRY_IMAGE:latest
    - name: ghcr.io/myorg/internal:v1@sha256:bbb222
`
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitlab-ci.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	results, err := parseGitLabForCheck(path, true, []string{"ghcr.io/myorg/*"})
	if err != nil {
		t.Fatalf("parseGitLabForCheck() error = %v", err)
	}

	want := []struct {
		image    string
		status   string
		original string
	}{
		{"node:24", "ok", "image: node:24@sha256:aaa111"},
		{"alpine:3.20", "fail", "image: alpine:3.20"},
		{"$CI_REGISTRY_IMAGE:latest", "skip", "- $CI_REGISTRY_IMAGE:latest"},
		{"ghcr.io/myorg/internal:v1", "skip", "name: ghcr.io/myorg/internal:v1@sha256:bbb222"},
	}
	if len(results) != len(want) {
		t.Fatalf("got %d results, want %d: %+v", len(results), len(want), results)
	}
	for i, w := range want {
		if results[i].Image != w.image {
			t.Errorf("[%d] Image = %q, want %q", i, results[i].Image, w.image)
		}
		if results[i].Status != w.status {
			t.Errorf("[%d] Status = %q, want %q", i, results[i].Status, w.status)
		}
		if results[i].Original != w.original {
			t.Errorf("[%d] Original = %q, want %q", i, results[i].Original, w.original)
		}
	}
}
