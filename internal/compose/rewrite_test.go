package compose

import (
	"strings"
	"testing"
)

func TestRewriteCompose(t *testing.T) {
	input := "services:\n  web:\n    image: node:20.11.1\n    ports:\n      - \"3000:3000\"\n  db:\n    image: postgres:16.2\n    environment:\n      POSTGRES_PASSWORD: secret\n  app:\n    build: .\n    image: myapp:latest\n"
	refs := []ComposeImageRef{
		{ServiceName: "web", ImageRef: "node:20.11.1", RawRef: "node:20.11.1", Line: 3},
		{ServiceName: "db", ImageRef: "postgres:16.2", RawRef: "postgres:16.2", Line: 7},
		{ServiceName: "app", ImageRef: "myapp:latest", RawRef: "myapp:latest", Line: 12, Skip: true, SkipReason: "has build directive"},
	}
	digests := map[int]string{
		0: "sha256:aaa111",
		1: "sha256:bbb222",
	}
	got := RewriteFile(input, refs, digests)
	if !strings.Contains(got, "image: node:20.11.1@sha256:aaa111") {
		t.Errorf("expected node pinned, got:\n%s", got)
	}
	if !strings.Contains(got, "image: postgres:16.2@sha256:bbb222") {
		t.Errorf("expected postgres pinned, got:\n%s", got)
	}
	if !strings.Contains(got, "image: myapp:latest") {
		t.Errorf("expected myapp NOT pinned, got:\n%s", got)
	}
}

func TestRewriteCompose_UpdateExisting(t *testing.T) {
	input := "services:\n  web:\n    image: node:20.11.1@sha256:olddigest\n"
	refs := []ComposeImageRef{
		{ServiceName: "web", ImageRef: "node:20.11.1", RawRef: "node:20.11.1@sha256:olddigest", Digest: "sha256:olddigest", Line: 3},
	}
	digests := map[int]string{0: "sha256:newdigest"}
	got := RewriteFile(input, refs, digests)
	if !strings.Contains(got, "image: node:20.11.1@sha256:newdigest") {
		t.Errorf("expected digest updated, got:\n%s", got)
	}
}

// A flow mapping writes several services on one line, and one image may be a
// prefix of another, so replacing by text alone would match inside the wrong one.
func TestRewriteCompose_FlowMappingPrefixSharingImages(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "later image is a prefix of an earlier one",
			input: "services: {a: {image: postgres:18}, b: {image: postgres}}\n",
			want:  "services: {a: {image: postgres:18@sha256:aaa111}, b: {image: postgres@sha256:bbb222}}\n",
		},
		{
			name:  "identical images",
			input: "services: {a: {image: postgres}, b: {image: postgres}}\n",
			want:  "services: {a: {image: postgres@sha256:aaa111}, b: {image: postgres@sha256:bbb222}}\n",
		},
		{
			name:  "quoted images",
			input: "services: {a: {image: \"postgres:18\"}, b: {image: 'postgres'}}\n",
			want:  "services: {a: {image: \"postgres:18@sha256:aaa111\"}, b: {image: 'postgres@sha256:bbb222'}}\n",
		},
		{
			name:  "earlier image already pinned",
			input: "services: {a: {image: postgres:18@sha256:old111}, b: {image: postgres}}\n",
			want:  "services: {a: {image: postgres:18@sha256:aaa111}, b: {image: postgres@sha256:bbb222}}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs, err := Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(refs) != 2 {
				t.Fatalf("Parse() returned %d refs, want 2", len(refs))
			}
			got := RewriteFile(tt.input, refs, map[int]string{
				0: "sha256:aaa111",
				1: "sha256:bbb222",
			})

			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// A real digest is far longer than the image it is appended to, so pinning one
// image moves every image after it well past the column it was written at.
func TestRewriteCompose_FlowMappingRealDigestLengths(t *testing.T) {
	input := "services: {a: {image: postgres:18}, b: {image: postgres}, c: {image: postgres:18-alpine}}\n"
	refs, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	digests := map[int]string{
		0: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		1: "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		2: "sha256:3333333333333333333333333333333333333333333333333333333333333333",
	}
	got := RewriteFile(input, refs, digests)

	want := "services: {a: {image: postgres:18@" + digests[0] +
		"}, b: {image: postgres@" + digests[1] +
		"}, c: {image: postgres:18-alpine@" + digests[2] + "}}\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
