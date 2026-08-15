package gitlab

import (
	"testing"
)

func TestParse_JobImageString(t *testing.T) {
	content := []byte(`
stages:
  - build

build:
  stage: build
  image: node:24
  script:
    - npm ci
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}
	r := refs[0]
	if r.ImageRef != "node:24" {
		t.Errorf("ImageRef = %q, want %q", r.ImageRef, "node:24")
	}
	if r.Location != "build.image" {
		t.Errorf("Location = %q, want %q", r.Location, "build.image")
	}
	if r.Line != 7 {
		t.Errorf("Line = %d, want 7", r.Line)
	}
}
