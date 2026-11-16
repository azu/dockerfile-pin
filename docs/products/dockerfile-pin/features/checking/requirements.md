# Checking

[日本語](requirements.ja.md)

## About this document

`dockerfile-pin check` reports, for every image reference in the target files, whether it has a digest and whether that digest exists in the registry, and fails when any reference does not pass.
It belongs to [dockerfile-pin](../../product.md).

### Scope

- The statuses and what causes each.
- `--syntax-only`, `--verify-tag`, `--format`, and `--exit-code`.

### Out of scope

- Which files are read: [Target file discovery](../target-file-discovery/requirements.md).
- How references are read: [Dockerfile image references](../dockerfile-image-references/requirements.md) and [YAML image references](../yaml-image-references/requirements.md).
- Ignore patterns: [Ignore rules and configuration](../ignore-and-configuration/requirements.md).
- Detecting tampering. `check` cannot tell whether a digest belongs to the image the author intended.

## User stories

### Stop unpinned references in CI

As a developer, I want CI to fail when a reference has no digest so that unpinned images are not merged.

**Acceptance criteria:**

Scenario: When a reference has no digest, it fails

- Given: A `Dockerfile` has `FROM node:20.11.1` on line 1.
- When: CI runs `dockerfile-pin check`.
- Then: `FAIL  Dockerfile:1    FROM node:20.11.1    missing digest` is printed and the exit code is 1.

Scenario: When every reference passes or is skipped, the command succeeds

- Given: A `Dockerfile` has `FROM python:3.12@sha256:<existing digest>` and `FROM scratch`.
- When: CI runs `dockerfile-pin check`.
- Then: The first is `OK`, the second is `SKIP` with `scratch image`, and the exit code is 0.

### Find digests that do not exist

As a developer, I want a mistyped or deleted digest reported before the build so that I do not find it at `docker build`.

**Acceptance criteria:**

Scenario: When the digest is absent from the registry, it fails

- Given: A reference has a well-formed digest that the registry does not have.
- When: The developer runs `dockerfile-pin check`.
- Then: It is reported as `FAIL` with `digest not found in registry`.

Scenario: When the registry cannot be reached, it warns without failing

- Given: The registry request fails with a network or authentication error.
- When: The developer runs `dockerfile-pin check`.
- Then: The reference is reported as `WARN` with `registry check failed: ...`, and it alone does not make the exit code non-zero.

Scenario: When `--syntax-only` is given, the registry is not contacted

- Given: A reference has any digest.
- When: The developer runs `dockerfile-pin check --syntax-only`.
- Then: It is reported as `OK` without a registry request.

### Verify that the tag matches the digest

As a developer, I want to check that each tag points to the pinned digest so that a digest of a different version is noticed.

**Acceptance criteria:**

Scenario: When the tag points to another digest, it fails

- Given: A file has `FROM busybox:1.37@sha256:<digest of another image>`, and that digest exists.
- When: The developer runs `dockerfile-pin check --verify-tag`.
- Then: It is reported as `FAIL` with `tag points to a different digest: <current digest>`.

Scenario: When the reference has no tag, there is nothing to verify

- Given: A file has `FROM node@sha256:<digest>`.
- When: The developer runs `dockerfile-pin check --verify-tag`.
- Then: It is reported as `OK` with `no tag to verify`.

Scenario: When `--verify-tag` is combined with `--syntax-only`, the command fails

- Given: Any target files.
- When: The developer runs `dockerfile-pin check --verify-tag --syntax-only`.
- Then: The command exits with `--verify-tag cannot be used with --syntax-only`.

### Read results from other tools

As a developer who integrates `check` into CI, I want machine-readable output and a configurable exit code.

**Acceptance criteria:**

Scenario: When `--format json` is given, a JSON array is printed

- Given: A file with one unpinned reference.
- When: The developer runs `dockerfile-pin check --format json`.
- Then: An array of objects with `file`, `line`, `image`, `status`, `message`, and `original` is printed, with `status` `"fail"`.

Scenario: When `--exit-code` is given, that code is used on failure

- Given: A file with one unpinned reference.
- When: The developer runs `dockerfile-pin check --exit-code 2`.
- Then: The exit code is 2.

## KPIs

None. dockerfile-pin does not collect usage data.

## Functional requirements

### Statuses

**Overview:** Each reference gets exactly one status.

**Input:** The references of the target files and the options.

**Output:** One result per reference, in file and reference order.

#### Applicable conditions

The first matching row decides the status.

| Order | Condition | Status | Message |
| --- | --- | --- | --- |
| 1 | The reference is skipped by its reader | `SKIP` | The skip reason |
| 2 | The image matches the ignore patterns | `SKIP` | `ignored` |
| 3 | No digest | `FAIL` | `missing digest` |
| 4 | `--syntax-only` | `OK` | |
| 5 | The registry request fails | `WARN` | `registry check failed: <error>` |
| 6 | The digest is not in the registry | `FAIL` | `digest not found in registry` |
| 7 | Otherwise | `OK` | |

With `--verify-tag`, a reference that reached row 7 is checked further: without a tag it stays `OK` with `no tag to verify`; when resolving the tag fails it becomes `WARN` with `tag resolve failed: <error>`; when the tag's digest differs it becomes `FAIL` with `tag points to a different digest: <digest>`.
A colon before the last `/`, as in `localhost:5000/app`, is a registry port and not a tag.

The exit code is the value of `--exit-code` (default 1) when any result is `FAIL`, and 0 otherwise.

#### Exceptional behavior

A file that cannot be read or parsed is reported on standard error as `error processing <file>: ...`; the other files are still checked, and that error alone does not change the exit code.

### Output formats

**Overview:** Text is the default; `--format json` prints JSON.

**Input:** `--format`.

**Output:** In text, one line per result: the status, `<file>:<line>`, the reference as written with its instruction or key (for example `FROM node:20`, `image: node:20`, `uses: docker://alpine:3`, `- postgres:16`), and the message. In JSON, an array of objects with `file`, `line`, `image` (without digest), `status` (`ok`, `fail`, `skip`, `warn`), `message`, and `original`.

## Data requirements

### Required data

- **Check result:** File, line, image, status, message, and the original text. It exists only while the command runs.

### Relationships

One result per reference.

### Personal information

None.

## External service dependencies

- **Container registries**
  - **Related user stories:** Find digests that do not exist; Verify that the tag matches the digest.
  - **Reason for dependency:** Whether a digest exists and where a tag points are known only to the registry.
  - **Contractual and usage decisions:** HEAD requests are used, which do not count against Docker Hub pull rate limits. `--verify-tag` adds one request per reference with a tag.

## Security considerations

`--verify-tag` compares the digest with where the tag points now. A registry does not return where a tag pointed in the past, so a mismatch means only that they differ now; it cannot tell an upstream rebuild from a replaced digest. This is why it is off by default.

## Open questions

None.
