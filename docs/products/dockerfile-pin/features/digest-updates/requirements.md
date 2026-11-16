# Digest updates

[日本語](requirements.ja.md)

## About this document

`run --update` re-resolves references that already have a digest, and `--min-age` holds back images that were built recently.
Ongoing, automated updates are left to Renovate, as the [dockerfile-pin](../../product.md) principles state; this feature is a manual re-resolve.

### Scope

- `--update` (`-u`) on `run`.
- `--min-age` on `run` and `min-age` in the configuration file.

### Out of scope

- Changing tags. Only the digest is updated; the tag stays as written.
- Scheduling updates or opening pull requests.
- The rest of `run`: [Pinning](../pinning/requirements.md).

## User stories

### Refresh pinned digests

As a developer, I want to re-resolve existing digests so that a moved tag is pinned to its current image.

**Acceptance criteria:**

Scenario: When `--update` is given, an existing digest is replaced with the tag's current digest

- Given: A file has `FROM node:20.11.1@sha256:<old>` and the tag now points to `<new>`.
- When: The developer runs `dockerfile-pin run --write --update`.
- Then: The line becomes `FROM node:20.11.1@sha256:<new>`.

Scenario: When `--update` is not given, an existing digest is kept

- Given: The same file.
- When: The developer runs `dockerfile-pin run --write`.
- Then: The line is unchanged.

### Wait before adopting new images

As a developer, I want to skip images built within the last few days so that I only pin images that have been public for a while.

**Acceptance criteria:**

Scenario: When an image was built within the period, it is not pinned

- Given: `nginx:latest` was built 2 days ago.
- When: The developer runs `dockerfile-pin run --write --update --min-age 7`.
- Then: `skipped nginx:latest (built <date>, within 7 days)` is printed and its line is unchanged.

Scenario: When an image has no build date, it is pinned

- Given: An image's configuration has no creation time, as with some reproducible builds.
- When: The developer runs `dockerfile-pin run --write --min-age 7`.
- Then: The image is pinned.

Scenario: When the build date cannot be read, the image is pinned with a warning

- Given: Reading an image's configuration fails.
- When: The developer runs `dockerfile-pin run --write --min-age 7`.
- Then: `WARN  <image>  failed to get creation time, pinning anyway: ...` is printed and the image is pinned.

Scenario: When `--min-age` is negative, the command fails

- Given: Any target files.
- When: The developer runs `dockerfile-pin run --min-age -1`.
- Then: The command exits with `--min-age must be a non-negative integer`.

## KPIs

None. dockerfile-pin does not collect usage data.

## Functional requirements

### Update

**Overview:** With `--update`, references with a digest are resolved like references without one.

**Input:** `--update` or `-u`.

**Output:** Existing digests replaced by the digest the tag points to now.

#### Applicable conditions

The image is resolved by the name and tag written before the `@`. Skipped and ignored references are not updated.

### Minimum age

**Overview:** An image whose build date is within the last N days is left out of the run.

**Input:** `--min-age N`, or `min-age: N` in the configuration file. The flag wins when both are given. The default is 0, which disables the check.

**Output:** Images newer than the period are neither pinned nor updated.

#### Applicable conditions

The build date is the creation time in the image's configuration.
The period applies to every image `run` resolves, with or without `--update`.

## Data requirements

### Required data

- **Build date:** The creation time of each resolved image, read only when the period is set.

### Relationships

None.

### Personal information

None.

## External service dependencies

- **Container registries**
  - **Related user stories:** All.
  - **Reason for dependency:** Current digests and image configurations exist only in the registry.
  - **Contractual and usage decisions:** With `--min-age`, the image configuration is fetched in addition to the HEAD request, so more registry requests are made.

## Security considerations

The minimum age is a cooldown: a tampered image pushed under an existing tag is not adopted until it has stayed public for the period.

## Open questions

None.
