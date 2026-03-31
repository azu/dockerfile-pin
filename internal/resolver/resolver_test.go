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

func TestCachedResolver_Exists_DeduplicatesCalls(t *testing.T) {
	counting := &CountingResolver{
		inner: &MockResolver{
			Digests: map[string]string{
				"node:20@sha256:abc":   "sha256:abc",
				"python:3.12@sha256:x": "sha256:x",
			},
		},
	}
	cached := NewCachedResolver(counting)
	ctx := context.Background()

	// Simulate a real-world scenario: same image appears in multiple Dockerfiles
	refs := []string{
		"node:20@sha256:abc",      // Dockerfile 1
		"python:3.12@sha256:x",    // Dockerfile 2
		"node:20@sha256:abc",      // Dockerfile 3 (duplicate)
		"node:20@sha256:abc",      // Dockerfile 4 (duplicate)
		"python:3.12@sha256:x",    // Dockerfile 5 (duplicate)
		"gone:1@sha256:nope",      // Dockerfile 6 (non-existent)
		"gone:1@sha256:nope",      // Dockerfile 7 (non-existent, duplicate)
	}
	for _, ref := range refs {
		cached.Exists(ctx, ref)
	}

	// 3 unique refs → inner should be called exactly 3 times
	if counting.existsCalls != 3 {
		t.Errorf("inner.Exists called %d times, want 3 (once per unique ref)", counting.existsCalls)
	}

	// Verify cached results are correct
	exists, _ := cached.Exists(ctx, "node:20@sha256:abc")
	if !exists {
		t.Error("cached Exists(node) = false, want true")
	}
	exists, _ = cached.Exists(ctx, "gone:1@sha256:nope")
	if exists {
		t.Error("cached Exists(gone) = true, want false")
	}

	// Still 3 calls — the verifications above hit cache
	if counting.existsCalls != 3 {
		t.Errorf("inner.Exists called %d times after verification, want 3", counting.existsCalls)
	}
}

// CountingResolver wraps a DigestResolver and counts calls.
type CountingResolver struct {
	inner        DigestResolver
	resolveCalls int
	existsCalls  int
}

func (r *CountingResolver) Resolve(ctx context.Context, imageRef string) (string, error) {
	r.resolveCalls++
	return r.inner.Resolve(ctx, imageRef)
}

func (r *CountingResolver) Exists(ctx context.Context, imageRef string) (bool, error) {
	r.existsCalls++
	return r.inner.Exists(ctx, imageRef)
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
