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

func TestParse_JobImageMapping(t *testing.T) {
	content := []byte(`
build:
  image:
    name: ruby:3.4
    entrypoint: [""]
  script:
    - bundle install
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}
	r := refs[0]
	if r.ImageRef != "ruby:3.4" {
		t.Errorf("ImageRef = %q, want %q", r.ImageRef, "ruby:3.4")
	}
	if r.Location != "build.image.name" {
		t.Errorf("Location = %q, want %q", r.Location, "build.image.name")
	}
	if r.Line != 4 {
		t.Errorf("Line = %d, want 4 (the name: line)", r.Line)
	}
}

// Hidden jobs are ordinary jobs that no pipeline runs directly; they are
// extended by real jobs, so their images reach the runner all the same.
func TestParse_HiddenJobTemplate(t *testing.T) {
	content := []byte(`
.build:
  image: golang:1.26
build:
  extends: .build
  script:
    - go build ./...
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}
	if refs[0].Location != ".build.image" {
		t.Errorf("Location = %q, want %q", refs[0].Location, ".build.image")
	}
}

func TestParse_NoImages(t *testing.T) {
	content := []byte(`
stages:
  - build

build:
  script:
    - make
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("got %d refs, want 0: %+v", len(refs), refs)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	if _, err := Parse([]byte("build:\n  image: [unclosed\n")); err == nil {
		t.Error("Parse() error = nil, want an error")
	}
}

// Declaring `image:` and `services:` at the root instead of under `default:`
// is deprecated but still accepted by GitLab, and remains widespread in
// existing pipelines.
func TestParse_DeprecatedRootImageAndServices(t *testing.T) {
	content := []byte(`
image: node:24
services:
  - postgres:18
build:
  script:
    - make
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2: %+v", len(refs), refs)
	}
	if refs[0].ImageRef != "node:24" {
		t.Errorf("[0] ImageRef = %q, want %q", refs[0].ImageRef, "node:24")
	}
	if refs[0].Location != "image" {
		t.Errorf("[0] Location = %q, want %q", refs[0].Location, "image")
	}
	if refs[1].ImageRef != "postgres:18" {
		t.Errorf("[1] ImageRef = %q, want %q", refs[1].ImageRef, "postgres:18")
	}
	if refs[1].Location != "services[0]" {
		t.Errorf("[1] Location = %q, want %q", refs[1].Location, "services[0]")
	}
}

// A reference assembled from CI variables cannot be resolved against a
// registry, because the variable values are only known to the running
// pipeline. Such references are reported and left alone.
func TestParse_VariableInterpolation(t *testing.T) {
	content := []byte(`
build:
  image: $CI_REGISTRY_IMAGE:latest
test:
  image: ${NODE_IMAGE}
deploy:
  image: alpine:3.20
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("got %d refs, want 3", len(refs))
	}
	for _, index := range []int{0, 1} {
		if !refs[index].Skip {
			t.Errorf("[%d] Skip = false, want true for %q", index, refs[index].RawRef)
		}
		if refs[index].SkipReason == "" {
			t.Errorf("[%d] SkipReason is empty", index)
		}
	}
	if refs[2].Skip {
		t.Errorf("[2] Skip = true, want false for %q", refs[2].RawRef)
	}
}

func TestParse_AlreadyPinned(t *testing.T) {
	content := []byte(`
build:
  image: node:24@sha256:abc123
  services:
    - name: postgres:18@sha256:def456
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
	for _, tt := range []struct {
		index      int
		wantImage  string
		wantDigest string
		wantRaw    string
	}{
		{0, "node:24", "sha256:abc123", "node:24@sha256:abc123"},
		{1, "postgres:18", "sha256:def456", "postgres:18@sha256:def456"},
	} {
		r := refs[tt.index]
		if r.ImageRef != tt.wantImage {
			t.Errorf("[%d] ImageRef = %q, want %q", tt.index, r.ImageRef, tt.wantImage)
		}
		if r.Digest != tt.wantDigest {
			t.Errorf("[%d] Digest = %q, want %q", tt.index, r.Digest, tt.wantDigest)
		}
		if r.RawRef != tt.wantRaw {
			t.Errorf("[%d] RawRef = %q, want %q", tt.index, r.RawRef, tt.wantRaw)
		}
	}
}

// The `default:` block carries images for every job, while the remaining
// global keywords are not jobs and must never contribute a reference, even
// when `variables:` happens to define a variable called `image`.
func TestParse_GlobalKeywords(t *testing.T) {
	content := []byte(`
stages:
  - build
variables:
  image: not-an-image:1.0
workflow:
  rules:
    - if: $CI_COMMIT_BRANCH
include:
  local: /templates/build.yml
default:
  image: alpine:3.20
build:
  script:
    - make
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1: %+v", len(refs), refs)
	}
	if refs[0].ImageRef != "alpine:3.20" {
		t.Errorf("ImageRef = %q, want %q", refs[0].ImageRef, "alpine:3.20")
	}
	if refs[0].Location != "default.image" {
		t.Errorf("Location = %q, want %q", refs[0].Location, "default.image")
	}
}

func TestParse_JobServicesStrings(t *testing.T) {
	content := []byte(`
test:
  services:
    - postgres:18
    - redis:7
  script:
    - pytest
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
	if refs[0].ImageRef != "postgres:18" {
		t.Errorf("[0] ImageRef = %q, want %q", refs[0].ImageRef, "postgres:18")
	}
	if refs[0].Location != "test.services[0]" {
		t.Errorf("[0] Location = %q, want %q", refs[0].Location, "test.services[0]")
	}
	if refs[1].ImageRef != "redis:7" {
		t.Errorf("[1] ImageRef = %q, want %q", refs[1].ImageRef, "redis:7")
	}
	if refs[1].Location != "test.services[1]" {
		t.Errorf("[1] Location = %q, want %q", refs[1].Location, "test.services[1]")
	}
}

// GitLab service entries key the image on `name:`, unlike Docker Compose,
// where the equivalent key is `image:`.
func TestParse_JobServicesMappings(t *testing.T) {
	content := []byte(`
test:
  services:
    - name: postgres:18
      alias: db
      variables:
        POSTGRES_DB: app
  script:
    - pytest
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}
	r := refs[0]
	if r.ImageRef != "postgres:18" {
		t.Errorf("ImageRef = %q, want %q", r.ImageRef, "postgres:18")
	}
	if r.Location != "test.services[0].name" {
		t.Errorf("Location = %q, want %q", r.Location, "test.services[0].name")
	}
	if r.Line != 4 {
		t.Errorf("Line = %d, want 4 (the name: line)", r.Line)
	}
}
