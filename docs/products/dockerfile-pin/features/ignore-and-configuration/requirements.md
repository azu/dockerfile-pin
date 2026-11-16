# Ignore rules and configuration

[日本語](requirements.ja.md)

## About this document

This feature lets a project exclude images from `run` and `check` with glob patterns, and keep its settings in a configuration file.
It belongs to [dockerfile-pin](../../product.md).

### Scope

- `--ignore-images` on `run` and `check`.
- `.dockerfile-pin.yaml` and its keys `ignore-images` and `min-age`.

### Out of scope

- What `min-age` does: [Digest updates](../digest-updates/requirements.md).
- Excluding files. Patterns match images, not paths.
- Setting other options, such as `--glob` or `--verify-tag`, in the configuration file.

## User stories

### Exclude images that should not be pinned

As a developer, I want to exclude some images, such as internal ones, so that `check` does not fail on them and `run` does not pin them.

**Acceptance criteria:**

Scenario: When an image matches a pattern, `check` skips it

- Given: A file has `FROM ghcr.io/myorg/app:v1` without a digest.
- When: The developer runs `dockerfile-pin check --ignore-images "ghcr.io/myorg/*"`.
- Then: The reference is reported as `SKIP` with `ignored`, and it does not cause a failure.

Scenario: When an image matches a pattern, `run` leaves it unchanged

- Given: The same file.
- When: The developer runs `dockerfile-pin run --write --ignore-images "ghcr.io/myorg/*"`.
- Then: The image is not resolved and its line is unchanged.

Scenario: When a later negation matches, the image is checked again

- Given: Patterns `ghcr.io/myorg/*` then `!ghcr.io/myorg/public-*`.
- When: `check` reads `ghcr.io/myorg/public-api:v1` and `ghcr.io/myorg/private:v1`.
- Then: The first is checked and the second is skipped.

Scenario: When a pattern is invalid, the command fails before reading files

- Given: A pattern with an unclosed `[`.
- When: The developer runs `dockerfile-pin check --ignore-images "[abc"`.
- Then: The command exits with `invalid ignore pattern "[abc"`.

### Keep settings in the repository

As a developer, I want the ignore rules in a file so that local runs and CI use the same rules.

**Acceptance criteria:**

Scenario: When the configuration file exists, its patterns apply

- Given: `.dockerfile-pin.yaml` in the current directory has `ignore-images: ["scratch", "mcr.microsoft.com/**"]`.
- When: The developer runs `dockerfile-pin check` with no `--ignore-images`.
- Then: Matching images are skipped as `ignored`.

Scenario: When both the file and the flag give patterns, the flag's patterns come last

- Given: The file has `ghcr.io/myorg/*`.
- When: The developer runs `dockerfile-pin check --ignore-images "!ghcr.io/myorg/app:*"`.
- Then: `ghcr.io/myorg/app:v1` is checked.

Scenario: When the configuration file is not valid YAML, the command fails

- Given: `.dockerfile-pin.yaml` cannot be parsed.
- When: The developer runs `dockerfile-pin check`.
- Then: The command exits with `parsing .dockerfile-pin.yaml: ...`.

## KPIs

None. dockerfile-pin does not collect usage data.

## Functional requirements

### Ignore patterns

**Overview:** An image is ignored when the last pattern that matches it is not a negation.

**Input:** The patterns of the configuration file followed by the `--ignore-images` values, in order. The flag can be repeated; each use is one pattern.

**Output:** For each image, ignored or not.

#### Applicable conditions

Patterns use doublestar glob syntax and must match the whole image: `*` does not cross `/`, `**` does, and `?` matches one character.
A pattern starting with `!` is a negation.
The text matched is the image without its digest and without the `docker://` prefix, after `ARG` expansion in `FROM`; for example `node:*` matches `node:20` whether or not the reference has a digest, and `scratch` does not match `scratch:latest`.
A reference its reader already skips keeps its own skip reason.

### Configuration file

**Overview:** Settings are read from `.dockerfile-pin.yaml`, or `.dockerfile-pin.yml` when the first does not exist, in the current directory.

**Input:** The file's `ignore-images` (list of patterns) and `min-age` (days).

**Output:** The settings; no file means no settings.

#### Exceptional behavior

A file that cannot be read or parsed stops the command with an error. Keys other than the two above are not used.

## Data requirements

### Required data

- **Configuration:** The patterns and the minimum age, read at the start of each command.

### Relationships

None.

### Personal information

None.

## External service dependencies

None.

## Security considerations

An ignored image is not checked, so a broad pattern can hide unpinned images. Negation patterns allow a narrow exception to be written instead of a broad exclusion.

## Open questions

None.
