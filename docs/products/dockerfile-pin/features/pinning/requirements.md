# Pinning

[日本語](requirements.ja.md)

## About this document

`dockerfile-pin run` resolves the tag of each image reference to the digest it points to now, and shows or writes the target files with those digests added.
It belongs to [dockerfile-pin](../../product.md).

### Scope

- Which references `run` resolves.
- Dry-run and `--write`.
- What `run` prints, and how registry failures are handled.
- Registry authentication.

### Out of scope

- Which files are read: [Target file discovery](../target-file-discovery/requirements.md).
- How references are read and rewritten: [Dockerfile image references](../dockerfile-image-references/requirements.md) and [YAML image references](../yaml-image-references/requirements.md).
- `--update` and `--min-age`: [Digest updates](../digest-updates/requirements.md).
- `--ignore-images` and the configuration file: [Ignore rules and configuration](../ignore-and-configuration/requirements.md).

## User stories

### Preview before changing files

As a developer, I want `run` to show the result without changing files so that I can review it first.

**Acceptance criteria:**

Scenario: When `--write` is not given, the pinned content is printed and files are unchanged

- Given: A `Dockerfile` has `FROM node:20.11.1`.
- When: The developer runs `dockerfile-pin run`.
- Then: The output has `found 1 file(s)`, `resolving 1 unique image(s)...`, a `resolved node:20.11.1 → sha256:...` line, and `--- Dockerfile` followed by the whole file with the digest added; the file on disk is unchanged.

### Write digests into files

As a developer, I want `run --write` to change the files in place so that I can commit the pinned files.

**Acceptance criteria:**

Scenario: When `--write` is given, files are rewritten and counted

- Given: A `Dockerfile` has two unpinned `FROM` lines.
- When: The developer runs `dockerfile-pin run --write`.
- Then: The file contains both digests, its permissions are unchanged, and `pinned 2 image(s) in Dockerfile` is printed.

Scenario: When a reference already has a digest, it is left as it is

- Given: A file has `FROM node:20@sha256:<digest>`.
- When: The developer runs `dockerfile-pin run --write` without `--update`.
- Then: The line is unchanged and the image is not resolved.

Scenario: When nothing in a file needs a digest, the file is not printed or written

- Given: Every reference in a file is pinned or skipped.
- When: The developer runs `dockerfile-pin run`.
- Then: No `--- <file>` block is printed for it.

### Keep going when a registry fails

As a developer, I want one failing image not to stop the others so that the rest is still pinned.

**Acceptance criteria:**

Scenario: When an image cannot be resolved, a warning is printed and the others are pinned

- Given: A file names `node:20` and an image that does not exist in its registry.
- When: The developer runs `dockerfile-pin run --write`.
- Then: `WARN  <image>  failed to resolve: ...` is printed on standard error, `node:20` is pinned, the missing image's line is unchanged, and the exit code is 0.

Scenario: When a file cannot be parsed, the other files are processed

- Given: One target file is not valid and another is.
- When: The developer runs `dockerfile-pin run`.
- Then: `error parsing <file>: ...` is printed on standard error and the valid file is processed.

## KPIs

None. dockerfile-pin does not collect usage data.

## Functional requirements

### Resolving

**Overview:** Each distinct image is resolved once for the whole run.

**Input:** The references of all target files.

**Output:** A digest for each image that could be resolved.

#### Applicable conditions

A reference is resolved when it is not skipped, not ignored, and has no digest.
The image is looked up by its name and tag as written; the same image in several files gets the same digest.
Resolution uses HEAD requests, which do not count against Docker Hub pull rate limits.
Credentials come from `~/.docker/config.json`, so `docker login` and credential helpers work for private registries.

#### Exceptional behavior

An image that fails to resolve is reported as a warning and left unchanged. The whole run stops after 10 minutes, and each registry request after 30 seconds.

### Output

**Overview:** Dry-run is the default; `--write` changes files.

**Input:** `--write`.

**Output:** Progress lines, then per file either the pinned content (dry-run) or a `pinned N image(s) in <file>` line.

#### Applicable conditions

Dry-run prints `--- <path>` and the whole pinned content for each file that gets at least one digest.
`--write` replaces the file content and keeps the file's permissions.
`run` accepts no positional arguments.

## Data requirements

### Required data

- **Resolved digest:** The digest for each image, held for the duration of the run.

### Relationships

One digest per image name and tag.

### Personal information

None.

## External service dependencies

- **Container registries (Docker Hub, GHCR, GCR, ECR, and others)**
  - **Related user stories:** All.
  - **Reason for dependency:** The digest a tag points to exists only in the registry.
  - **Contractual and usage decisions:** The user's own registry accounts and limits apply. HEAD requests avoid Docker Hub pull rate limits.

## Security considerations

Credentials are read from the user's Docker configuration and are not printed. Files are changed only with `--write`.

## Open questions

None.
