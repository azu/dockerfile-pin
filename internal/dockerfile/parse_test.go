package dockerfile

import (
	"strings"
	"testing"
)

func TestParse_BasicFromLines(t *testing.T) {
	input := "FROM node:20.11.1\nFROM python:3.12-slim\nFROM golang:1.22\n"
	instructions, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(instructions) != 3 {
		t.Fatalf("expected 3 instructions, got %d", len(instructions))
	}
	tests := []struct {
		idx      int
		imageRef string
		digest   string
		skip     bool
	}{
		{0, "node:20.11.1", "", false},
		{1, "python:3.12-slim", "", false},
		{2, "golang:1.22", "", false},
	}
	for _, tt := range tests {
		inst := instructions[tt.idx]
		if inst.ImageRef != tt.imageRef {
			t.Errorf("[%d] ImageRef = %q, want %q", tt.idx, inst.ImageRef, tt.imageRef)
		}
		if inst.Digest != tt.digest {
			t.Errorf("[%d] Digest = %q, want %q", tt.idx, inst.Digest, tt.digest)
		}
		if inst.Skip != tt.skip {
			t.Errorf("[%d] Skip = %v, want %v", tt.idx, inst.Skip, tt.skip)
		}
	}
}

func TestParse_MultiStage(t *testing.T) {
	input := "FROM golang:1.22 AS builder\nFROM --platform=linux/amd64 debian:bookworm-slim AS runtime\nFROM scratch\nFROM builder AS final\n"
	instructions, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(instructions) != 4 {
		t.Fatalf("expected 4 instructions, got %d", len(instructions))
	}
	if instructions[0].ImageRef != "golang:1.22" {
		t.Errorf("[0] ImageRef = %q, want %q", instructions[0].ImageRef, "golang:1.22")
	}
	if instructions[0].StageName != "builder" {
		t.Errorf("[0] StageName = %q, want %q", instructions[0].StageName, "builder")
	}
	if instructions[0].Skip {
		t.Error("[0] should not be skipped")
	}
	if instructions[1].ImageRef != "debian:bookworm-slim" {
		t.Errorf("[1] ImageRef = %q, want %q", instructions[1].ImageRef, "debian:bookworm-slim")
	}
	if instructions[1].Platform != "linux/amd64" {
		t.Errorf("[1] Platform = %q, want %q", instructions[1].Platform, "linux/amd64")
	}
	if instructions[1].StageName != "runtime" {
		t.Errorf("[1] StageName = %q, want %q", instructions[1].StageName, "runtime")
	}
	if !instructions[2].Skip {
		t.Error("[2] scratch should be skipped")
	}
	if instructions[2].SkipReason != "scratch image" {
		t.Errorf("[2] SkipReason = %q, want %q", instructions[2].SkipReason, "scratch image")
	}
	if !instructions[3].Skip {
		t.Error("[3] stage reference should be skipped")
	}
	if instructions[3].SkipReason != "stage reference" {
		t.Errorf("[3] SkipReason = %q, want %q", instructions[3].SkipReason, "stage reference")
	}
}

func TestParse_AlreadyPinned(t *testing.T) {
	input := "FROM node:20.11.1@sha256:d938c1761e3afbae9242848ffbb95b9cc1cb0a24d889f8bd955204d347a7266e\n"
	instructions, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(instructions) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(instructions))
	}
	if instructions[0].ImageRef != "node:20.11.1" {
		t.Errorf("ImageRef = %q, want %q", instructions[0].ImageRef, "node:20.11.1")
	}
	if instructions[0].Digest != "sha256:d938c1761e3afbae9242848ffbb95b9cc1cb0a24d889f8bd955204d347a7266e" {
		t.Errorf("Digest = %q", instructions[0].Digest)
	}
}

func TestParse_ArgExpansion(t *testing.T) {
	input := "ARG NODE_VERSION=20.11.1\nFROM node:${NODE_VERSION}\n"
	instructions, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(instructions) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(instructions))
	}
	if instructions[0].ImageRef != "node:20.11.1" {
		t.Errorf("ImageRef = %q, want %q", instructions[0].ImageRef, "node:20.11.1")
	}
	if instructions[0].RawRef != "node:${NODE_VERSION}" {
		t.Errorf("RawRef = %q, want %q", instructions[0].RawRef, "node:${NODE_VERSION}")
	}
	if instructions[0].Skip {
		t.Error("should not be skipped (ARG has default value)")
	}
}

func TestParse_ArgNoDefault(t *testing.T) {
	input := "ARG BASE_IMAGE\nFROM ${BASE_IMAGE}\n"
	instructions, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(instructions) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(instructions))
	}
	if !instructions[0].Skip {
		t.Error("should be skipped (ARG has no default)")
	}
	if instructions[0].SkipReason != "unresolved ARG variable" {
		t.Errorf("SkipReason = %q", instructions[0].SkipReason)
	}
}

func TestParse_PlatformVariable(t *testing.T) {
	input := "FROM --platform=$BUILDPLATFORM golang:1.22\n"
	instructions, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(instructions) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(instructions))
	}
	if instructions[0].ImageRef != "golang:1.22" {
		t.Errorf("ImageRef = %q, want %q", instructions[0].ImageRef, "golang:1.22")
	}
	if instructions[0].Skip {
		t.Error("should not be skipped (only platform is variable)")
	}
}

func TestExpandVars(t *testing.T) {
	defaults := map[string]string{"VERSION": "3.12", "REG": "ghcr.io"}
	tests := []struct {
		input      string
		want       string
		unresolved bool
	}{
		{"python:${VERSION}", "python:3.12", false},
		{"${REG}/app:latest", "ghcr.io/app:latest", false},
		{"${UNKNOWN}/app", "${UNKNOWN}/app", true},
		{"node:20", "node:20", false},
		{"$VERSION-slim", "3.12-slim", false},
	}
	for _, tt := range tests {
		got, unresolved := expandVars(tt.input, defaults)
		if got != tt.want {
			t.Errorf("expandVars(%q) = %q, want %q", tt.input, got, tt.want)
		}
		if unresolved != tt.unresolved {
			t.Errorf("expandVars(%q) unresolved = %v, want %v", tt.input, unresolved, tt.unresolved)
		}
	}
}

// wantInst is the expected shape of one parsed instruction.
type wantInst struct {
	imageRef   string
	rawRef     string
	digest     string
	isCopyFrom bool
	skip       bool
	skipReason string
	startLine  int
}

func checkInstructions(t *testing.T, input string, want []wantInst) {
	t.Helper()
	got, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d instructions, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		inst := got[i]
		if inst.ImageRef != w.imageRef {
			t.Errorf("[%d] ImageRef = %q, want %q", i, inst.ImageRef, w.imageRef)
		}
		if w.rawRef != "" && inst.RawRef != w.rawRef {
			t.Errorf("[%d] RawRef = %q, want %q", i, inst.RawRef, w.rawRef)
		}
		if inst.Digest != w.digest {
			t.Errorf("[%d] Digest = %q, want %q", i, inst.Digest, w.digest)
		}
		if inst.IsCopyFrom != w.isCopyFrom {
			t.Errorf("[%d] IsCopyFrom = %v, want %v", i, inst.IsCopyFrom, w.isCopyFrom)
		}
		if inst.Skip != w.skip {
			t.Errorf("[%d] Skip = %v, want %v (reason %q)", i, inst.Skip, w.skip, inst.SkipReason)
		}
		if inst.SkipReason != w.skipReason {
			t.Errorf("[%d] SkipReason = %q, want %q", i, inst.SkipReason, w.skipReason)
		}
		if w.startLine != 0 && inst.StartLine != w.startLine {
			t.Errorf("[%d] StartLine = %d, want %d", i, inst.StartLine, w.startLine)
		}
	}
}

// TestParse_CopyFrom covers how each form of `COPY --from=` is classified. The
// reference forms follow the Dockerfile reference, which defines --from as naming
// "an image, a build stage, or a named context".
func TestParse_CopyFrom(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []wantInst
	}{
		{
			// The reproduction from the feature request.
			name:  "external image with tag alongside a stage index",
			input: "FROM ubuntu:24.04\nCOPY --from=nginx:1.27 /etc/nginx /etc/nginx\nCOPY --from=0 /a /b\n",
			want: []wantInst{
				{imageRef: "ubuntu:24.04", rawRef: "ubuntu:24.04", startLine: 1},
				{imageRef: "nginx:1.27", rawRef: "nginx:1.27", isCopyFrom: true, startLine: 2},
				{imageRef: "0", rawRef: "0", isCopyFrom: true, skip: true, skipReason: SkipStageIndex, startLine: 3},
			},
		},
		{
			name:  "stage declared above",
			input: "FROM golang:1.22 AS builder\nCOPY --from=builder /app /app\n",
			want: []wantInst{
				{imageRef: "golang:1.22", startLine: 1},
				{imageRef: "builder", isCopyFrom: true, skip: true, skipReason: SkipStageRef, startLine: 2},
			},
		},
		{
			// BuildKit looks stage names up with strings.ToLower, so case does not matter.
			name:  "stage name match is case-insensitive",
			input: "FROM golang:1.22 AS Builder\nCOPY --from=BUILDER /app /app\n",
			want: []wantInst{
				{imageRef: "golang:1.22", startLine: 1},
				{imageRef: "BUILDER", isCopyFrom: true, skip: true, skipReason: SkipStageRef, startLine: 2},
			},
		},
		{
			// Naming a stage declared below is a build error about stage order
			// ("cannot copy from stage ... it needs to be defined before current
			// stage"), not an unpinned image: the name is still a stage, so there is
			// nothing here to rewrite.
			name:  "stage declared below",
			input: "FROM alpine:3.19 AS base\nCOPY --from=late /bin/tool /usr/local/bin/tool\nFROM ubuntu:24.04 AS late\n",
			want: []wantInst{
				{imageRef: "alpine:3.19", startLine: 1},
				{imageRef: "late", isCopyFrom: true, skip: true, skipReason: SkipStageRef, startLine: 2},
				{imageRef: "ubuntu:24.04", startLine: 3},
			},
		},
		{
			name:  "already pinned",
			input: "COPY --from=nginx:1.27@sha256:abc123 /etc/nginx /etc/nginx\n",
			want: []wantInst{
				{imageRef: "nginx:1.27", rawRef: "nginx:1.27@sha256:abc123", digest: "sha256:abc123", isCopyFrom: true},
			},
		},
		{
			name:  "pinned without a tag",
			input: "COPY --from=nginx@sha256:abc123 /etc/nginx /etc/nginx\n",
			want: []wantInst{
				{imageRef: "nginx", rawRef: "nginx@sha256:abc123", digest: "sha256:abc123", isCopyFrom: true},
			},
		},
		{
			// CopyCommand.Expand expands --chown, --chmod and the paths but not --from,
			// so BuildKit reads the value verbatim and the build fails to parse the
			// stage name. FROM on the same file still expands, which is the asymmetry
			// this case pins down.
			name:  "variable in --from is not expanded, unlike FROM",
			input: "ARG NGINX_VERSION=1.27\nFROM nginx:${NGINX_VERSION}\nCOPY --from=nginx:${NGINX_VERSION} /etc/nginx /etc/nginx\n",
			want: []wantInst{
				{imageRef: "nginx:1.27", rawRef: "nginx:${NGINX_VERSION}", startLine: 2},
				{imageRef: "nginx:${NGINX_VERSION}", isCopyFrom: true, skip: true, skipReason: SkipCopyFromVar, startLine: 3},
			},
		},
		{
			name:  "bare $VAR in --from is not expanded either",
			input: "ARG NGINX_IMAGE=nginx:1.27\nFROM ubuntu:24.04\nCOPY --from=$NGINX_IMAGE /etc/nginx /etc/nginx\n",
			want: []wantInst{
				{imageRef: "ubuntu:24.04", startLine: 2},
				{imageRef: "$NGINX_IMAGE", isCopyFrom: true, skip: true, skipReason: SkipCopyFromVar, startLine: 3},
			},
		},
		{
			name:  "scratch has nothing to copy from a registry",
			input: "COPY --from=scratch /a /b\n",
			want: []wantInst{
				{imageRef: "scratch", isCopyFrom: true, skip: true, skipReason: SkipScratch},
			},
		},
		{
			name:  "registry with a port",
			input: "COPY --from=registry.example.com:5000/tool:1.0 /tool /tool\n",
			want: []wantInst{
				{imageRef: "registry.example.com:5000/tool:1.0", isCopyFrom: true},
			},
		},
		{
			name:  "surrounded by other COPY flags",
			input: "COPY --chown=65532:65532 --from=gcr.io/distroless/base:nonroot --chmod=755 /etc/passwd /etc/passwd\n",
			want: []wantInst{
				{imageRef: "gcr.io/distroless/base:nonroot", isCopyFrom: true},
			},
		},
		{
			name:  "boolean flags do not hide --from",
			input: "COPY --link --parents --from=busybox:1.36 /bin/ /bin/\n",
			want: []wantInst{
				{imageRef: "busybox:1.36", isCopyFrom: true},
			},
		},
		{
			// A bare name that matches no stage is an image to BuildKit, which is also
			// how `FROM ubuntu` is treated. A named build context supplied at build time
			// (--build-context name=...) is written the same way and cannot be told
			// apart from the Dockerfile alone; --ignore-images excludes those.
			name:  "bare name that matches no stage is an image",
			input: "COPY --from=ubuntu /etc/os-release /etc/os-release\n",
			want: []wantInst{
				{imageRef: "ubuntu", isCopyFrom: true},
			},
		},
		{
			name:  "instruction keyword is case-insensitive",
			input: "copy --from=nginx:1.27 /etc/nginx /etc/nginx\n",
			want: []wantInst{
				{imageRef: "nginx:1.27", isCopyFrom: true},
			},
		},
		{
			name:  "several COPY instructions from the same image",
			input: "COPY --from=nginx:1.27 /etc/nginx /etc/nginx\nCOPY --from=nginx:1.27 /usr/share/nginx /usr/share/nginx\n",
			want: []wantInst{
				{imageRef: "nginx:1.27", isCopyFrom: true, startLine: 1},
				{imageRef: "nginx:1.27", isCopyFrom: true, startLine: 2},
			},
		},
		{
			name:  "plain COPY reads the build context",
			input: "FROM alpine:3.19\nCOPY /src /dst\nCOPY . .\n",
			want: []wantInst{
				{imageRef: "alpine:3.19", startLine: 1},
			},
		},
		{
			// BuildKit matches flag names case-sensitively, so --FROM is an unknown
			// flag and `docker build` rejects the Dockerfile outright.
			name:  "--FROM is not the --from flag",
			input: "COPY --FROM=nginx:1.27 /a /b\n",
			want:  nil,
		},
		{
			// ADD accepts --keep-git-dir, --checksum, --chmod, --chown, --link,
			// --unpack and --exclude; --from belongs to COPY alone.
			name:  "ADD has no --from flag",
			input: "ADD --from=nginx:1.27 /a /b\n",
			want:  nil,
		},
		{
			// The image is named inside the --mount value, not by a --from flag.
			name:  "RUN --mount=from= is a different flag",
			input: "RUN --mount=type=bind,from=nginx:1.27,source=/a,target=/b true\n",
			want:  nil,
		},
		{
			name:  "empty --from value",
			input: "COPY --from= /a /b\n",
			want: []wantInst{
				{imageRef: "", isCopyFrom: true, skip: true, skipReason: SkipMissingRef},
			},
		},
		{
			// BuildKit classifies the value with strconv.Atoi before anything else, so
			// every value it accepts selects a stage by position.
			name:  "a signed number is still a stage index",
			input: "COPY --from=-1 /a /b\n",
			want: []wantInst{
				{imageRef: "-1", isCopyFrom: true, skip: true, skipReason: SkipStageIndex},
			},
		},
		{
			name:  "leading zeros are still a stage index",
			input: "COPY --from=007 /a /b\n",
			want: []wantInst{
				{imageRef: "007", isCopyFrom: true, skip: true, skipReason: SkipStageIndex},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkInstructions(t, tt.input, tt.want)
		})
	}
}

// TestParse_CopyFromLineSpan records the line range of an instruction continued
// with "\", which is what the rewriter searches for the reference.
func TestParse_CopyFromLineSpan(t *testing.T) {
	input := "FROM ubuntu:24.04\nCOPY \\\n  --from=nginx:1.27 \\\n  /etc/nginx /etc/nginx\n"
	instructions, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(instructions) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(instructions))
	}
	copyInst := instructions[1]
	if copyInst.ImageRef != "nginx:1.27" {
		t.Errorf("ImageRef = %q, want %q", copyInst.ImageRef, "nginx:1.27")
	}
	if copyInst.StartLine != 2 {
		t.Errorf("StartLine = %d, want 2", copyInst.StartLine)
	}
	if copyInst.EndLine != 4 {
		t.Errorf("EndLine = %d, want 4", copyInst.EndLine)
	}
}

// TestParse_ARGScopeIsSequential guards the scoping rule that an ARG below a FROM
// belongs to the stage that FROM opens, so it cannot expand that FROM's own ref.
func TestParse_ARGScopeIsSequential(t *testing.T) {
	input := "FROM python:${VERSION}\nARG VERSION=3.12\nCOPY --from=nginx:1.27 /a /b\n"
	checkInstructions(t, input, []wantInst{
		{imageRef: "python:${VERSION}", skip: true, skipReason: SkipUnresolvedARG, startLine: 1},
		{imageRef: "nginx:1.27", isCopyFrom: true, startLine: 3},
	})
}

// TestParse_FromStageReferenceStaysSequential guards the FROM rule that differs from
// COPY: BuildKit resolves a FROM against the stages declared above it, so a name
// defined later is an image, not a stage reference.
func TestParse_FromStageReferenceStaysSequential(t *testing.T) {
	input := "FROM builder\nFROM golang:1.22 AS builder\nCOPY --from=builder /app /app\n"
	checkInstructions(t, input, []wantInst{
		{imageRef: "builder", startLine: 1},
		{imageRef: "golang:1.22", startLine: 2},
		{imageRef: "builder", isCopyFrom: true, skip: true, skipReason: SkipStageRef, startLine: 3},
	})
}

// TestParse_ScratchThroughARG covers scratch reached by variable expansion. A FROM
// expands to it and is recognised; the COPY value is never expanded, so it is held
// back for that reason instead.
func TestParse_ScratchThroughARG(t *testing.T) {
	input := "ARG BASE=scratch\nFROM ${BASE}\nCOPY --from=${BASE} /a /b\n"
	checkInstructions(t, input, []wantInst{
		{imageRef: "scratch", rawRef: "${BASE}", skip: true, skipReason: SkipScratch, startLine: 2},
		{imageRef: "${BASE}", rawRef: "${BASE}", isCopyFrom: true, skip: true, skipReason: SkipCopyFromVar, startLine: 3},
	})
}

func TestIsStageIndex(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"0", true},
		{"12", true},
		{"007", true},
		{"-1", true},
		{"+1", true},
		{"", false},
		{"1.0", false},
		{"nginx", false},
		{"nginx:1.27", false},
		{"1nginx", false},
	}
	for _, tt := range tests {
		if got := isStageIndex(tt.in); got != tt.want {
			t.Errorf("isStageIndex(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
