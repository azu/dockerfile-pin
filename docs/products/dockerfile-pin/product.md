# dockerfile-pin

[日本語](product.ja.md)

## Overview

dockerfile-pin is a command-line tool that adds `@sha256:<digest>` to the Docker image references written in Dockerfiles, compose files, GitHub Actions files, and GitLab CI files.
`run` resolves each tag to the digest it points to in the registry and writes that digest next to the tag, and `check` reports references that have no digest or whose digest does not exist in the registry.
It runs on a developer's machine or in CI and reads registry credentials from `~/.docker/config.json`.

## Background and problems

### Background

Unconfirmed. See Open questions.

### Problems to solve

Unconfirmed. See Open questions.

## Target users

### Unconfirmed

- **Characteristics:** Unconfirmed. See Open questions.
- **Problems:** Unconfirmed. See Open questions.
- **Usage context:** Unconfirmed. See Open questions.

## Value provided

### Unconfirmed

- **Recipient:** Unconfirmed. See Open questions.
- **Offering:** Unconfirmed. See Open questions.
- **Reason:** Unconfirmed. See Open questions.

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

Unconfirmed. See Open questions.

## Development principles

Unconfirmed. See Open questions.

## SLA, SLI, and SLO

### SLA

None. dockerfile-pin is a command-line tool that runs on the user's machine or CI, and it has no hosted service whose availability could be guaranteed.

### SLI and SLO

None, for the same reason. The tool does not collect usage data, so there are no measured indicators.

## Open questions

### Purpose, users, and value

- **What to confirm:** Whether the following purpose, users, and value still describe dockerfile-pin.
- **Candidate and its sources:** Tags are mutable, so the same tag can point to different images over time, and pinning by digest keeps a build on the image that was reviewed. The aim is to prevent supply chain attacks, as pinact does for GitHub Actions. Users are developers who maintain Dockerfiles, compose files, and CI files, and who run the tool locally for migration and in CI for checking. Sources: the README's first paragraph and the Motivation and Goals of `docs/superpowers/specs/2026-03-27-dockerpin-design.md`.
- **Until decided:** Background, problems, target users, value, the definition of success, and development principles stay unconfirmed in the body.
- **Who decides:** The maintainer (azu).

### Digest updates relative to Renovate

- **What to confirm:** The 2026-03-27 design lists automatic digest updates as a non-goal delegated to Renovate, while `run --update` and `--min-age` now exist. Whether dockerfile-pin intends to own digest updates or still leaves ongoing updates to Renovate.
- **Candidate and its sources:** The README keeps recommending Renovate for automated ongoing updates and describes `--update` as a manual re-resolve.
- **Until decided:** The Digest updates feature is listed without a stated position relative to Renovate.
- **Who decides:** The maintainer (azu).
