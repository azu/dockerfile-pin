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

// TestParseDockerfileForCheck_CopyFrom covers the status and the line `check` reports
// for each form of COPY --from.
func TestParseDockerfileForCheck_CopyFrom(t *testing.T) {
	content := `FROM golang:1.22@sha256:golang111 AS builder
COPY --from=builder /app /app
COPY --from=0 /go/bin/tool /usr/local/bin/tool
COPY --from=nginx:1.27 /etc/nginx /etc/nginx
COPY --from=busybox:1.36@sha256:busybox222 /bin/busybox /bin/busybox
COPY --from=ghcr.io/myorg/tool:v1 /tool /tool
COPY --from=nginx:${NGINX_VERSION} /etc/nginx /etc/nginx.orig
COPY ./config /config
`
	dir := t.TempDir()
	path := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	results, err := parseDockerfileForCheck(path, true, []string{"ghcr.io/myorg/*"})
	if err != nil {
		t.Fatalf("parseDockerfileForCheck() error = %v", err)
	}

	want := []struct {
		image    string
		status   string
		line     int
		original string
	}{
		{"golang:1.22", "ok", 1, "FROM golang:1.22@sha256:golang111 AS builder"},
		{"builder", "skip", 2, "COPY --from=builder /app /app"},
		{"0", "skip", 3, "COPY --from=0 /go/bin/tool /usr/local/bin/tool"},
		{"nginx:1.27", "fail", 4, "COPY --from=nginx:1.27 /etc/nginx /etc/nginx"},
		{"busybox:1.36", "ok", 5, "COPY --from=busybox:1.36@sha256:busybox222 /bin/busybox /bin/busybox"},
		{"ghcr.io/myorg/tool:v1", "skip", 6, "COPY --from=ghcr.io/myorg/tool:v1 /tool /tool"},
		{"nginx:${NGINX_VERSION}", "skip", 7, "COPY --from=nginx:${NGINX_VERSION} /etc/nginx /etc/nginx.orig"},
	}
	if len(results) != len(want) {
		t.Fatalf("got %d results, want %d: %+v", len(results), len(want), results)
	}
	for i, w := range want {
		got := results[i]
		if got.Image != w.image || got.Status != w.status || got.Line != w.line || got.Original != w.original {
			t.Errorf("[%d] got %+v, want image=%q status=%q line=%d original=%q",
				i, got, w.image, w.status, w.line, w.original)
		}
	}
}
