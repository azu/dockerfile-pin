package resolver

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type DigestResolver interface {
	Resolve(ctx context.Context, imageRef string) (string, error)
	Exists(ctx context.Context, imageRef string) (bool, error)
}

const perRequestTimeout = 30 * time.Second

type CraneResolver struct{}

func (r *CraneResolver) Resolve(ctx context.Context, imageRef string) (string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("parsing reference %q: %w", imageRef, err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, perRequestTimeout)
	defer cancel()
	desc, err := remote.Head(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithContext(reqCtx))
	if err != nil {
		return "", fmt.Errorf("resolving digest for %q: %w", imageRef, err)
	}
	return desc.Digest.String(), nil
}

func (r *CraneResolver) Exists(ctx context.Context, imageRef string) (bool, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return false, fmt.Errorf("parsing reference %q: %w", imageRef, err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, perRequestTimeout)
	defer cancel()
	_, err = remote.Head(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithContext(reqCtx))
	if err != nil {
		return false, nil
	}
	return true, nil
}

// CachedResolver wraps a DigestResolver with an in-memory cache.
// Resolve and Exists use separate caches to avoid cross-method interference.
// Safe for concurrent use.
type CachedResolver struct {
	inner        DigestResolver
	mu           sync.RWMutex
	resolveCache map[string]resolveEntry
	existsCache  map[string]existsEntry
}

type resolveEntry struct {
	digest string
	err    error
}

type existsEntry struct {
	exists bool
	err    error
}

func NewCachedResolver(inner DigestResolver) *CachedResolver {
	return &CachedResolver{
		inner:        inner,
		resolveCache: make(map[string]resolveEntry),
		existsCache:  make(map[string]existsEntry),
	}
}

func (r *CachedResolver) Resolve(ctx context.Context, imageRef string) (string, error) {
	r.mu.RLock()
	entry, ok := r.resolveCache[imageRef]
	r.mu.RUnlock()
	if ok {
		return entry.digest, entry.err
	}

	digest, err := r.inner.Resolve(ctx, imageRef)

	r.mu.Lock()
	r.resolveCache[imageRef] = resolveEntry{digest: digest, err: err}
	r.mu.Unlock()

	return digest, err
}

func (r *CachedResolver) Exists(ctx context.Context, imageRef string) (bool, error) {
	r.mu.RLock()
	entry, ok := r.existsCache[imageRef]
	r.mu.RUnlock()
	if ok {
		return entry.exists, entry.err
	}

	exists, err := r.inner.Exists(ctx, imageRef)

	r.mu.Lock()
	r.existsCache[imageRef] = existsEntry{exists: exists, err: err}
	r.mu.Unlock()

	return exists, err
}

type MockResolver struct {
	Digests map[string]string
}

func (r *MockResolver) Resolve(_ context.Context, imageRef string) (string, error) {
	digest, ok := r.Digests[imageRef]
	if !ok {
		return "", fmt.Errorf("unknown image: %s", imageRef)
	}
	return digest, nil
}

func (r *MockResolver) Exists(_ context.Context, imageRef string) (bool, error) {
	_, ok := r.Digests[imageRef]
	return ok, nil
}
