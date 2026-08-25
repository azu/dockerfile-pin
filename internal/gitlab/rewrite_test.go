package gitlab

import (
	"strings"
	"testing"
)

func TestRewriteFile_ScalarImage(t *testing.T) {
	content := `build:
  image: node:24
  script:
    - npm ci
`
	refs, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result := RewriteFile(content, refs, map[int]string{0: "sha256:aaa111"})

	if !strings.Contains(result, "image: node:24@sha256:aaa111") {
		t.Errorf("image line was not pinned:\n%s", result)
	}
	if !strings.Contains(result, "    - npm ci") {
		t.Errorf("unrelated lines were not preserved:\n%s", result)
	}
}

func TestRewriteFile_MappingImageAndServices(t *testing.T) {
	content := `# pipeline for the api
default:
  image:
    name: ruby:3.4
    entrypoint: [""]
test:
  services:
    - postgres:18
    - name: redis:7
      alias: cache
`
	refs, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result := RewriteFile(content, refs, map[int]string{
		0: "sha256:aaa111",
		1: "sha256:bbb222",
		2: "sha256:ccc333",
	})

	for _, want := range []string{
		"    name: ruby:3.4@sha256:aaa111",
		"    - postgres:18@sha256:bbb222",
		"    - name: redis:7@sha256:ccc333",
		"# pipeline for the api",
		`    entrypoint: [""]`,
		"      alias: cache",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("missing %q in:\n%s", want, result)
		}
	}
}

func TestRewriteFile_ComponentSpecHeader(t *testing.T) {
	content := `spec:
  inputs:
    stage:
      default: test
---
build:
  image: node:24
`
	refs, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result := RewriteFile(content, refs, map[int]string{0: "sha256:aaa111"})

	if !strings.Contains(result, "image: node:24@sha256:aaa111") {
		t.Errorf("image in the second document was not pinned:\n%s", result)
	}
	if !strings.Contains(result, "      default: test") {
		t.Errorf("the header document was not preserved:\n%s", result)
	}
}

func TestRewriteFile_SkippedRefUntouched(t *testing.T) {
	content := `build:
  image: $CI_REGISTRY_IMAGE:latest
`
	refs, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result := RewriteFile(content, refs, map[int]string{0: "sha256:aaa111"})

	if result != content {
		t.Errorf("skipped ref was rewritten:\n%s", result)
	}
}

func TestRewriteFile_AlreadyPinnedIsReplaced(t *testing.T) {
	content := `build:
  image: node:24@sha256:old111
`
	refs, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result := RewriteFile(content, refs, map[int]string{0: "sha256:new222"})

	if !strings.Contains(result, "image: node:24@sha256:new222") {
		t.Errorf("digest was not replaced:\n%s", result)
	}
	if strings.Contains(result, "sha256:old111") {
		t.Errorf("old digest remains:\n%s", result)
	}
}

// A flow sequence puts several references on one line, so the rewrite has to
// replace more than once within it.
func TestRewriteFile_FlowSequenceServices(t *testing.T) {
	content := `test:
  services: [postgres:18, redis:7]
`
	refs, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result := RewriteFile(content, refs, map[int]string{
		0: "sha256:aaa111",
		1: "sha256:bbb222",
	})

	if !strings.Contains(result, "[postgres:18@sha256:aaa111, redis:7@sha256:bbb222]") {
		t.Errorf("flow sequence entries were not both pinned:\n%s", result)
	}
}

// One entry of a flow sequence can be a prefix of another, so replacing by text
// alone would match inside the entry that was pinned first.
func TestRewriteFile_FlowSequencePrefixSharingEntries(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "later entry is a prefix of an earlier one",
			content: "test:\n  services: [postgres:18, postgres]\n",
			want:    "  services: [postgres:18@sha256:aaa111, postgres@sha256:bbb222]",
		},
		{
			name:    "earlier entry is a prefix of a later one",
			content: "test:\n  services: [postgres, postgres:18]\n",
			want:    "  services: [postgres@sha256:aaa111, postgres:18@sha256:bbb222]",
		},
		{
			name:    "identical entries",
			content: "test:\n  services: [postgres, postgres]\n",
			want:    "  services: [postgres@sha256:aaa111, postgres@sha256:bbb222]",
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

// A flow mapping puts a service's name next to another entry on the same line.
func TestRewriteFile_FlowMappingServiceName(t *testing.T) {
	content := "test:\n  services: [redis, {name: redis:7, alias: cache}]\n"
	refs, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result := RewriteFile(content, refs, map[int]string{
		0: "sha256:aaa111",
		1: "sha256:bbb222",
	})

	want := "  services: [redis@sha256:aaa111, {name: redis:7@sha256:bbb222, alias: cache}]"
	if !strings.Contains(result, want) {
		t.Errorf("got:\n%s\nwant line %q", result, want)
	}
}

// A quoted scalar starts one column after the node itself, so a reference that is
// quoted has to be found from its column rather than spliced at it.
func TestRewriteFile_FlowSequenceQuotedEntries(t *testing.T) {
	content := "test:\n  services: [\"postgres:18\", 'postgres']\n"
	refs, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result := RewriteFile(content, refs, map[int]string{
		0: "sha256:aaa111",
		1: "sha256:bbb222",
	})

	want := `  services: ["postgres:18@sha256:aaa111", 'postgres@sha256:bbb222']`
	if !strings.Contains(result, want) {
		t.Errorf("got:\n%s\nwant line %q", result, want)
	}
}

// Re-pinning has to find the entry that already carries a digest, not the shorter
// entry whose text is a prefix of it.
func TestRewriteFile_FlowSequencePrefixSharingAlreadyPinned(t *testing.T) {
	content := "test:\n  services: [postgres:18@sha256:old111, postgres]\n"
	refs, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result := RewriteFile(content, refs, map[int]string{
		0: "sha256:aaa111",
		1: "sha256:bbb222",
	})

	want := "  services: [postgres:18@sha256:aaa111, postgres@sha256:bbb222]"
	if !strings.Contains(result, want) {
		t.Errorf("got:\n%s\nwant line %q", result, want)
	}
	if strings.Contains(result, "sha256:old111") {
		t.Errorf("old digest remains:\n%s", result)
	}
}

// Only the entries that resolved are pinned, the rest of the line staying as written.
func TestRewriteFile_FlowSequencePartialDigests(t *testing.T) {
	content := "test:\n  services: [postgres:18, postgres]\n"
	refs, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result := RewriteFile(content, refs, map[int]string{1: "sha256:bbb222"})

	want := "  services: [postgres:18, postgres@sha256:bbb222]"
	if !strings.Contains(result, want) {
		t.Errorf("got:\n%s\nwant line %q", result, want)
	}
}

// A real digest is far longer than the reference it is appended to, so pinning one
// entry moves every entry after it well past the column it was written at.
func TestRewriteFile_FlowSequenceRealDigestLengths(t *testing.T) {
	content := "test:\n  services: [postgres:18, postgres, postgres:18-alpine]\n"
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

	want := "test:\n  services: [postgres:18@" + digests[0] +
		", postgres@" + digests[1] +
		", postgres:18-alpine@" + digests[2] + "]\n"
	if result != want {
		t.Errorf("got:\n%s\nwant:\n%s", result, want)
	}
}
