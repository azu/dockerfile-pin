package actions

import (
	"testing"
)

func TestParse_WorkflowContainerImage(t *testing.T) {
	content := []byte(`
name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    container:
      image: node:24
    steps:
      - uses: actions/checkout@v4
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
	if r.HasPrefix {
		t.Error("HasPrefix should be false")
	}
	if r.Location != "jobs.test.container.image" {
		t.Errorf("Location = %q, want %q", r.Location, "jobs.test.container.image")
	}
}

func TestParse_WorkflowContainerString(t *testing.T) {
	content := []byte(`
name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    container: node:24
    steps:
      - uses: actions/checkout@v4
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}
	if refs[0].ImageRef != "node:24" {
		t.Errorf("ImageRef = %q, want %q", refs[0].ImageRef, "node:24")
	}
	if refs[0].Location != "jobs.test.container" {
		t.Errorf("Location = %q, want %q", refs[0].Location, "jobs.test.container")
	}
}

func TestParse_WorkflowServices(t *testing.T) {
	content := []byte(`
name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      db:
        image: postgres:18
      redis:
        image: redis:7
    steps:
      - uses: actions/checkout@v4
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
	if refs[0].Location != "jobs.test.services.db.image" {
		t.Errorf("[0] Location = %q, want %q", refs[0].Location, "jobs.test.services.db.image")
	}
	if refs[1].ImageRef != "redis:7" {
		t.Errorf("[1] ImageRef = %q, want %q", refs[1].ImageRef, "redis:7")
	}
}

func TestParse_WorkflowServiceWithDockerPrefix(t *testing.T) {
	content := []byte(`
name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      db:
        image: docker://postgres:18
    steps:
      - uses: actions/checkout@v4
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
	if !r.HasPrefix {
		t.Error("HasPrefix should be true for docker:// prefixed service image")
	}
	if r.RawRef != "docker://postgres:18" {
		t.Errorf("RawRef = %q, want %q", r.RawRef, "docker://postgres:18")
	}
}

func TestParse_WorkflowDockerStep(t *testing.T) {
	content := []byte(`
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker://ghcr.io/foo/bar:latest
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}
	r := refs[0]
	if r.ImageRef != "ghcr.io/foo/bar:latest" {
		t.Errorf("ImageRef = %q, want %q", r.ImageRef, "ghcr.io/foo/bar:latest")
	}
	if !r.HasPrefix {
		t.Error("HasPrefix should be true")
	}
	if r.RawRef != "docker://ghcr.io/foo/bar:latest" {
		t.Errorf("RawRef = %q, want %q", r.RawRef, "docker://ghcr.io/foo/bar:latest")
	}
}

func TestParse_WorkflowNonDockerStepIgnored(t *testing.T) {
	content := []byte(`
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("got %d refs, want 0 (non-docker steps should be ignored)", len(refs))
	}
}

func TestParse_WorkflowMixed(t *testing.T) {
	content := []byte(`
name: CI
on: push
jobs:
  sample:
    runs-on: ubuntu-latest
    container:
      image: node:24
    services:
      db:
        image: postgres:18
    steps:
      - uses: docker://ghcr.io/foo/bar:latest
      - uses: actions/checkout@v4
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("got %d refs, want 3", len(refs))
	}
	if refs[0].ImageRef != "node:24" {
		t.Errorf("[0] ImageRef = %q, want %q", refs[0].ImageRef, "node:24")
	}
	if refs[1].ImageRef != "postgres:18" {
		t.Errorf("[1] ImageRef = %q, want %q", refs[1].ImageRef, "postgres:18")
	}
	if refs[2].ImageRef != "ghcr.io/foo/bar:latest" {
		t.Errorf("[2] ImageRef = %q, want %q", refs[2].ImageRef, "ghcr.io/foo/bar:latest")
	}
}

func TestParse_ActionDockerImage(t *testing.T) {
	content := []byte(`
name: My Action
description: A custom action
runs:
  using: docker
  image: docker://debian:stretch-slim
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}
	r := refs[0]
	if r.ImageRef != "debian:stretch-slim" {
		t.Errorf("ImageRef = %q, want %q", r.ImageRef, "debian:stretch-slim")
	}
	if !r.HasPrefix {
		t.Error("HasPrefix should be true")
	}
	if r.Location != "runs.image" {
		t.Errorf("Location = %q, want %q", r.Location, "runs.image")
	}
}

func TestParse_ActionLocalDockerfile(t *testing.T) {
	content := []byte(`
name: My Action
description: A custom action
runs:
  using: docker
  image: Dockerfile
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1 (local Dockerfile should be a skip ref)", len(refs))
	}
	r := refs[0]
	if !r.Skip {
		t.Error("Skip should be true for local Dockerfile")
	}
	if r.SkipReason != "local Dockerfile" {
		t.Errorf("SkipReason = %q, want %q", r.SkipReason, "local Dockerfile")
	}
}

func TestParse_ActionLocalDockerfilePath(t *testing.T) {
	content := []byte(`
name: My Action
description: A custom action
runs:
  using: docker
  image: ./Dockerfile
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}
	if !refs[0].Skip {
		t.Error("Skip should be true for local Dockerfile path")
	}
}

func TestParse_AlreadyPinned(t *testing.T) {
	content := []byte(`
name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    container:
      image: node:24@sha256:abc123
    steps:
      - uses: docker://ghcr.io/foo/bar:latest@sha256:def456
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
	if refs[0].ImageRef != "node:24" {
		t.Errorf("[0] ImageRef = %q, want %q", refs[0].ImageRef, "node:24")
	}
	if refs[0].Digest != "sha256:abc123" {
		t.Errorf("[0] Digest = %q, want %q", refs[0].Digest, "sha256:abc123")
	}
	if refs[1].ImageRef != "ghcr.io/foo/bar:latest" {
		t.Errorf("[1] ImageRef = %q, want %q", refs[1].ImageRef, "ghcr.io/foo/bar:latest")
	}
	if refs[1].Digest != "sha256:def456" {
		t.Errorf("[1] Digest = %q, want %q", refs[1].Digest, "sha256:def456")
	}
}

func TestParse_NoRelevantKeys(t *testing.T) {
	content := []byte(`
name: something
version: "1.0"
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("got %d refs, want 0", len(refs))
	}
}

func TestParse_ContainerWithDockerPrefix(t *testing.T) {
	content := []byte(`
name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    container: docker://node:24
    steps:
      - uses: actions/checkout@v4
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
	if !r.HasPrefix {
		t.Error("HasPrefix should be true for docker:// prefixed container")
	}
	if r.RawRef != "docker://node:24" {
		t.Errorf("RawRef = %q, want %q", r.RawRef, "docker://node:24")
	}
}

func TestParse_ContainerImageWithDockerPrefix(t *testing.T) {
	content := []byte(`
name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    container:
      image: docker://node:24
    steps:
      - uses: actions/checkout@v4
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
	if !r.HasPrefix {
		t.Error("HasPrefix should be true for docker:// prefixed container image")
	}
}

func TestParse_MultipleJobs(t *testing.T) {
	content := []byte(`
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    container:
      image: golang:1.22
    steps:
      - uses: actions/checkout@v4
  test:
    runs-on: ubuntu-latest
    services:
      db:
        image: postgres:16
    steps:
      - uses: docker://alpine:3.19
`)
	refs, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("got %d refs, want 3", len(refs))
	}
	images := []string{refs[0].ImageRef, refs[1].ImageRef, refs[2].ImageRef}
	expected := []string{"golang:1.22", "postgres:16", "alpine:3.19"}
	for i, want := range expected {
		if images[i] != want {
			t.Errorf("[%d] ImageRef = %q, want %q", i, images[i], want)
		}
	}
}
