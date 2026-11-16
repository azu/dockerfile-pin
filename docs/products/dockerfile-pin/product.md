# dockerfile-pin

[日本語](product.ja.md)

## Overview

dockerfile-pin is a command-line tool that adds `@sha256:<digest>` to the Docker image references written in Dockerfiles, compose files, GitHub Actions files, and GitLab CI files.
`run` resolves each tag to the digest it points to in the registry and writes that digest next to the tag, and `check` reports references that have no digest or whose digest does not exist in the registry.
It runs on a developer's machine or in CI and reads registry credentials from `~/.docker/config.json`.

## Background and problems

### Background

A Docker image tag such as `node:20` is mutable: the same tag can be pushed again with different content ([Docker Community Forums](https://forums.docker.com/t/can-a-docker-hub-tag-have-its-content-changed/139358)).
If someone malicious gains access to a registry, they can push a tampered image under an existing tag, and every build that names only the tag pulls it.
Supply chain attacks such as the [attack on Trivy](https://www.aquasec.com/blog/trivy-supply-chain-attack-what-you-need-to-know/) show that the registries and pipelines that publish images are realistic targets.
Docker Hub offers [immutable tags](https://docs.docker.com/docker-hub/repos/manage/hub-images/immutable-tags/), but the repository owner sets them, so the people who use an image cannot rely on them.

A digest is the content hash of an image, so naming an image by `@sha256:<digest>` keeps it on the content that was reviewed, the same way a lockfile fixes a package's integrity hash.
Keeping both the tag and the digest, as in `node:20.11.1@sha256:<digest>`, leaves the tag for people to read and lets Renovate update both.

### Problems to solve

Developers have to look up and write a digest for every image by hand, which is impractical across many Dockerfiles, compose files, and CI files.
For GitHub Actions `uses:`, [pinact](https://github.com/suzuki-shunsuke/pinact) automates SHA pinning, but no comparably simple tool exists for Docker image references: dockpin's maintenance has stalled, and hadolint has no rule that requires digests.
Without a check in CI, a new reference without a digest can be merged unnoticed, and a mistyped or deleted digest is found only when `docker build` fails.

## Target users

### Developers who maintain Docker and CI files

- **Characteristics:** They maintain repositories with Dockerfiles, compose files, GitHub Actions files, or GitLab CI files that name public or private registry images.
- **Problems:** They want every image reference pinned by digest, without writing digests by hand, and want to stop new unpinned references from being merged.
- **Usage context:** They run `run --write` once locally to pin existing files, run `check` in CI on every pull request, and let Renovate keep the digests up to date afterward.

## Value provided

### Pinning existing files in bulk

- **Recipient:** Developers adopting digest pinning in an existing repository.
- **Offering:** `run --write` adds the digest each tag points to now to every reference in the target files, keeping the tag.
- **Reason:** Looking up digests by hand does not scale, and keeping the tag lets people and Renovate keep reading and updating it.

### Preventing unpinned references in CI

- **Recipient:** Developers and reviewers of pull requests.
- **Offering:** `check` fails when a reference has no digest or its digest does not exist in the registry.
- **Reason:** Pinning once is not enough if new references can be added without digests, and checking existence finds a mistyped or deleted digest before the build does. The check uses HEAD requests, so it does not count against Docker Hub pull rate limits.

## Key concepts

- **Image reference:** The text that names a Docker image in a file, such as `node:20`, `ghcr.io/org/app:v1`, or `docker://alpine:3`. It consists of the image name, an optional tag, and an optional digest.
- **Digest:** The `sha256:<hex>` value that identifies one image content in a registry. A reference with a digest is written as `image:tag@sha256:<hex>`.
- **Pin:** To add the digest that a tag points to now to a reference, keeping the tag.
- **Target file:** A file dockerfile-pin reads. Its kind is one of Dockerfile, compose file, GitHub Actions file (workflow or `action.yml`), and GitLab CI file, decided from its path.
- **Skip:** A reference dockerfile-pin reads but does not pin or check, with a reason such as `scratch image`, `has build directive`, or `contains CI variable`.
- **Ignore pattern:** A glob pattern from `--ignore-images` or `.dockerfile-pin.yaml` that excludes matching images from pinning and checking.

## Feature list

| Feature | Description | Specification |
| --- | --- | --- |
| Dockerfile image references | Reads image references from `FROM` and `COPY --from=` and writes digests onto them | Not written |
| YAML image references | Reads image references from compose files, GitHub Actions files, and GitLab CI files and writes digests onto them | [Requirements](features/yaml-image-references/requirements.md) |
| Target file discovery | Finds target files from `-f`, `--glob`, or the default pattern and decides each file's kind | Not written |
| Pinning | `run` resolves tags to digests and shows or writes the pinned files | Not written |
| Digest updates | `run --update` re-resolves existing digests, and `--min-age` skips recently built images | Not written |
| Checking | `check` reports missing digests, digests absent from the registry, and, with `--verify-tag`, tags that point elsewhere | Not written |
| Ignore rules and configuration | Excludes images by pattern and reads settings from `.dockerfile-pin.yaml` | Not written |

## Definition of success

Every image reference in the repositories that use dockerfile-pin carries a digest, and CI keeps references without digests from being merged.

## Development principles

- Keep the tool simple: it adds digests and checks them, like pinact does for GitHub Actions.
- Leave ongoing digest updates to Renovate. dockerfile-pin does the first pinning and the CI check, and `run --update` is a manual re-resolve rather than an update service.
- Build on maintained libraries, such as Google's go-containerregistry (crane), instead of stalled ones.
- Make the tool itself safe to install in CI: releases are published with checksums for installers such as aqua, and GitHub Releases immutability is enabled.

## SLA, SLI, and SLO

### SLA

None. dockerfile-pin is a command-line tool that runs on the user's machine or CI, and it has no hosted service whose availability could be guaranteed.

### SLI and SLO

None, for the same reason. The tool does not collect usage data, so there are no measured indicators.

## Open questions

None.
