package compose

import "testing"

func TestParseCompose_BasicServices(t *testing.T) {
	input := []byte("services:\n  web:\n    image: node:20.11.1\n    ports:\n      - \"3000:3000\"\n  db:\n    image: postgres:16.2\n")
	refs, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0].ServiceName != "web" {
		t.Errorf("[0] ServiceName = %q", refs[0].ServiceName)
	}
	if refs[0].ImageRef != "node:20.11.1" {
		t.Errorf("[0] ImageRef = %q", refs[0].ImageRef)
	}
	if refs[1].ServiceName != "db" {
		t.Errorf("[1] ServiceName = %q", refs[1].ServiceName)
	}
	if refs[1].ImageRef != "postgres:16.2" {
		t.Errorf("[1] ImageRef = %q", refs[1].ImageRef)
	}
}

func TestParseCompose_SkipBuild(t *testing.T) {
	input := []byte("services:\n  app:\n    build: .\n    image: myapp:latest\n  web:\n    image: node:20\n")
	refs, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if !refs[0].Skip {
		t.Error("[0] service with build should be skipped")
	}
	if refs[0].SkipReason != "has build directive" {
		t.Errorf("[0] SkipReason = %q", refs[0].SkipReason)
	}
	if refs[1].Skip {
		t.Error("[1] service without build should not be skipped")
	}
}

func TestParseCompose_AlreadyPinned(t *testing.T) {
	input := []byte("services:\n  web:\n    image: node:20.11.1@sha256:abc123\n")
	refs, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if refs[0].ImageRef != "node:20.11.1" {
		t.Errorf("ImageRef = %q", refs[0].ImageRef)
	}
	if refs[0].Digest != "sha256:abc123" {
		t.Errorf("Digest = %q", refs[0].Digest)
	}
}

func TestParseCompose_NoImageKey(t *testing.T) {
	input := []byte("services:\n  builder:\n    build:\n      context: .\n      dockerfile: Dockerfile\n")
	refs, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected 0 refs, got %d", len(refs))
	}
}

func TestParseCompose_MergedAnchorTemplate(t *testing.T) {
	// The example from https://github.com/azu/dockerfile-pin/issues/59
	input := []byte("x-alpine: &alpine\n  image: alpine:latest\n\nservices:\n  bash:\n    <<: *alpine\n    entrypoint: [\"/bin/sh\", \"-c\"]\n")
	refs, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d: %+v", len(refs), refs)
	}
	if refs[0].ImageRef != "alpine:latest" || refs[0].ServiceName != "bash" {
		t.Errorf("ref = %+v", refs[0])
	}
	if refs[0].Line != 2 || refs[0].Column != 10 {
		t.Errorf("position = %d:%d, want 2:10 (the template)", refs[0].Line, refs[0].Column)
	}
}

func TestParseCompose_SharedTemplateIsOneRef(t *testing.T) {
	input := []byte("x-alpine: &alpine\n  image: alpine:latest\nservices:\n  a:\n    <<: *alpine\n  b:\n    <<: *alpine\n")
	refs, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d: %+v", len(refs), refs)
	}
	if refs[0].Line != 2 || refs[0].ServiceName != "a" {
		t.Errorf("ref = %+v, want line 2 reached first from a", refs[0])
	}
}

func TestParseCompose_AliasImageValue(t *testing.T) {
	input := []byte("x-image: &img alpine:3.20\nservices:\n  a:\n    image: *img\n")
	refs, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if refs[0].ImageRef != "alpine:3.20" || refs[0].Line != 1 {
		t.Errorf("ref = %+v, want alpine:3.20 at line 1", refs[0])
	}
}

func TestParseCompose_OwnImageOverridesMerged(t *testing.T) {
	input := []byte("x-alpine: &alpine\n  image: alpine:latest\nservices:\n  a:\n    <<: *alpine\n    image: busybox:1.36\n")
	refs, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d: %+v", len(refs), refs)
	}
	if refs[0].ImageRef != "busybox:1.36" || refs[0].Line != 6 {
		t.Errorf("ref = %+v, want busybox:1.36 at line 6", refs[0])
	}
}

func TestParseCompose_MergeSequencePrecedence(t *testing.T) {
	input := []byte("x-a: &a\n  image: alpine:3\nx-b: &b\n  image: busybox:1\nservices:\n  s:\n    <<: [*a, *b]\n")
	refs, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 || refs[0].ImageRef != "alpine:3" {
		t.Fatalf("refs = %+v, want only alpine:3", refs)
	}
}

func TestParseCompose_MergedBuildSkips(t *testing.T) {
	input := []byte("x-app: &app\n  build: .\n  image: myapp:latest\nservices:\n  a:\n    <<: *app\n")
	refs, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 || !refs[0].Skip || refs[0].SkipReason != "has build directive" {
		t.Fatalf("refs = %+v, want one skipped ref", refs)
	}
}

func TestParseCompose_SharedTemplateCheckedWhenAnyUseHasNoBuild(t *testing.T) {
	for name, input := range map[string]string{
		"build service first": "x-alpine: &alpine\n  image: alpine:latest\nservices:\n  built:\n    <<: *alpine\n    build: .\n  pulled:\n    <<: *alpine\n",
		"build service last":  "x-alpine: &alpine\n  image: alpine:latest\nservices:\n  pulled:\n    <<: *alpine\n  built:\n    <<: *alpine\n    build: .\n",
	} {
		t.Run(name, func(t *testing.T) {
			refs, err := Parse([]byte(input))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(refs) != 1 {
				t.Fatalf("expected 1 ref, got %d: %+v", len(refs), refs)
			}
			if refs[0].Skip || refs[0].ImageRef != "alpine:latest" || refs[0].Line != 2 {
				t.Errorf("ref = %+v, want unskipped alpine:latest at line 2", refs[0])
			}
		})
	}
}

func TestParseCompose_ServicesFromAlias(t *testing.T) {
	input := []byte("x-services: &services\n  web:\n    image: node:20\nservices: *services\n")
	refs, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(refs) != 1 || refs[0].ImageRef != "node:20" || refs[0].Line != 3 {
		t.Fatalf("refs = %+v, want node:20 at line 3", refs)
	}
}
