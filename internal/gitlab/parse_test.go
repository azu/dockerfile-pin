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
