package dockerfile

import "testing"

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
		{
			name:     "COPY --from simple tag",
			original: "COPY --from=nginx:alpine /etc/nginx /etc/nginx",
			rawRef:   "nginx:alpine",
			digest:   "sha256:abc123",
			want:     "COPY --from=nginx:alpine@sha256:abc123 /etc/nginx /etc/nginx",
		},
		{
			name:     "COPY --from update existing digest",
			original: "COPY --from=nginx:alpine@sha256:olddigest /etc/nginx /etc/nginx",
			rawRef:   "nginx:alpine@sha256:olddigest",
			digest:   "sha256:newdigest",
			want:     "COPY --from=nginx:alpine@sha256:newdigest /etc/nginx /etc/nginx",
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

func TestRewriteFile_WithCopyFrom(t *testing.T) {
	content := "FROM golang:1.22 AS builder\nCOPY --from=builder /app /app\nCOPY --from=nginx:alpine /etc/nginx /etc/nginx\nFROM debian:bookworm-slim\n"
	instructions := []FromInstruction{
		{ImageRef: "golang:1.22", RawRef: "golang:1.22", StartLine: 1},
		{ImageRef: "builder", RawRef: "builder", StartLine: 2, IsCopyFrom: true, Skip: true, SkipReason: "stage reference"},
		{ImageRef: "nginx:alpine", RawRef: "nginx:alpine", StartLine: 3, IsCopyFrom: true},
		{ImageRef: "debian:bookworm-slim", RawRef: "debian:bookworm-slim", StartLine: 4},
	}
	digests := map[int]string{
		0: "sha256:golang111",
		2: "sha256:nginx222",
		3: "sha256:debian333",
	}
	got := RewriteFile(content, instructions, digests)
	want := "FROM golang:1.22@sha256:golang111 AS builder\nCOPY --from=builder /app /app\nCOPY --from=nginx:alpine@sha256:nginx222 /etc/nginx /etc/nginx\nFROM debian:bookworm-slim@sha256:debian333\n"
	if got != want {
		t.Errorf("RewriteFile() =\n%s\nwant:\n%s", got, want)
	}
}
