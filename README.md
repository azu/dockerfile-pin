# dockerfile-pin

A CLI tool that adds `@sha256:<digest>` to `FROM` lines in Dockerfiles and `image` fields in docker-compose.yml to prevent supply chain attacks.

## Install

### Homebrew / curl

```bash
# Download binary (macOS Apple Silicon)
curl -sL "https://github.com/azu/dockerfile-pin/releases/latest/download/dockerfile-pin_darwin_arm64.tar.gz" | tar xz
sudo mv dockerfile-pin /usr/local/bin/

# Download binary (Linux amd64)
curl -sL "https://github.com/azu/dockerfile-pin/releases/latest/download/dockerfile-pin_linux_amd64.tar.gz" | tar xz
sudo mv dockerfile-pin /usr/local/bin/
```

### aqua

Add to your `aqua.yaml`:

```yaml
registries:
  - name: dockerfile-pin
    type: github_content
    repo_owner: azu
    repo_name: dockerfile-pin
    ref: <commit-sha> # main
    path: registry.yaml
packages:
  - name: azu/dockerfile-pin@v1.0.1
    registry: dockerfile-pin
```

```bash
aqua i
```

### Go

```bash
go install github.com/azu/dockerfile-pin@latest
```

See [GitHub Releases](https://github.com/azu/dockerfile-pin/releases) for all platforms.

## Usage

### Run

Add digests to Dockerfile `FROM` lines or docker-compose.yml `image` fields.
By default, shows changes without modifying files (dry-run).

```bash
# Preview changes (dry-run, default)
dockerfile-pin run

# Preview a specific file
dockerfile-pin run -f path/to/Dockerfile

# Preview multiple files using glob
dockerfile-pin run --glob '**/Dockerfile*'

# Preview docker-compose.yml
dockerfile-pin run -f docker-compose.yml

# Actually write changes to files
dockerfile-pin run --write

# Update existing digests
dockerfile-pin run --write --update
```

**Before:**

```dockerfile
FROM node:20.11.1
FROM python:3.12-slim AS builder
FROM scratch
```

**After:**

```dockerfile
FROM node:20.11.1@sha256:e06aae17c40c7a6b5296ca6f942a02e6737ae61bbbf3e2158624bb0f887991b5
FROM python:3.12-slim@sha256:3d5ed973e45820f5ba5e46bd065bd88b3a504ff0724d85980dcd05eab361fcf4 AS builder
FROM scratch
```

### Check

Validate that digests are present and exist in the registry.

```bash
# Check a single Dockerfile
dockerfile-pin check -f Dockerfile

# Check multiple files
dockerfile-pin check --glob '**/Dockerfile*'

# Syntax check only (no registry queries)
dockerfile-pin check --syntax-only

# JSON output for CI
dockerfile-pin check --format json

# Ignore specific images
dockerfile-pin check --ignore-images scratch,mylocal
```

**Output:**

```
FAIL  Dockerfile:1    FROM node:20.11.1                                  missing digest
OK    Dockerfile:3    FROM python:3.12@sha256:abc123...
SKIP  Dockerfile:5    FROM scratch                                       scratch image
```

Exit code is `1` when any check fails (configurable with `--exit-code`).

## Supported Patterns

### Dockerfiles

| Pattern | Supported |
|---------|-----------|
| `FROM image:tag` | Yes |
| `FROM image:tag AS name` | Yes |
| `FROM --platform=linux/amd64 image:tag` | Yes |
| `FROM image:tag@sha256:...` (already pinned) | Skipped (use `--update` to refresh) |
| `FROM scratch` | Skipped |
| `FROM <stage-name>` (multi-stage ref) | Skipped |
| `ARG VERSION=1.0` + `FROM image:${VERSION}` | Yes (expanded from default) |
| `ARG BASE` + `FROM ${BASE}` (no default) | Skipped with warning |
| `FROM ghcr.io/org/image:tag` | Yes |
| `FROM registry:5000/image:tag` | Yes |

### docker-compose.yml

| Pattern | Supported |
|---------|-----------|
| `image: node:20` | Yes |
| `image: node:20@sha256:...` | Skipped (use `--update`) |
| Service with `build:` directive | Skipped |
| Service without `image:` key | Skipped |

## CI Integration

### Check (PR validation)

Validate that all images are pinned on every pull request.

With aqua (if your project already uses aqua):

```yaml
# .github/workflows/dockerfile-check.yml
name: Dockerfile Digest Check
on: [pull_request]
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: aquaproj/aqua-installer@v3
        with:
          aqua_version: v2.45.0
      - run: dockerfile-pin check
```

Without aqua:

```yaml
# .github/workflows/dockerfile-check.yml
name: Dockerfile Digest Check
on: [pull_request]
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install dockerfile-pin
        run: |
          curl -sL "https://github.com/azu/dockerfile-pin/releases/latest/download/dockerfile-pin_linux_amd64.tar.gz" | tar xz -C /usr/local/bin
      - run: dockerfile-pin check
```

`dockerfile-pin check` exits with code 1 if any image is missing a digest.
By default, it auto-detects all Dockerfiles and compose files in the repository (using `git ls-files`).

### Pin (migration)

Run locally to add digests to all Dockerfiles and compose files:

```bash
# Preview changes
dockerfile-pin run

# Apply changes
dockerfile-pin run --write
```

### Private registries

For private registries (GCR, GHCR, ECR), configure Docker credentials before running:

```yaml
      # GHCR
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      # GCR
      - uses: google-github-actions/auth@v2
        with:
          credentials_json: ${{ secrets.GCP_SA_KEY }}
      - uses: google-github-actions/setup-gcloud@v2
      - run: gcloud auth configure-docker
```

`dockerfile-pin` uses `~/.docker/config.json` for authentication, so any `docker login` or credential helper works.

## How It Works

- Uses [go-containerregistry](https://github.com/google/go-containerregistry) (crane) for registry API calls
- Uses BuildKit's Dockerfile parser for accurate FROM line parsing
- `run` resolves digests via HEAD requests (does not count against Docker Hub pull rate limits)
- `check` verifies digest existence via HEAD requests
- Authenticates using `~/.docker/config.json` (supports Docker Hub, GHCR, GCR, ECR, etc.)

## Digest Updates

This tool handles initial pinning and validation. For ongoing digest updates, use [Renovate](https://docs.renovatebot.com/docker/) which understands the `image:tag@sha256:digest` format.

## License

MIT
