# Design Doc: DockerPin — Dockerfile digest pinning tool

## Overview

A CLI tool that adds `@sha256:<digest>` to FROM lines in Dockerfiles to prevent supply chain attacks. Based on crane, it provides two features: migration (bulk pinning of existing Dockerfiles) and check (validation of digest syntax and registry existence). Digest updates are deferred to Renovate and out of scope.

---

## Motivation

Docker image tags are mutable — the same tag `node:20` can point to different images on different days. Adding `@sha256:<digest>` guarantees build immutability, but manual management is impractical. Just as pinact (https://github.com/suzuki-shunsuke/pinact) automates SHA pinning for GitHub Actions, a similar tool is needed for Dockerfiles.

Existing tools (dockpin, docker-lock, etc.) have stalled maintenance. Building on crane (google/go-containerregistry), a Google-maintained library, is more sustainable.

---

## Goals

- Bulk-add digests to FROM lines in existing Dockerfiles (migration)
- Validate that FROM line digests are syntactically correct and exist in the registry (check)
- Integrate into CI (GitHub Actions, etc.)
- Support private registries (GCR, GHCR, ECR, etc.)

## Non-Goals

- Automatic digest updates (delegated to Renovate)
- docker-compose.yml or Kubernetes manifest support (future consideration)
- Base image vulnerability scanning (delegated to Trivy, etc.)

---

## Design

### Command Structure

```
dockerpin pin [flags]       # Migration: add digests to FROM lines
dockerpin check [flags]     # Check: validate digest syntax + registry existence
```

### Subcommand: `dockerpin pin`

Parses FROM lines in existing Dockerfiles and adds digests to tag-only image references.

**Input**:

```dockerfile
FROM node:20.11.1
FROM python:3.12-slim AS builder
FROM golang:1.22
```

**Output**:

```dockerfile
FROM node:20.11.1@sha256:abc123...
FROM python:3.12-slim@sha256:def456... AS builder
FROM golang:1.22@sha256:789ghi...
```

**Behavior**:

1. Parse Dockerfile line by line, extract FROM lines
2. Extract image reference (`image:tag`) from each FROM line
3. Skip lines that already have `@sha256:` (force update with `--update` flag)
4. Fetch digest from registry using crane library (https://github.com/google/go-containerregistry)
5. Rewrite FROM line in `image:tag@sha256:<digest>` format
6. Preserve multi-stage build `AS <name>` and `--platform` flags

**Flags**:

- `-f, --file <path>`: Target Dockerfile path (default: `./Dockerfile`)
- `--glob <pattern>`: Specify target files by glob pattern (e.g., `**/Dockerfile*`, `services/*/Dockerfile`)
- `--dry-run`: Display changes to stdout without modifying files
- `--update`: Update digests even on lines that already have one
- `--platform <os/arch>`: Platform for multi-architecture images (default: `linux/amd64`)

### Subcommand: `dockerpin check`

Performs both syntax checks (digest presence) and registry existence checks (digest actually exists) on FROM lines.

**Check contents**:

1. **Syntax check**: Report error if FROM line does not contain `@sha256:`
2. **Existence check**: If `@sha256:` is present, verify the digest exists in the registry via HEAD request

**Output example**:

```
FAIL  Dockerfile:3    FROM node:20.11.1              missing digest
FAIL  Dockerfile:7    FROM python:3.12@sha256:abc... digest not found in registry
OK    Dockerfile:12   FROM golang:1.22@sha256:def...
```

**Flags**:

- `-f, --file <path>`: Target Dockerfile path
- `--glob <pattern>`: Specify target files by glob pattern (e.g., `**/Dockerfile*`)
- `--syntax-only`: Skip registry queries, perform syntax check only
- `--format <text|json>`: Output format (JSON is convenient for CI parsing)
- `--ignore-images <pattern>`: Exclude specific images from checks (e.g., `scratch`, local build images)
- `--exit-code`: Exit code on check failure (default: 1)

### CI Integration

GitHub Actions usage example:

```yaml
name: Dockerfile Digest Check
on: [pull_request]
jobs:
  check-digest:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install dockerpin
        run: go install github.com/azu/dockerfile-pin@latest
      - name: Check Dockerfile digests
        run: dockerpin check --glob '**/Dockerfile*'
```

---

## Registry API and Rate Limits

crane uses OCI Distribution Spec compliant registry APIs. Rate limits vary by registry.

### Docker Hub

- **Unauthenticated**: 100 pulls / 6 hours (IP-based)
- **Personal (free)**: 200 pulls / 6 hours
- **Pro/Team/Business**: Unlimited
- **HEAD requests**: HEAD requests to `/v2/<name>/manifests/<reference>` do not count against pull rate limits (https://docs.docker.com/docker-hub/usage/pulls/). However, general abuse rate limits may apply (https://www.augmentedmind.de/2024/12/15/docker-hub-rate-limit-head-request/).

### GHCR (GitHub Container Registry)

- Public images: unlimited
- Rate limit: 44,000 requests/min (effectively no issue)

### GCR / Artifact Registry (Google)

- No explicit rate limits published for authenticated requests
- Generally not an issue even with high request volumes

### Design Considerations

- `dockerpin check` uses HEAD requests (`crane head` equivalent) for existence checks, avoiding Docker Hub pull rate limit consumption
- `dockerpin pin` requires GET requests for digest retrieval, but Dockerfiles typically have only a few to a dozen FROM lines, making rate limit exhaustion unlikely
- Private registry access authenticates via Docker credential helper (`~/.docker/config.json`). crane supports this natively.

---

## Implementation

### Language

Go. crane library (https://github.com/google/go-containerregistry) can be imported directly as a Go library. While shell script + crane CLI is possible, Go is more appropriate for error handling and cross-platform support.

### Dependencies

- `github.com/google/go-containerregistry`: Registry API operations (digest retrieval, manifest retrieval)
- `github.com/moby/buildkit`: Dockerfile parser (for accurate FROM line parsing)
- `github.com/spf13/cobra`: CLI framework

### Dockerfile Parser Design

FROM line parsing uses BuildKit's Dockerfile parser instead of regex. This correctly handles:

- `FROM --platform=$BUILDPLATFORM golang:1.22 AS builder`
- `FROM ${BASE_IMAGE}:${VERSION}` (ARG expansion is out of scope; warn when ARGs are present)
- `FROM scratch` (`scratch` is a special image, skip)
- Multi-stage build `FROM <stage-name>` (skip internal stage name references)

### scratch / ARG / Local Image Handling

Some FROM lines must be excluded from pinning:

- `FROM scratch`: Docker special keyword, not an actual image. Always skip.
- `FROM <stage-name>`: References a previous stage in multi-stage build. Match against stage names defined in the Dockerfile, skip.

### ARG Variables in FROM Lines

Dockerfiles can use `ARG`-defined variables in `FROM`. Handling depends on usage pattern.

**Pattern 1: ARG with default value (supported)**

```dockerfile
ARG NODE_VERSION=20.11.1
FROM node:${NODE_VERSION}
```

Image reference can be statically resolved by reading ARG default values from the Dockerfile. `pin` rewrites as `FROM node:${NODE_VERSION}@sha256:...`. Renovate also supports this pattern.

**Pattern 2: Registry-only variable (supported)**

```dockerfile
ARG REGISTRY=docker.io
FROM ${REGISTRY}/node:20.11.1
```

Tag portion is statically readable, so digest can be fetched against the default registry. Warn that digest may not match if a different registry is provided at runtime.

**Pattern 3: Fully variable image name (not supported)**

```dockerfile
ARG BASE_IMAGE
FROM ${BASE_IMAGE}
```

No default value, passed via `--build-arg` at build time. Cannot be resolved from Dockerfile alone. Report warning and skip. `check` also skips (cannot verify digest).

**Pattern 4: Platform-only variable (supported)**

```dockerfile
FROM --platform=$BUILDPLATFORM golang:1.22
```

Image reference is static even with variable `--platform`. Can use manifest list digest for platform-independent handling.

**Implementation approach**: After AST-ifying with BuildKit's Dockerfile parser, scan ARG instructions first to build a default value map, then expand variables in FROM lines where possible. FROM lines with unexpandable variables (no default value) are reported as warnings and skipped.

---

## Testing

### Unit Tests

- FROM line parser tests (various patterns: with tag, with digest, platform specification, AS clause, ARG reference)
- Digest format validation
- Output format verification

### Integration Tests

- Digest retrieval and existence verification against real registries (Docker Hub, GHCR)
- Private registry mocking

### Test Dockerfile Sample

```dockerfile
# Lines that should be pinned
FROM node:20.11.1
FROM python:3.12-slim AS builder
FROM --platform=linux/amd64 golang:1.22

# Lines that should be skipped
FROM scratch
FROM builder AS final
FROM ${BASE_IMAGE}:${VERSION}

# Already pinned (OK for check, skip for pin)
FROM node:20.11.1@sha256:d938c1761e3afbae9242848ffbb95b9cc1cb0a24d889f8bd955204d347a7266e
```

---

## Known Limitations

- **ARG in FROM lines**: ARGs with default values can be expanded and pinned. Fully dynamic references like `FROM ${BASE}:${TAG}` without defaults cannot be pinned and are reported as warnings. See "ARG Variables in FROM Lines" section.
- **Multi-architecture**: Digests differ per platform. Choice between explicit `--platform` flag or manifest list digest (common across all platforms). Manifest list digest is more versatile (Renovate also uses this approach).
- **Tag-less digest**: Notation like `FROM alpine@sha256:...` without a tag has poor human readability. `dockerpin pin` recommends keeping both tag and digest.

---

## References

- crane (go-containerregistry): https://github.com/google/go-containerregistry
- pinact: https://github.com/suzuki-shunsuke/pinact
- Renovate Docker config: https://docs.renovatebot.com/docker/
- Docker Hub rate limit: https://docs.docker.com/docker-hub/usage/pulls/
- OCI Distribution Spec: https://github.com/opencontainers/distribution-spec
- dockpin (prior tool): https://github.com/Jille/dockpin
