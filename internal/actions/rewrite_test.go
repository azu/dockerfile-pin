package actions

import (
	"strings"
	"testing"
)

func TestRewriteFile_ContainerImage(t *testing.T) {
	content := `name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    container:
      image: node:24
    steps:
      - uses: actions/checkout@v4
`
	refs := []ActionsImageRef{
		{ImageRef: "node:24", RawRef: "node:24", Line: 7, HasPrefix: false},
	}
	digests := map[int]string{0: "sha256:aaa111"}
	result := RewriteFile(content, refs, digests)

	if !strings.Contains(result, "image: node:24@sha256:aaa111") {
		t.Errorf("expected pinned container image, got:\n%s", result)
	}
}

func TestRewriteFile_ServiceImage(t *testing.T) {
	content := `name: CI
on: push
jobs:
  test:
    services:
      db:
        image: postgres:18
`
	refs := []ActionsImageRef{
		{ImageRef: "postgres:18", RawRef: "postgres:18", Line: 7, HasPrefix: false},
	}
	digests := map[int]string{0: "sha256:bbb222"}
	result := RewriteFile(content, refs, digests)

	if !strings.Contains(result, "image: postgres:18@sha256:bbb222") {
		t.Errorf("expected pinned service image, got:\n%s", result)
	}
}

func TestRewriteFile_DockerPrefixStep(t *testing.T) {
	content := `name: CI
on: push
jobs:
  build:
    steps:
      - uses: docker://ghcr.io/foo/bar:latest
`
	refs := []ActionsImageRef{
		{ImageRef: "ghcr.io/foo/bar:latest", RawRef: "docker://ghcr.io/foo/bar:latest", Line: 6, HasPrefix: true},
	}
	digests := map[int]string{0: "sha256:ccc333"}
	result := RewriteFile(content, refs, digests)

	if !strings.Contains(result, "uses: docker://ghcr.io/foo/bar:latest@sha256:ccc333") {
		t.Errorf("expected pinned docker:// step, got:\n%s", result)
	}
}

func TestRewriteFile_ActionImage(t *testing.T) {
	content := `name: My Action
runs:
  using: docker
  image: docker://debian:stretch-slim
`
	refs := []ActionsImageRef{
		{ImageRef: "debian:stretch-slim", RawRef: "docker://debian:stretch-slim", Line: 4, HasPrefix: true},
	}
	digests := map[int]string{0: "sha256:ddd444"}
	result := RewriteFile(content, refs, digests)

	if !strings.Contains(result, "image: docker://debian:stretch-slim@sha256:ddd444") {
		t.Errorf("expected pinned action image, got:\n%s", result)
	}
}

func TestRewriteFile_UpdateExistingDigest(t *testing.T) {
	content := `name: CI
on: push
jobs:
  test:
    container:
      image: node:24@sha256:olddigest
`
	refs := []ActionsImageRef{
		{ImageRef: "node:24", RawRef: "node:24@sha256:olddigest", Line: 6, HasPrefix: false, Digest: "sha256:olddigest"},
	}
	digests := map[int]string{0: "sha256:newdigest"}
	result := RewriteFile(content, refs, digests)

	if !strings.Contains(result, "image: node:24@sha256:newdigest") {
		t.Errorf("expected updated digest, got:\n%s", result)
	}
	if strings.Contains(result, "olddigest") {
		t.Error("old digest should be replaced")
	}
}

func TestRewriteFile_UpdateDockerPrefixDigest(t *testing.T) {
	content := `name: CI
on: push
jobs:
  build:
    steps:
      - uses: docker://ghcr.io/foo/bar:latest@sha256:olddigest
`
	refs := []ActionsImageRef{
		{ImageRef: "ghcr.io/foo/bar:latest", RawRef: "docker://ghcr.io/foo/bar:latest@sha256:olddigest", Line: 6, HasPrefix: true, Digest: "sha256:olddigest"},
	}
	digests := map[int]string{0: "sha256:newdigest"}
	result := RewriteFile(content, refs, digests)

	if !strings.Contains(result, "uses: docker://ghcr.io/foo/bar:latest@sha256:newdigest") {
		t.Errorf("expected updated docker:// digest, got:\n%s", result)
	}
}

func TestRewriteFile_SkipWithoutDigest(t *testing.T) {
	content := `name: CI
on: push
jobs:
  test:
    container:
      image: node:24
`
	refs := []ActionsImageRef{
		{ImageRef: "node:24", RawRef: "node:24", Line: 6, HasPrefix: false},
	}
	// No digest in map for this ref
	digests := map[int]string{}
	result := RewriteFile(content, refs, digests)

	if result != content {
		t.Errorf("content should be unchanged when no digest provided, got:\n%s", result)
	}
}

func TestRewriteFile_SkipFlagged(t *testing.T) {
	content := `name: CI
on: push
jobs:
  test:
    container:
      image: node:24
`
	refs := []ActionsImageRef{
		{ImageRef: "node:24", RawRef: "node:24", Line: 6, HasPrefix: false, Skip: true, SkipReason: "test"},
	}
	digests := map[int]string{0: "sha256:aaa111"}
	result := RewriteFile(content, refs, digests)

	if result != content {
		t.Errorf("content should be unchanged for skipped refs, got:\n%s", result)
	}
}

func TestRewriteFile_Mixed(t *testing.T) {
	content := `name: CI
on: push
jobs:
  sample:
    container:
      image: node:24
    services:
      db:
        image: postgres:18
    steps:
      - uses: docker://ghcr.io/foo/bar:latest
      - uses: actions/checkout@v4
`
	refs := []ActionsImageRef{
		{ImageRef: "node:24", RawRef: "node:24", Line: 6, HasPrefix: false},
		{ImageRef: "postgres:18", RawRef: "postgres:18", Line: 9, HasPrefix: false},
		{ImageRef: "ghcr.io/foo/bar:latest", RawRef: "docker://ghcr.io/foo/bar:latest", Line: 11, HasPrefix: true},
	}
	digests := map[int]string{
		0: "sha256:aaa111",
		1: "sha256:bbb222",
		2: "sha256:ccc333",
	}
	result := RewriteFile(content, refs, digests)

	if !strings.Contains(result, "image: node:24@sha256:aaa111") {
		t.Error("expected pinned container image")
	}
	if !strings.Contains(result, "image: postgres:18@sha256:bbb222") {
		t.Error("expected pinned service image")
	}
	if !strings.Contains(result, "uses: docker://ghcr.io/foo/bar:latest@sha256:ccc333") {
		t.Error("expected pinned docker:// step")
	}
	if !strings.Contains(result, "uses: actions/checkout@v4") {
		t.Error("non-docker step should be unchanged")
	}
}
