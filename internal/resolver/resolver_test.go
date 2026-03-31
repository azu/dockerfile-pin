package resolver

import (
	"context"
	"testing"
)

func TestMockResolver_Resolve(t *testing.T) {
	mock := &MockResolver{
		Digests: map[string]string{
			"node:20.11.1":     "sha256:abc123",
			"python:3.12-slim": "sha256:def456",
		},
	}
	ctx := context.Background()

	digest, err := mock.Resolve(ctx, "node:20.11.1")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if digest != "sha256:abc123" {
		t.Errorf("Resolve() = %q, want %q", digest, "sha256:abc123")
	}

	_, err = mock.Resolve(ctx, "unknown:latest")
	if err == nil {
		t.Error("Resolve() expected error for unknown image")
	}
}

func TestCachedResolver_Exists_NonExistent(t *testing.T) {
	mock := &MockResolver{
		Digests: map[string]string{
			"node:20@sha256:abc": "sha256:abc",
		},
	}
	cached := NewCachedResolver(mock)
	ctx := context.Background()

	// First call: image does not exist
	exists, err := cached.Exists(ctx, "node:20@sha256:nonexistent")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() = true, want false")
	}

	// Second call (cache hit): must still return false
	exists, err = cached.Exists(ctx, "node:20@sha256:nonexistent")
	if err != nil {
		t.Fatalf("Exists() cached error = %v", err)
	}
	if exists {
		t.Error("Exists() cached = true, want false (cache returned wrong result)")
	}
}

func TestMockResolver_Exists(t *testing.T) {
	mock := &MockResolver{
		Digests: map[string]string{
			"node:20.11.1@sha256:abc123": "sha256:abc123",
		},
	}
	ctx := context.Background()

	exists, err := mock.Exists(ctx, "node:20.11.1@sha256:abc123")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() = false, want true")
	}

	exists, err = mock.Exists(ctx, "node:20.11.1@sha256:nonexistent")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() = true, want false")
	}
}
