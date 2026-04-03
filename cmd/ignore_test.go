package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsIgnored(t *testing.T) {
	tests := []struct {
		name     string
		imageRef string
		patterns []string
		want     bool
	}{
		{
			name:     "exact match",
			imageRef: "scratch",
			patterns: []string{"scratch"},
			want:     true,
		},
		{
			name:     "exact match with tag does not match bare name",
			imageRef: "scratch:latest",
			patterns: []string{"scratch"},
			want:     false,
		},
		{
			name:     "glob star matches single segment",
			imageRef: "ghcr.io/myorg/app:latest",
			patterns: []string{"ghcr.io/myorg/*"},
			want:     true,
		},
		{
			name:     "glob star does not cross slash",
			imageRef: "ghcr.io/myorg/sub/app:latest",
			patterns: []string{"ghcr.io/myorg/*"},
			want:     false,
		},
		{
			name:     "doublestar crosses slash",
			imageRef: "ghcr.io/myorg/sub/app:latest",
			patterns: []string{"ghcr.io/myorg/**"},
			want:     true,
		},
		{
			name:     "ECR pattern with wildcards",
			imageRef: "123456789.dkr.ecr.us-east-1.amazonaws.com/myapp:v1",
			patterns: []string{"*.dkr.ecr.*.amazonaws.com/*"},
			want:     true,
		},
		{
			name:     "negation overrides previous match",
			imageRef: "ghcr.io/myorg/public-app:v1",
			patterns: []string{"ghcr.io/myorg/*", "!ghcr.io/myorg/public-*"},
			want:     false,
		},
		{
			name:     "negation does not affect non-matching",
			imageRef: "ghcr.io/myorg/internal:v1",
			patterns: []string{"ghcr.io/myorg/*", "!ghcr.io/myorg/public-*"},
			want:     true,
		},
		{
			name:     "no patterns means not ignored",
			imageRef: "node:20",
			patterns: nil,
			want:     false,
		},
		{
			name:     "mcr.microsoft.com wildcard",
			imageRef: "mcr.microsoft.com/playwright:v1.40.0-noble",
			patterns: []string{"mcr.microsoft.com/**"},
			want:     true,
		},
		{
			name:     "mcr.microsoft.com single star for direct child",
			imageRef: "mcr.microsoft.com/mssql/server:2019",
			patterns: []string{"mcr.microsoft.com/*"},
			want:     false, // mssql/server has a slash, * doesn't cross it
		},
		{
			name:     "mcr.microsoft.com doublestar for nested",
			imageRef: "mcr.microsoft.com/mssql/server:2019",
			patterns: []string{"mcr.microsoft.com/**"},
			want:     true,
		},
		{
			name:     "last match wins - re-include after exclude",
			imageRef: "ghcr.io/myorg/app:v1",
			patterns: []string{"ghcr.io/myorg/*", "!ghcr.io/myorg/*", "ghcr.io/myorg/app:*"},
			want:     true,
		},
		{
			name:     "escaped glob metachar with backslash",
			imageRef: "my[image]:latest",
			patterns: []string{"my\\[image\\]:*"},
			want:     true,
		},
		{
			name:     "positive then negative then positive (last match wins)",
			imageRef: "ghcr.io/myorg/app:v1",
			patterns: []string{"ghcr.io/myorg/*", "!ghcr.io/myorg/*", "ghcr.io/myorg/app:v1"},
			want:     true,
		},
		{
			name:     "only negative pattern without prior positive",
			imageRef: "node:20",
			patterns: []string{"!node:20"},
			want:     false,
		},
		{
			name:     "question mark wildcard",
			imageRef: "node:20",
			patterns: []string{"node:2?"},
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsIgnored(tt.imageRef, tt.patterns)
			if got != tt.want {
				t.Errorf("IsIgnored(%q, %v) = %v, want %v", tt.imageRef, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestValidatePatterns(t *testing.T) {
	t.Run("valid patterns", func(t *testing.T) {
		err := ValidatePatterns([]string{"ghcr.io/*", "!scratch", "*.dkr.ecr.*.amazonaws.com/**"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("empty patterns", func(t *testing.T) {
		err := ValidatePatterns(nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("invalid pattern", func(t *testing.T) {
		err := ValidatePatterns([]string{"[invalid"})
		if err == nil {
			t.Error("expected error for invalid pattern")
		}
	})
}

func TestLoadConfig(t *testing.T) {
	t.Run("no config file", func(t *testing.T) {
		dir := t.TempDir()
		origDir, _ := os.Getwd()
		os.Chdir(dir)
		defer os.Chdir(origDir)

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.IgnoreImages) != 0 {
			t.Errorf("expected empty IgnoreImages, got %v", cfg.IgnoreImages)
		}
	})

	t.Run("yaml config", func(t *testing.T) {
		dir := t.TempDir()
		origDir, _ := os.Getwd()
		os.Chdir(dir)
		defer os.Chdir(origDir)

		content := `ignore-images:
  - "ghcr.io/myorg/*"
  - "!ghcr.io/myorg/public-*"
  - "scratch"
`
		os.WriteFile(filepath.Join(dir, ".dockerfile-pin.yaml"), []byte(content), 0644)

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.IgnoreImages) != 3 {
			t.Errorf("expected 3 patterns, got %d: %v", len(cfg.IgnoreImages), cfg.IgnoreImages)
		}
	})

	t.Run("yml config", func(t *testing.T) {
		dir := t.TempDir()
		origDir, _ := os.Getwd()
		os.Chdir(dir)
		defer os.Chdir(origDir)

		content := `ignore-images:
  - "scratch"
`
		os.WriteFile(filepath.Join(dir, ".dockerfile-pin.yml"), []byte(content), 0644)

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.IgnoreImages) != 1 {
			t.Errorf("expected 1 pattern, got %d", len(cfg.IgnoreImages))
		}
	})

	t.Run("yaml takes precedence over yml", func(t *testing.T) {
		dir := t.TempDir()
		origDir, _ := os.Getwd()
		os.Chdir(dir)
		defer os.Chdir(origDir)

		os.WriteFile(filepath.Join(dir, ".dockerfile-pin.yaml"), []byte("ignore-images:\n  - \"a\"\n  - \"b\"\n"), 0644)
		os.WriteFile(filepath.Join(dir, ".dockerfile-pin.yml"), []byte("ignore-images:\n  - \"c\"\n"), 0644)

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.IgnoreImages) != 2 {
			t.Errorf("expected 2 patterns from .yaml, got %d: %v", len(cfg.IgnoreImages), cfg.IgnoreImages)
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		dir := t.TempDir()
		origDir, _ := os.Getwd()
		os.Chdir(dir)
		defer os.Chdir(origDir)

		os.WriteFile(filepath.Join(dir, ".dockerfile-pin.yaml"), []byte("invalid: [yaml: bad"), 0644)

		_, err := LoadConfig()
		if err == nil {
			t.Error("expected error for invalid YAML")
		}
	})
}

func TestMergeIgnorePatterns(t *testing.T) {
	t.Run("both empty", func(t *testing.T) {
		result := MergeIgnorePatterns(nil, nil)
		if len(result) != 0 {
			t.Errorf("expected empty, got %v", result)
		}
	})
	t.Run("config then cli", func(t *testing.T) {
		result := MergeIgnorePatterns([]string{"a", "b"}, []string{"c"})
		if len(result) != 3 || result[0] != "a" || result[1] != "b" || result[2] != "c" {
			t.Errorf("expected [a b c], got %v", result)
		}
	})
}
