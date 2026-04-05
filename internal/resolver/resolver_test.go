package resolver

import (
	"context"
	"errors"
	"testing"
)

// failingResolver returns an error for the first `failCount` calls to Resolve/Exists,
// then returns the configured digest/exists value.
type failingResolver struct {
	resolveCalls int
	existsCalls  int
	failCount    int
	digest       string
}

func (r *failingResolver) Resolve(_ context.Context, _ string) (string, error) {
	r.resolveCalls++
	if r.resolveCalls <= r.failCount {
		return "", errors.New("transient network error")
	}
	return r.digest, nil
}

func (r *failingResolver) Exists(_ context.Context, _ string) (bool, error) {
	r.existsCalls++
	if r.existsCalls <= r.failCount {
		return false, errors.New("transient network error")
	}
	return true, nil
}

// TestCachedResolver_DoesNotCacheResolveErrors verifies that a transient error from
// Resolve is not stored in the cache so the next call retries the inner resolver.
func TestCachedResolver_DoesNotCacheResolveErrors(t *testing.T) {
	inner := &failingResolver{failCount: 1, digest: "sha256:abc123"}
	cached := NewCachedResolver(inner)
	ctx := context.Background()

	// First call: inner fails — should propagate the error.
	_, err := cached.Resolve(ctx, "node:20")
	if err == nil {
		t.Fatal("expected error on first call, got nil")
	}

	// Second call: inner succeeds now — must NOT be short-circuited by a cached error.
	digest, err := cached.Resolve(ctx, "node:20")
	if err != nil {
		t.Fatalf("expected success on second call, got: %v", err)
	}
	if digest != "sha256:abc123" {
		t.Errorf("expected sha256:abc123, got %q", digest)
	}

	// Third call: result is now cached; inner must not be called again.
	callsBefore := inner.resolveCalls
	digest2, err := cached.Resolve(ctx, "node:20")
	if err != nil {
		t.Fatalf("unexpected error on third call: %v", err)
	}
	if digest2 != digest {
		t.Errorf("expected same cached digest, got %q", digest2)
	}
	if inner.resolveCalls != callsBefore {
		t.Errorf("inner called %d extra times, want 0", inner.resolveCalls-callsBefore)
	}
}

// TestCachedResolver_DoesNotCacheExistsErrors verifies that a transient error from
// Exists is not stored in the cache so the next call retries the inner resolver.
func TestCachedResolver_DoesNotCacheExistsErrors(t *testing.T) {
	inner := &failingResolver{failCount: 1}
	cached := NewCachedResolver(inner)
	ctx := context.Background()

	// First call: inner fails — should propagate the error.
	_, err := cached.Exists(ctx, "node:20")
	if err == nil {
		t.Fatal("expected error on first call, got nil")
	}

	// Second call: inner succeeds now — must NOT be short-circuited by a cached error.
	exists, err := cached.Exists(ctx, "node:20")
	if err != nil {
		t.Fatalf("expected success on second call, got: %v", err)
	}
	if !exists {
		t.Error("expected exists=true on second call")
	}

	// Third call: result is now cached; inner must not be called again.
	callsBefore := inner.existsCalls
	_, err = cached.Exists(ctx, "node:20")
	if err != nil {
		t.Fatalf("unexpected error on third call: %v", err)
	}
	if inner.existsCalls != callsBefore {
		t.Errorf("inner called %d extra times, want 0", inner.existsCalls-callsBefore)
	}
}

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
		"node:20@sha256:abc",   // Dockerfile 1
		"python:3.12@sha256:x", // Dockerfile 2
		"node:20@sha256:abc",   // Dockerfile 3 (duplicate)
		"node:20@sha256:abc",   // Dockerfile 4 (duplicate)
		"python:3.12@sha256:x", // Dockerfile 5 (duplicate)
		"gone:1@sha256:nope",   // Dockerfile 6 (non-existent)
		"gone:1@sha256:nope",   // Dockerfile 7 (non-existent, duplicate)
	}
	for _, ref := range refs {
		_, _ = cached.Exists(ctx, ref)
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

func TestCachedResolver_CrossMethodNoInterference(t *testing.T) {
	mock := &MockResolver{
		Digests: map[string]string{
			"node:20": "sha256:abc",
		},
	}
	cached := NewCachedResolver(mock)
	ctx := context.Background()

	// Resolve first, then Exists on the same key must still work
	digest, err := cached.Resolve(ctx, "node:20")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if digest != "sha256:abc" {
		t.Errorf("Resolve() = %q, want %q", digest, "sha256:abc")
	}

	exists, err := cached.Exists(ctx, "node:20")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() after Resolve() = false, want true")
	}

	// Exists first on a different key, then Resolve
	exists, err = cached.Exists(ctx, "python:3.12")
	if err != nil {
		t.Fatalf("Exists(python) error = %v", err)
	}
	if exists {
		t.Error("Exists(python) = true, want false (not in mock)")
	}

	_, err = cached.Resolve(ctx, "python:3.12")
	if err == nil {
		t.Error("Resolve(python) after Exists() should error for unknown image")
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
