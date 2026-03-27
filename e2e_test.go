package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azu/dockerfile-pin/internal/dockerfile"
	"github.com/azu/dockerfile-pin/internal/resolver"
)

func TestPinEndToEnd(t *testing.T) {
	input := "ARG NODE_VERSION=20.11.1\nFROM node:${NODE_VERSION}\nFROM python:3.12-slim AS builder\nFROM --platform=linux/amd64 golang:1.22\nFROM scratch\nFROM builder AS final\nFROM node:20.11.1@sha256:existingdigest\n"

	mock := &resolver.MockResolver{
		Digests: map[string]string{
			"node:20.11.1":     "sha256:aaa111",
			"python:3.12-slim": "sha256:bbb222",
			"golang:1.22":      "sha256:ccc333",
		},
	}

	instructions, err := dockerfile.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	ctx := context.Background()
	digests := make(map[int]string)
	for i, inst := range instructions {
		if inst.Skip || inst.Digest != "" {
			continue
		}
		digest, err := mock.Resolve(ctx, inst.ImageRef)
		if err != nil {
			t.Logf("skipping %s: %v", inst.ImageRef, err)
			continue
		}
		digests[i] = digest
	}

	result := dockerfile.RewriteFile(input, instructions, digests)

	if !strings.Contains(result, "FROM node:${NODE_VERSION}@sha256:aaa111") {
		t.Error("expected node ARG line to be pinned")
	}
	if !strings.Contains(result, "FROM python:3.12-slim@sha256:bbb222 AS builder") {
		t.Error("expected python line to be pinned with AS clause preserved")
	}
	if !strings.Contains(result, "FROM --platform=linux/amd64 golang:1.22@sha256:ccc333") {
		t.Error("expected golang line to be pinned with platform preserved")
	}
	if !strings.Contains(result, "FROM scratch") {
		t.Error("scratch should be preserved")
	}
	if !strings.Contains(result, "FROM builder AS final") {
		t.Error("stage reference should be preserved")
	}
	if !strings.Contains(result, "FROM node:20.11.1@sha256:existingdigest") {
		t.Error("already-pinned line should be preserved without --update")
	}
}

func TestCheckEndToEnd(t *testing.T) {
	input := "FROM node:20.11.1\nFROM python:3.12-slim@sha256:validdigest\nFROM golang:1.22@sha256:invaliddigest\nFROM scratch\n"

	mock := &resolver.MockResolver{
		Digests: map[string]string{
			"python:3.12-slim@sha256:validdigest": "sha256:validdigest",
		},
	}

	instructions, err := dockerfile.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	ctx := context.Background()
	type checkResult struct {
		imageRef string
		status   string
	}
	var results []checkResult

	for _, inst := range instructions {
		if inst.Skip {
			results = append(results, checkResult{inst.ImageRef, "skip"})
			continue
		}
		if inst.Digest == "" {
			results = append(results, checkResult{inst.ImageRef, "fail-missing"})
			continue
		}
		fullRef := inst.ImageRef + "@" + inst.Digest
		exists, _ := mock.Exists(ctx, fullRef)
		if exists {
			results = append(results, checkResult{inst.ImageRef, "ok"})
		} else {
			results = append(results, checkResult{inst.ImageRef, "fail-notfound"})
		}
	}

	expected := []checkResult{
		{"node:20.11.1", "fail-missing"},
		{"python:3.12-slim", "ok"},
		{"golang:1.22", "fail-notfound"},
		{"scratch", "skip"},
	}

	if len(results) != len(expected) {
		t.Fatalf("expected %d results, got %d", len(expected), len(results))
	}
	for i, want := range expected {
		if results[i] != want {
			t.Errorf("[%d] got %+v, want %+v", i, results[i], want)
		}
	}
}

func TestPinFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Dockerfile")
	content := "FROM alpine:3.19\nRUN echo hello\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	instructions, err := dockerfile.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	digests := map[int]string{0: "sha256:testdigest123"}
	result := dockerfile.RewriteFile(content, instructions, digests)

	if err := os.WriteFile(path, []byte(result), 0644); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(written), "FROM alpine:3.19@sha256:testdigest123") {
		t.Errorf("round-trip failed: %s", string(written))
	}
	if !strings.Contains(string(written), "RUN echo hello") {
		t.Error("non-FROM lines should be preserved")
	}
}

func TestRealWorldPatterns(t *testing.T) {
	// Test all patterns from real production codebase
	input := "FROM golang:1.26.1-trixie AS builder\nFROM gcr.io/distroless/static:01b9ed74ee38468719506f73b50d7bd8e596c37b\nFROM node:24.14.0-bookworm-slim AS build\nFROM ghcr.io/astral-sh/uv:0.10.9-python3.13-trixie AS uv-builder\nFROM python:3.13-slim-trixie AS runtime\nFROM ubuntu\nFROM headscale/headscale:latest\nFROM registry.example.com:5000/myapp:1.0\nFROM postgres:16.6-bookworm\nFROM debian:bookworm-20250407-slim\nFROM gcr.io/distroless/static-debian12:nonroot\n"

	instructions, err := dockerfile.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// All should be parseable and not skipped
	for i, inst := range instructions {
		if inst.Skip {
			t.Errorf("[%d] %q should not be skipped (reason: %s)", i, inst.ImageRef, inst.SkipReason)
		}
	}

	// Verify specific patterns
	expectations := []struct {
		idx      int
		imageRef string
	}{
		{0, "golang:1.26.1-trixie"},
		{1, "gcr.io/distroless/static:01b9ed74ee38468719506f73b50d7bd8e596c37b"},
		{2, "node:24.14.0-bookworm-slim"},
		{3, "ghcr.io/astral-sh/uv:0.10.9-python3.13-trixie"},
		{4, "python:3.13-slim-trixie"},
		{5, "ubuntu"},
		{6, "headscale/headscale:latest"},
		{7, "registry.example.com:5000/myapp:1.0"},
		{8, "postgres:16.6-bookworm"},
		{9, "debian:bookworm-20250407-slim"},
		{10, "gcr.io/distroless/static-debian12:nonroot"},
	}

	if len(instructions) != len(expectations) {
		t.Fatalf("expected %d instructions, got %d", len(expectations), len(instructions))
	}

	for _, e := range expectations {
		if instructions[e.idx].ImageRef != e.imageRef {
			t.Errorf("[%d] ImageRef = %q, want %q", e.idx, instructions[e.idx].ImageRef, e.imageRef)
		}
	}
}
