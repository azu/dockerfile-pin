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

// A flow sequence puts several references on one line, which no Docker
// Compose file can produce, so the line-based rewrite has to handle more
// than one replacement per line.
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
