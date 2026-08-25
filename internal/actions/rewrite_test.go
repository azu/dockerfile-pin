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

// A flow mapping writes several services on one line, and one image may be a
// prefix of another, so replacing by text alone would match inside the wrong one.
func TestRewriteFile_FlowMappingPrefixSharingServices(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "later image is a prefix of an earlier one",
			content: "jobs:\n  test:\n    services: {a: {image: postgres:18}, b: {image: postgres}}\n",
			want:    "    services: {a: {image: postgres:18@sha256:aaa111}, b: {image: postgres@sha256:bbb222}}",
		},
		{
			name:    "identical images",
			content: "jobs:\n  test:\n    services: {a: {image: postgres}, b: {image: postgres}}\n",
			want:    "    services: {a: {image: postgres@sha256:aaa111}, b: {image: postgres@sha256:bbb222}}",
		},
		{
			name:    "quoted images",
			content: "jobs:\n  test:\n    services: {a: {image: \"postgres:18\"}, b: {image: 'postgres'}}\n",
			want:    "    services: {a: {image: \"postgres:18@sha256:aaa111\"}, b: {image: 'postgres@sha256:bbb222'}}",
		},
		{
			name:    "earlier image already pinned",
			content: "jobs:\n  test:\n    services: {a: {image: postgres:18@sha256:old111}, b: {image: postgres}}\n",
			want:    "    services: {a: {image: postgres:18@sha256:aaa111}, b: {image: postgres@sha256:bbb222}}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs, err := Parse([]byte(tt.content))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(refs) != 2 {
				t.Fatalf("Parse() returned %d refs, want 2", len(refs))
			}
			result := RewriteFile(tt.content, refs, map[int]string{
				0: "sha256:aaa111",
				1: "sha256:bbb222",
			})

			if !strings.Contains(result, tt.want) {
				t.Errorf("got:\n%s\nwant line %q", result, tt.want)
			}
		})
	}
}

// A flow sequence of steps puts two docker:// references on one line.
func TestRewriteFile_FlowSequenceDockerSteps(t *testing.T) {
	content := "jobs:\n  test:\n    steps: [{uses: docker://postgres:18}, {uses: docker://postgres}]\n"
	refs, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("Parse() returned %d refs, want 2", len(refs))
	}
	result := RewriteFile(content, refs, map[int]string{
		0: "sha256:aaa111",
		1: "sha256:bbb222",
	})

	want := "    steps: [{uses: docker://postgres:18@sha256:aaa111}, {uses: docker://postgres@sha256:bbb222}]"
	if !strings.Contains(result, want) {
		t.Errorf("got:\n%s\nwant line %q", result, want)
	}
}

// A real digest is far longer than the image it is appended to, so pinning one
// image moves every image after it well past the column it was written at.
func TestRewriteFile_FlowMappingRealDigestLengths(t *testing.T) {
	content := "jobs:\n  test:\n    services: {a: {image: postgres:18}, b: {image: postgres}, c: {image: postgres:18-alpine}}\n"
	refs, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	digests := map[int]string{
		0: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		1: "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		2: "sha256:3333333333333333333333333333333333333333333333333333333333333333",
	}
	result := RewriteFile(content, refs, digests)

	want := "jobs:\n  test:\n    services: {a: {image: postgres:18@" + digests[0] +
		"}, b: {image: postgres@" + digests[1] +
		"}, c: {image: postgres:18-alpine@" + digests[2] + "}}\n"
	if result != want {
		t.Errorf("got:\n%s\nwant:\n%s", result, want)
	}
}
