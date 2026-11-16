package dockerfile

import (
	"strings"
	"testing"
)

func TestAddDigest(t *testing.T) {
	tests := []struct {
		name     string
		original string
		rawRef   string
		digest   string
		want     string
	}{
		{
			name:     "simple tag",
			original: "FROM node:20.11.1",
			rawRef:   "node:20.11.1",
			digest:   "sha256:abc123",
			want:     "FROM node:20.11.1@sha256:abc123",
		},
		{
			name:     "with AS clause",
			original: "FROM python:3.12-slim AS builder",
			rawRef:   "python:3.12-slim",
			digest:   "sha256:def456",
			want:     "FROM python:3.12-slim@sha256:def456 AS builder",
		},
		{
			name:     "with platform",
			original: "FROM --platform=linux/amd64 golang:1.22",
			rawRef:   "golang:1.22",
			digest:   "sha256:ghi789",
			want:     "FROM --platform=linux/amd64 golang:1.22@sha256:ghi789",
		},
		{
			name:     "with ARG variable",
			original: "FROM node:${NODE_VERSION}",
			rawRef:   "node:${NODE_VERSION}",
			digest:   "sha256:abc123",
			want:     "FROM node:${NODE_VERSION}@sha256:abc123",
		},
		{
			name:     "update existing digest",
			original: "FROM node:20.11.1@sha256:olddigest",
			rawRef:   "node:20.11.1@sha256:olddigest",
			digest:   "sha256:newdigest",
			want:     "FROM node:20.11.1@sha256:newdigest",
		},
		{
			name:     "with platform and AS",
			original: "FROM --platform=linux/amd64 golang:1.22 AS builder",
			rawRef:   "golang:1.22",
			digest:   "sha256:abc123",
			want:     "FROM --platform=linux/amd64 golang:1.22@sha256:abc123 AS builder",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AddDigest(tt.original, tt.rawRef, tt.digest)
			if got != tt.want {
				t.Errorf("AddDigest() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRewriteFile(t *testing.T) {
	content := "# My Dockerfile\nFROM node:20.11.1\nRUN npm install\nFROM python:3.12-slim AS builder\nRUN pip install -r requirements.txt\nFROM scratch\n"
	instructions := []FromInstruction{
		{ImageRef: "node:20.11.1", RawRef: "node:20.11.1", StartLine: 2, Original: "FROM node:20.11.1"},
		{ImageRef: "python:3.12-slim", RawRef: "python:3.12-slim", StartLine: 4, Original: "FROM python:3.12-slim AS builder"},
		{ImageRef: "scratch", RawRef: "scratch", StartLine: 6, Original: "FROM scratch", Skip: true, SkipReason: "scratch image"},
	}
	digests := map[int]string{
		0: "sha256:abc123",
		1: "sha256:def456",
	}

	got := RewriteFile(content, instructions, digests)
	want := "# My Dockerfile\nFROM node:20.11.1@sha256:abc123\nRUN npm install\nFROM python:3.12-slim@sha256:def456 AS builder\nRUN pip install -r requirements.txt\nFROM scratch\n"
	if got != want {
		t.Errorf("RewriteFile() =\n%s\nwant:\n%s", got, want)
	}
}

func TestAddCopyFromDigest(t *testing.T) {
	tests := []struct {
		name     string
		original string
		rawRef   string
		digest   string
		want     string
	}{
		{
			name:     "simple tag",
			original: "COPY --from=nginx:1.27 /etc/nginx /etc/nginx",
			rawRef:   "nginx:1.27",
			digest:   "sha256:abc123",
			want:     "COPY --from=nginx:1.27@sha256:abc123 /etc/nginx /etc/nginx",
		},
		{
			name:     "replaces an existing digest",
			original: "COPY --from=nginx:1.27@sha256:olddigest /etc/nginx /etc/nginx",
			rawRef:   "nginx:1.27@sha256:olddigest",
			digest:   "sha256:newdigest",
			want:     "COPY --from=nginx:1.27@sha256:newdigest /etc/nginx /etc/nginx",
		},
		{
			name:     "other flags are left alone",
			original: "COPY --chown=65532:65532 --from=gcr.io/distroless/base:nonroot --chmod=755 /etc/passwd /etc/passwd",
			rawRef:   "gcr.io/distroless/base:nonroot",
			digest:   "sha256:abc123",
			want:     "COPY --chown=65532:65532 --from=gcr.io/distroless/base:nonroot@sha256:abc123 --chmod=755 /etc/passwd /etc/passwd",
		},
		{
			// Anchoring on --from= is what keeps the earlier --chown value intact:
			// a plain first-occurrence replacement would rewrite that instead.
			name:     "the same text in an earlier flag is not the reference",
			original: "COPY --chown=node:20 --from=node:20 /app /app",
			rawRef:   "node:20",
			digest:   "sha256:abc123",
			want:     "COPY --chown=node:20 --from=node:20@sha256:abc123 /app /app",
		},
		{
			name:     "registry with a port keeps its colon",
			original: "COPY --from=registry.example.com:5000/tool:1.0 /tool /tool",
			rawRef:   "registry.example.com:5000/tool:1.0",
			digest:   "sha256:abc123",
			want:     "COPY --from=registry.example.com:5000/tool:1.0@sha256:abc123 /tool /tool",
		},
		{
			name:     "reference absent from the line",
			original: "COPY --from=nginx:1.27 /etc/nginx /etc/nginx",
			rawRef:   "alpine:3.19",
			digest:   "sha256:abc123",
			want:     "COPY --from=nginx:1.27 /etc/nginx /etc/nginx",
		},
		{
			name:     "empty reference",
			original: "COPY --from= /a /b",
			rawRef:   "",
			digest:   "sha256:abc123",
			want:     "COPY --from= /a /b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AddCopyFromDigest(tt.original, tt.rawRef, tt.digest); got != tt.want {
				t.Errorf("AddCopyFromDigest() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRewriteFile_IssueReproduction pins the Dockerfile from the feature request and
// compares the whole file, including the parts that must not change.
func TestRewriteFile_IssueReproduction(t *testing.T) {
	content := "# Dockerfile\nFROM ubuntu:24.04\nCOPY --from=nginx:1.27 /etc/nginx /etc/nginx\nCOPY --from=0 /a /b\n"
	instructions, err := Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	digests := map[int]string{
		0: "sha256:33ceb71981b602c1a7443a53469e4dba065f7503eab3078a2d7a57a2ab987517",
		1: "sha256:6784fb08b4b7c3b6bcd3f4a1b4d1b1f3e3b7a7ca42ec3e0d9df8a97a2c9a3b1d",
	}
	want := "# Dockerfile\n" +
		"FROM ubuntu:24.04@sha256:33ceb71981b602c1a7443a53469e4dba065f7503eab3078a2d7a57a2ab987517\n" +
		"COPY --from=nginx:1.27@sha256:6784fb08b4b7c3b6bcd3f4a1b4d1b1f3e3b7a7ca42ec3e0d9df8a97a2c9a3b1d /etc/nginx /etc/nginx\n" +
		"COPY --from=0 /a /b\n"
	if got := RewriteFile(content, instructions, digests); got != want {
		t.Errorf("RewriteFile() =\n%s\nwant:\n%s", got, want)
	}
}

// TestRewriteFile_CopyFromMultiStage checks a realistic multi-stage file: external
// images are pinned while stage references, stage indexes and plain COPY are not.
func TestRewriteFile_CopyFromMultiStage(t *testing.T) {
	content := "FROM golang:1.22 AS builder\n" +
		"RUN go build -o /app\n" +
		"FROM gcr.io/distroless/base:nonroot\n" +
		"COPY --from=builder /app /app\n" +
		"COPY --from=0 /go/bin/tool /usr/local/bin/tool\n" +
		"COPY --chown=65532:65532 --from=nginx:1.27 /etc/nginx /etc/nginx\n" +
		"COPY ./config /config\n"
	instructions, err := Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	digests := map[int]string{
		0: "sha256:golang111",
		1: "sha256:distroless222",
		// index 2 is the stage reference, index 3 the stage index: both skipped.
		4: "sha256:nginx333",
	}
	want := "FROM golang:1.22@sha256:golang111 AS builder\n" +
		"RUN go build -o /app\n" +
		"FROM gcr.io/distroless/base:nonroot@sha256:distroless222\n" +
		"COPY --from=builder /app /app\n" +
		"COPY --from=0 /go/bin/tool /usr/local/bin/tool\n" +
		"COPY --chown=65532:65532 --from=nginx:1.27@sha256:nginx333 /etc/nginx /etc/nginx\n" +
		"COPY ./config /config\n"
	if got := RewriteFile(content, instructions, digests); got != want {
		t.Errorf("RewriteFile() =\n%s\nwant:\n%s", got, want)
	}
}

// TestRewriteFile_LineContinuation covers instructions continued with "\", where the
// reference sits on a line below the one the instruction starts on.
func TestRewriteFile_LineContinuation(t *testing.T) {
	content := "FROM \\\n  ubuntu:24.04\n" +
		"COPY \\\n  --from=nginx:1.27 \\\n  /etc/nginx /etc/nginx\n"
	instructions, err := Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	digests := map[int]string{0: "sha256:ubuntu111", 1: "sha256:nginx222"}
	want := "FROM \\\n  ubuntu:24.04@sha256:ubuntu111\n" +
		"COPY \\\n  --from=nginx:1.27@sha256:nginx222 \\\n  /etc/nginx /etc/nginx\n"
	if got := RewriteFile(content, instructions, digests); got != want {
		t.Errorf("RewriteFile() =\n%s\nwant:\n%s", got, want)
	}
}

// TestRewriteFile_CopyFromUpdatesExistingDigest covers the --update path, where an
// already-pinned COPY --from is re-resolved to a new digest.
func TestRewriteFile_CopyFromUpdatesExistingDigest(t *testing.T) {
	content := "FROM ubuntu:24.04@sha256:oldubuntu\nCOPY --from=nginx:1.27@sha256:oldnginx /etc/nginx /etc/nginx\n"
	instructions, err := Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	digests := map[int]string{0: "sha256:newubuntu", 1: "sha256:newnginx"}
	want := "FROM ubuntu:24.04@sha256:newubuntu\nCOPY --from=nginx:1.27@sha256:newnginx /etc/nginx /etc/nginx\n"
	if got := RewriteFile(content, instructions, digests); got != want {
		t.Errorf("RewriteFile() =\n%s\nwant:\n%s", got, want)
	}
}

// TestRewriteFile_RepeatedCopyFrom checks that two COPY instructions using the same
// image are pinned on their own lines rather than one line twice.
func TestRewriteFile_RepeatedCopyFrom(t *testing.T) {
	content := "COPY --from=nginx:1.27 /etc/nginx /etc/nginx\nCOPY --from=nginx:1.27 /usr/share/nginx /usr/share/nginx\n"
	instructions, err := Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	digests := map[int]string{0: "sha256:nginx111", 1: "sha256:nginx111"}
	want := "COPY --from=nginx:1.27@sha256:nginx111 /etc/nginx /etc/nginx\nCOPY --from=nginx:1.27@sha256:nginx111 /usr/share/nginx /usr/share/nginx\n"
	if got := RewriteFile(content, instructions, digests); got != want {
		t.Errorf("RewriteFile() =\n%s\nwant:\n%s", got, want)
	}
}

// TestRewriteFile_SkippedCopyFromUntouched checks that a digest handed in for a
// skipped instruction never reaches the file.
func TestRewriteFile_SkippedCopyFromUntouched(t *testing.T) {
	content := "FROM golang:1.22 AS builder\nCOPY --from=builder /app /app\nCOPY --from=scratch /a /b\n"
	instructions, err := Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	digests := map[int]string{0: "sha256:golang111", 1: "sha256:bogus", 2: "sha256:bogus"}
	want := "FROM golang:1.22@sha256:golang111 AS builder\nCOPY --from=builder /app /app\nCOPY --from=scratch /a /b\n"
	if got := RewriteFile(content, instructions, digests); got != want {
		t.Errorf("RewriteFile() =\n%s\nwant:\n%s", got, want)
	}
}

// TestRewriteFile_CopyFromAnchoredToFlag guards the whole-file path against
// rewriting text that merely looks like the reference: here the image ref is also
// the value of an earlier --chown flag, and only the --from= one may gain a digest.
func TestRewriteFile_CopyFromAnchoredToFlag(t *testing.T) {
	content := "FROM node:20 AS builder\nCOPY --chown=node:20 --from=node:20 /app /app\n"
	instructions, err := Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	digests := map[int]string{0: "sha256:node111", 1: "sha256:node222"}
	want := "FROM node:20@sha256:node111 AS builder\nCOPY --chown=node:20 --from=node:20@sha256:node222 /app /app\n"
	if got := RewriteFile(content, instructions, digests); got != want {
		t.Errorf("RewriteFile() =\n%s\nwant:\n%s", got, want)
	}
}

// TestRewriteFile_ReferenceSplitAcrossLines covers a reference broken up by a "\"
// continuation, where no single line holds the text the parser reports. Searching each
// line on its own would find nothing and leave the file unchanged while the caller
// still counted the image as pinned.
func TestRewriteFile_ReferenceSplitAcrossLines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		digest  string
		want    string
	}{
		{
			name:    "value continues on the next line",
			content: "COPY --from=\\\nnginx:1.27 /src /dst\n",
			digest:  "sha256:nginx111",
			want:    "COPY --from=\\\nnginx:1.27@sha256:nginx111 /src /dst\n",
		},
		{
			name:    "the flag name itself is split",
			content: "COPY --fr\\\nom=nginx:1.27 /src /dst\n",
			digest:  "sha256:nginx111",
			want:    "COPY --fr\\\nom=nginx:1.27@sha256:nginx111 /src /dst\n",
		},
		{
			name:    "FROM split mid-reference",
			content: "FROM ubu\\\nntu:24.04\n",
			digest:  "sha256:ubuntu111",
			want:    "FROM ubu\\\nntu:24.04@sha256:ubuntu111\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instructions, err := Parse(strings.NewReader(tt.content))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(instructions) != 1 {
				t.Fatalf("expected 1 instruction, got %d", len(instructions))
			}
			got, unrewritten := RewriteFileReport(tt.content, instructions, map[int]string{0: tt.digest})
			if len(unrewritten) != 0 {
				t.Errorf("instruction reported as unrewritten: %v", unrewritten)
			}
			if got != tt.want {
				t.Errorf("RewriteFile() =\n%q\nwant:\n%q", got, tt.want)
			}
			// The result must still parse, and now carry the digest.
			after, err := Parse(strings.NewReader(got))
			if err != nil {
				t.Fatalf("rewritten file no longer parses: %v", err)
			}
			if len(after) != 1 || after[0].Digest != tt.digest {
				t.Errorf("after rewrite: %+v, want digest %q", after, tt.digest)
			}
		})
	}
}

// TestRewriteFile_SplitExistingDigestReplaced covers --update where the digest being
// replaced is itself broken across the continuation.
func TestRewriteFile_SplitExistingDigestReplaced(t *testing.T) {
	content := "COPY --from=nginx:1.27@sha256:\\\nolddigest /src /dst\n"
	instructions, err := Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got := RewriteFile(content, instructions, map[int]string{0: "sha256:newdigest"})
	after, err := Parse(strings.NewReader(got))
	if err != nil {
		t.Fatalf("rewritten file no longer parses: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("expected 1 instruction after rewrite, got %d", len(after))
	}
	if after[0].ImageRef != "nginx:1.27" || after[0].Digest != "sha256:newdigest" {
		t.Errorf("after rewrite ref=%q digest=%q, want nginx:1.27 / sha256:newdigest",
			after[0].ImageRef, after[0].Digest)
	}
	if strings.Contains(got, "olddigest") {
		t.Errorf("old digest still present:\n%q", got)
	}
}

// TestRewriteFile_OnbuildCopyFrom pins the image an ONBUILD COPY --from names.
func TestRewriteFile_OnbuildCopyFrom(t *testing.T) {
	content := "FROM alpine:3.19\nONBUILD COPY --from=nginx:1.27 /a /b\n"
	instructions, err := Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	digests := map[int]string{0: "sha256:alpine111", 1: "sha256:nginx222"}
	want := "FROM alpine:3.19@sha256:alpine111\nONBUILD COPY --from=nginx:1.27@sha256:nginx222 /a /b\n"
	if got := RewriteFile(content, instructions, digests); got != want {
		t.Errorf("RewriteFile() =\n%s\nwant:\n%s", got, want)
	}
}

// TestRewriteFileReport_UnrewrittenReported checks that a reference the rewriter cannot
// find is reported rather than silently counted as pinned.
func TestRewriteFileReport_UnrewrittenReported(t *testing.T) {
	content := "FROM alpine:3.19\nRUN echo hi\n"
	instructions := []FromInstruction{
		{ImageRef: "alpine:3.19", RawRef: "alpine:3.19", StartLine: 1},
		{ImageRef: "nginx:1.27", RawRef: "nginx:1.27", StartLine: 2, IsCopyFrom: true},
	}
	got, unrewritten := RewriteFileReport(content, instructions, map[int]string{
		0: "sha256:alpine111",
		1: "sha256:nginx222",
	})
	if want := "FROM alpine:3.19@sha256:alpine111\nRUN echo hi\n"; got != want {
		t.Errorf("RewriteFileReport() =\n%q\nwant:\n%q", got, want)
	}
	if len(unrewritten) != 1 || unrewritten[0] != 1 {
		t.Errorf("unrewritten = %v, want [1]", unrewritten)
	}
}

// TestRewriteFile_EscapeDirective covers a Dockerfile that changes its continuation
// character with "# escape=`". Stripping a hard-coded backslash would leave the
// backtick in the rebuilt instruction, and the reference would never be found.
func TestRewriteFile_EscapeDirective(t *testing.T) {
	tests := []struct {
		name    string
		content string
		digest  string
		want    string
	}{
		{
			name:    "COPY --from split on a backtick",
			content: "# escape=`\nCOPY --from=nginx:`\n1.27 /src /dst\n",
			digest:  "sha256:nginx111",
			want:    "# escape=`\nCOPY --from=nginx:`\n1.27@sha256:nginx111 /src /dst\n",
		},
		{
			name:    "FROM split on a backtick",
			content: "# escape=`\nFROM ubu`\nntu:24.04\n",
			digest:  "sha256:ubuntu111",
			want:    "# escape=`\nFROM ubu`\nntu:24.04@sha256:ubuntu111\n",
		},
		{
			// With a backtick escape a backslash is an ordinary character, so a
			// Windows-style path must survive untouched.
			name:    "backslashes in a path are not continuations",
			content: "# escape=`\nCOPY --from=nginx:1.27 `\n   C:\\nginx\\conf C:\\dst\n",
			digest:  "sha256:nginx222",
			want:    "# escape=`\nCOPY --from=nginx:1.27@sha256:nginx222 `\n   C:\\nginx\\conf C:\\dst\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instructions, err := Parse(strings.NewReader(tt.content))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(instructions) != 1 {
				t.Fatalf("expected 1 instruction, got %d", len(instructions))
			}
			got, unrewritten := RewriteFileReport(tt.content, instructions, map[int]string{0: tt.digest})
			if len(unrewritten) != 0 {
				t.Errorf("instruction reported as unrewritten: %v", unrewritten)
			}
			if got != tt.want {
				t.Errorf("RewriteFile() =\n%q\nwant:\n%q", got, tt.want)
			}
			after, err := Parse(strings.NewReader(got))
			if err != nil {
				t.Fatalf("rewritten file no longer parses: %v", err)
			}
			if len(after) != 1 || after[0].Digest != tt.digest {
				t.Errorf("after rewrite: %+v, want digest %q", after, tt.digest)
			}
		})
	}
}

// TestRewriteFile_OnbuildPinsNameMatchingLocalStage is the rewrite side of ONBUILD
// scoping: the name matches a stage in this file, but the trigger runs elsewhere, so
// it is pinned as an image.
func TestRewriteFile_OnbuildPinsNameMatchingLocalStage(t *testing.T) {
	content := "FROM alpine:3.19 AS nginx\nONBUILD COPY --from=nginx /a /b\n"
	instructions, err := Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(instructions) != 2 || instructions[1].Skip {
		t.Fatalf("expected the ONBUILD ref to be pinnable, got %+v", instructions)
	}
	digests := map[int]string{0: "sha256:alpine111", 1: "sha256:nginx222"}
	want := "FROM alpine:3.19@sha256:alpine111 AS nginx\nONBUILD COPY --from=nginx@sha256:nginx222 /a /b\n"
	if got := RewriteFile(content, instructions, digests); got != want {
		t.Errorf("RewriteFile() =\n%s\nwant:\n%s", got, want)
	}
}

func TestRewriteFile_RunMountFrom(t *testing.T) {
	const d = "sha256:aaa111"
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "bind mount",
			input: "FROM scratch\nRUN --mount=type=bind,from=nginx:1.27,source=/etc/nginx,target=/mnt cp -r /mnt /etc\n",
			want:  "FROM scratch\nRUN --mount=type=bind,from=nginx:1.27@" + d + ",source=/etc/nginx,target=/mnt cp -r /mnt /etc\n",
		},
		{
			name:  "from is the last option",
			input: "FROM scratch\nRUN --mount=target=/mnt,from=nginx:1.27 true\n",
			want:  "FROM scratch\nRUN --mount=target=/mnt,from=nginx:1.27@" + d + " true\n",
		},
		{
			name:  "existing digest is replaced",
			input: "FROM scratch\nRUN --mount=from=nginx:1.27@sha256:old,target=/mnt true\n",
			want:  "FROM scratch\nRUN --mount=from=nginx:1.27@" + d + ",target=/mnt true\n",
		},
		{
			name:  "two mounts of the same image are each pinned once",
			input: "FROM scratch\nRUN --mount=from=nginx:1.27,target=/a --mount=from=nginx:1.27,target=/b true\n",
			want:  "FROM scratch\nRUN --mount=from=nginx:1.27@" + d + ",target=/a --mount=from=nginx:1.27@" + d + ",target=/b true\n",
		},
		{
			name:  "same text in another option and in the command stays",
			input: "FROM scratch\nRUN --mount=type=bind,source=nginx:1.27,from=nginx:1.27,target=/a echo from=nginx:1.27\n",
			want:  "FROM scratch\nRUN --mount=type=bind,source=nginx:1.27,from=nginx:1.27@" + d + ",target=/a echo from=nginx:1.27\n",
		},
		{
			name:  "mounts on continued lines",
			input: "FROM scratch\nRUN --mount=type=cache,target=/c \\\n    --mount=type=bind,from=nginx:1.27,target=/a \\\n    --mount=type=bind,from=busybox:1,target=/b \\\n    true\n",
			want:  "FROM scratch\nRUN --mount=type=cache,target=/c \\\n    --mount=type=bind,from=nginx:1.27@" + d + ",target=/a \\\n    --mount=type=bind,from=busybox:1@" + d + ",target=/b \\\n    true\n",
		},
		{
			name:  "quoted value and upper case key",
			input: "FROM scratch\nRUN --mount=type=bind,FROM=\"nginx:1.27\",target=/a true\n",
			want:  "FROM scratch\nRUN --mount=type=bind,FROM=\"nginx:1.27@" + d + "\",target=/a true\n",
		},
		{
			// BuildKit uses the last from, so that is the one to pin.
			name:  "repeated from pins the last one",
			input: "FROM scratch\nRUN --mount=from=nginx:1.27,from=nginx:1.27,target=/a true\n",
			want:  "FROM scratch\nRUN --mount=from=nginx:1.27,from=nginx:1.27@" + d + ",target=/a true\n",
		},
		{
			name:  "source path that looks like the option stays",
			input: "FROM scratch\nRUN --mount=type=bind,source=from=nginx:1.27,from=nginx:1.27,target=/a true\n",
			want:  "FROM scratch\nRUN --mount=type=bind,source=from=nginx:1.27,from=nginx:1.27@" + d + ",target=/a true\n",
		},
		{
			name:  "quoted value that looks like another flag",
			input: "FROM scratch\nRUN --mount=type=bind,source=\"/path --mount=other\",from=nginx:1.27,target=/a --mount=from=busybox:1,target=/b true\n",
			want:  "FROM scratch\nRUN --mount=type=bind,source=\"/path --mount=other\",from=nginx:1.27@" + d + ",target=/a --mount=from=busybox:1@" + d + ",target=/b true\n",
		},
		{
			name:  "other flags between mounts",
			input: "FROM scratch\nRUN --network=none --mount=from=nginx:1.27,target=/a --security=insecure --mount=from=busybox:1,target=/b true\n",
			want:  "FROM scratch\nRUN --network=none --mount=from=nginx:1.27@" + d + ",target=/a --security=insecure --mount=from=busybox:1@" + d + ",target=/b true\n",
		},
		{
			name:  "double-quoted option list",
			input: "FROM scratch\nRUN --mount=\"type=bind,from=nginx:1.27,target=/mnt\" true\n",
			want:  "FROM scratch\nRUN --mount=\"type=bind,from=nginx:1.27@" + d + ",target=/mnt\" true\n",
		},
		{
			name:  "single-quoted option list with from last",
			input: "FROM scratch\nRUN --mount='type=bind,target=/mnt,from=nginx:1.27' true\n",
			want:  "FROM scratch\nRUN --mount='type=bind,target=/mnt,from=nginx:1.27@" + d + "' true\n",
		},
		{
			name:  "individually quoted options",
			input: "FROM scratch\nRUN --mount=\"type=bind\",from=nginx:1.27,target=\"/mnt\" true\n",
			want:  "FROM scratch\nRUN --mount=\"type=bind\",from=nginx:1.27@" + d + ",target=\"/mnt\" true\n",
		},
		{
			name:  "quoted option and quoted value together",
			input: "FROM scratch\nRUN --mount=type=bind,'from=nginx:1.27',target=/a --mount=\"from\"=\"busybox:1\",target=/b true\n",
			want:  "FROM scratch\nRUN --mount=type=bind,'from=nginx:1.27@" + d + "',target=/a --mount=\"from\"=\"busybox:1@" + d + "\",target=/b true\n",
		},
		{
			name:  "comment lines inside a continued RUN",
			input: "FROM scratch\nRUN \\\n    # external configuration from=busybox:1\n    --mount=from=nginx:1.27,target=/a \\\n    # --mount=from=alpine:3,target=/c\n    --mount=from=busybox:1,target=/b \\\n    true\n",
			want:  "FROM scratch\nRUN \\\n    # external configuration from=busybox:1\n    --mount=from=nginx:1.27@" + d + ",target=/a \\\n    # --mount=from=alpine:3,target=/c\n    --mount=from=busybox:1@" + d + ",target=/b \\\n    true\n",
		},
		{
			name:  "ONBUILD RUN",
			input: "FROM scratch\nONBUILD RUN --mount=from=nginx:1.27,target=/a true\n",
			want:  "FROM scratch\nONBUILD RUN --mount=from=nginx:1.27@" + d + ",target=/a true\n",
		},
		{
			name:  "stage and variable are left alone",
			input: "FROM scratch AS base\nRUN --mount=from=base,target=/a --mount=from=img:${TAG},target=/b true\n",
			want:  "FROM scratch AS base\nRUN --mount=from=base,target=/a --mount=from=img:${TAG},target=/b true\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			insts, err := Parse(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			digests := map[int]string{}
			for i := range insts {
				digests[i] = d
			}
			got, unrewritten := RewriteFileReport(tt.input, insts, digests)
			if got != tt.want {
				t.Errorf("RewriteFileReport() =\n%s\nwant\n%s", got, tt.want)
			}
			if len(unrewritten) != 0 {
				t.Errorf("unrewritten = %v, want none", unrewritten)
			}
		})
	}
}
