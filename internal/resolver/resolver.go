package resolver

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type DigestResolver interface {
	Resolve(ctx context.Context, imageRef string) (string, error)
	Exists(ctx context.Context, imageRef string) (bool, error)
}

type CraneResolver struct{}

func (r *CraneResolver) Resolve(ctx context.Context, imageRef string) (string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("parsing reference %q: %w", imageRef, err)
	}
	desc, err := remote.Head(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithContext(ctx))
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
	_, err = remote.Head(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithContext(ctx))
	if err != nil {
		return false, nil
	}
	return true, nil
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
