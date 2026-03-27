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
