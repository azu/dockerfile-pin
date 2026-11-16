# Dockerfile image references

[日本語](requirements.ja.md)

## About this document

This feature reads Docker image references from `FROM`, `COPY --from=`, and the `from=` option of `RUN --mount` in a Dockerfile, and writes a digest onto each reference where it is written.
`run` pins the references it reads, and `check` reports on them.
It belongs to [dockerfile-pin](../../product.md).

### Scope

- Which instructions are read, and which references are skipped.
- How `ARG` defaults are expanded in `FROM`.
- How a reference that already has a digest is recognized.
- How a digest is written into the Dockerfile text, including instructions continued over several lines.

### Out of scope

- Finding Dockerfiles, which belongs to [Target file discovery](../target-file-discovery/requirements.md).
- Resolving digests and the output of `run` and `check`, which belong to [Pinning](../pinning/requirements.md) and [Checking](../checking/requirements.md).
- `ADD --from=`. It is not read; the Dockerfile reference lists no such option for `ADD`.
- Named build contexts (`docker buildx build --build-context name=...`). A `COPY --from=<name>` or `RUN --mount=...,from=<name>` that matches no stage is read as an image, because the two cannot be told apart from the Dockerfile alone.

## User stories

### Pin base images

As a developer who maintains Dockerfiles, I want `run` to add digests to `FROM` so that builds keep using the base images I pinned.

**Acceptance criteria:**

Scenario: When `FROM` names an image with a tag, the digest is written after the tag

- Given: A Dockerfile has `FROM python:3.12-slim AS builder` and `FROM --platform=linux/amd64 node:20`.
- When: The developer runs `dockerfile-pin run --write -f Dockerfile`.
- Then: The lines become `FROM python:3.12-slim@sha256:<digest> AS builder` and `FROM --platform=linux/amd64 node:20@sha256:<digest>`, and every other line is unchanged.

Scenario: When `FROM` names `scratch` or an earlier stage, it is skipped

- Given: A Dockerfile has `FROM golang:1.22 AS builder`, then `FROM scratch`, then `FROM builder`.
- When: The developer runs `dockerfile-pin check -f Dockerfile`.
- Then: `FROM scratch` is reported as `SKIP` with `scratch image`, `FROM builder` as `SKIP` with `stage reference`, and `run` leaves both unchanged.

Scenario: When `FROM` uses an `ARG` with a default, the default is expanded

- Given: A Dockerfile has `ARG VERSION=1.0` followed by `FROM image:${VERSION}`.
- When: The developer runs `dockerfile-pin run --write`.
- Then: The digest of `image:1.0` is written, and the line becomes `FROM image:${VERSION}@sha256:<digest>`.

Scenario: When `FROM` uses an `ARG` without a default, it is skipped

- Given: A Dockerfile has `ARG BASE` followed by `FROM ${BASE}`.
- When: The developer runs `dockerfile-pin check`.
- Then: The reference is reported as `SKIP` with `unresolved ARG variable`.

### Pin images copied from

As a developer who copies files from external images, I want `COPY --from=` images to be pinned so that they get the same guarantee as base images.

**Acceptance criteria:**

Scenario: When `COPY --from=` names an image, the digest is written into the flag

- Given: A Dockerfile has `COPY --chown=node:20 --from=node:20 /app /app`.
- When: The developer runs `dockerfile-pin run --write`.
- Then: The line becomes `COPY --chown=node:20 --from=node:20@sha256:<digest> /app /app`; the `--chown` value is unchanged.

Scenario: When `COPY --from=` names a stage or a stage index, it is skipped

- Given: A Dockerfile has `COPY --from=builder /a /b` where `builder` is a stage declared anywhere in the file, and `COPY --from=0 /a /b`.
- When: The developer runs `dockerfile-pin check`.
- Then: They are reported as `SKIP` with `stage reference` and `stage index reference`.

Scenario: When `COPY --from=` contains a variable, it is skipped

- Given: A Dockerfile has `COPY --from=image:${TAG} /a /b`.
- When: The developer runs `dockerfile-pin check`.
- Then: The reference is reported as `SKIP` with `unexpanded variable in COPY --from`, and `run` leaves it unchanged.

Scenario: When `ONBUILD COPY --from=` names an image, it is pinned even if a stage shares the name

- Given: A Dockerfile declares `FROM alpine AS nginx` and has `ONBUILD COPY --from=nginx /a /b`.
- When: The developer runs `dockerfile-pin run --write`.
- Then: The digest of the image `nginx` is written into the `ONBUILD` line.

### Pin images mounted from

As a developer who mounts files from external images during `RUN`, I want `RUN --mount=...,from=` images to be pinned so that `check` does not pass while such an image is unpinned.

**Acceptance criteria:**

Scenario: When `RUN --mount` names an image in `from=`, the digest is written into that option

- Given: A Dockerfile has `RUN --mount=type=bind,from=nginx:1.27,source=/etc/nginx,target=/mnt cp -r /mnt /etc/nginx`.
- When: The developer runs `dockerfile-pin run --write`.
- Then: The option becomes `from=nginx:1.27@sha256:<digest>`, and the other options and the command are unchanged.

Scenario: When a `RUN` has several mounts, each `from=` is its own reference

- Given: A `RUN` has `--mount=from=nginx:1.27,target=/a` and `--mount=from=nginx:1.27,target=/b`.
- When: The developer runs `dockerfile-pin run --write`.
- Then: Both options become `from=nginx:1.27@sha256:<digest>`, each with exactly one `@sha256:`.

Scenario: When the `from` value or the whole option list is quoted, the digest is written inside the quotes

- Given: A Dockerfile has `RUN --mount=type=bind,from="nginx:1.27",target=/a true` and `RUN --mount="type=bind,from=nginx:1.27,target=/a" true`.
- When: The developer runs `dockerfile-pin run --write`.
- Then: The lines become `RUN --mount=type=bind,from="nginx:1.27@sha256:<digest>",target=/a true` and `RUN --mount="type=bind,from=nginx:1.27@sha256:<digest>,target=/a" true`.

Scenario: When `from=` names a stage, `scratch`, or contains a variable, it is skipped

- Given: A Dockerfile declares a stage `builder` and has `RUN --mount=from=builder,target=/a`, `RUN --mount=from=scratch,target=/a`, and `RUN --mount=from=image:${TAG},target=/a`.
- When: The developer runs `dockerfile-pin check`.
- Then: They are reported as `SKIP` with `stage reference`, `scratch image`, and `unexpanded variable in RUN --mount from`.

Scenario: When a mount is written in a form that is not supported, it is skipped and left unchanged

- Given: A Dockerfile has `RUN --mount=type=bind,from=nginx:1.27,target="/mnt" true`, where a value other than `from` is quoted.
- When: The developer runs `dockerfile-pin check` and then `dockerfile-pin run --write`.
- Then: `check` reports the reference as `SKIP` with `unsupported RUN --mount syntax`, and `run` leaves the line unchanged.

Scenario: When a mount has no `from=`, it yields no reference

- Given: A Dockerfile has `RUN --mount=type=cache,target=/root/.cache make`.
- When: The developer runs `dockerfile-pin check`.
- Then: Nothing is reported for that `RUN`.

### Pin references split over lines

As a developer who breaks long instructions with line continuations, I want the digest to land in the right place so that the file is not silently left unpinned.

**Acceptance criteria:**

Scenario: When a reference is split by a continuation, the digest is written where the reference ends

- Given: A Dockerfile has a `COPY` whose `--from=` value is split across two lines with `\`.
- When: The developer runs `dockerfile-pin run --write`.
- Then: The digest is inserted at the end of the reference on the line where it ends, and the continuation is kept.

Scenario: When the reference cannot be found in the source, the line is left unchanged and reported

- Given: A reference for which a digest was resolved does not appear in its instruction's source text.
- When: The developer runs `dockerfile-pin run --write`.
- Then: A `WARN` naming the file, line, and image with `reference not found in the source line, left unchanged` is printed, and that image is not counted as pinned.

## KPIs

None. dockerfile-pin does not collect usage data.

## Functional requirements

### Reading references

**Overview:** Every `FROM`, every `COPY` or `ONBUILD COPY` with a `--from=` flag, and every `--mount` flag of a `RUN` or `ONBUILD RUN` that has a `from=` option yields one reference.

**Input:** The content of a Dockerfile.

**Output:** The references in file order, each with the text as written, the image after `ARG` expansion and without digest, the existing digest, the lines the instruction spans, the instruction text, and whether it is skipped with a reason.

#### Applicable conditions

##### FROM

The reference is the first word after `FROM` and its flags. `--platform` and `AS <name>` are kept as written.
`$VAR` and `${VAR}` are replaced with the defaults of `ARG` instructions written above the `FROM`; an `ARG` written below a `FROM` does not affect it.
A name is a stage reference only when a stage with that name is declared above the `FROM`; stage names are compared without regard to case.

##### COPY --from

The flag name is matched as `--from=` exactly; `--FROM=` is not read.
A value that is an integer is a stage index. A value containing `$` is not expanded.
A name is a stage reference when a stage with that name is declared anywhere in the file.
For `ONBUILD COPY`, the stage names of this file do not apply, because the trigger runs in the build that uses this image as its base.

##### RUN --mount from

Each `--mount=` flag written before the command of a `RUN` is read as a comma-separated list of `key=value` options, and the value of `from` is the reference, whatever the mount `type` is ([Dockerfile reference](https://docs.docker.com/reference/dockerfile/#run---mount)).
The option key is matched without regard to case, so `FROM=` is read too. A mount with no `from`, or with an empty one, yields no reference, because it reads the build context or nothing.
A value containing `$` is not expanded, because BuildKit rejects a variable there.
A name is a stage reference when a stage with that name is declared anywhere in the file. An integer is not a stage index here, unlike `COPY --from=`: BuildKit looks the value up among stage names only, so `from=0` is read as an image named `0`.
For `ONBUILD RUN`, the stage names of this file do not apply, as with `ONBUILD COPY`.

Two syntaxes are pinned. In both, the options are separated by commas and there is exactly one `from` option.

- An option list: `--mount=type=bind,from=image:tag,target=/mnt`, written without quotes or escape characters. As a variant, the value of `from` may be enclosed in one pair of quotes: `from="image:tag"`.
- A quoted option list: `--mount="type=bind,from=image:tag,target=/mnt"`, where one pair of quotes encloses the whole list and no other quote appears inside.

A quote is `"` or `'`.
Any other form that BuildKit accepts, such as quotes around another value or around a whole option, escape characters, CSV quoting, or a repeated `from`, is read so that `check` can show it, but it is skipped and never rewritten.

##### Digests and stages

A reference that contains `@` already has a digest: the image is the text before the last `@` and the digest is the text after it.
A reference with a digest is never a stage reference, because the whole value, digest included, is compared with stage names.

##### Escape directive

A `# escape=` parser directive changes the line-continuation character, and it is honored when reading and writing.

#### Exceptional behavior

##### Skipped references

| Reference | Reason shown |
| --- | --- |
| `FROM` or `--from=` with no value | `missing image reference` |
| `scratch` | `scratch image` |
| A stage name | `stage reference` |
| A stage index in `COPY --from=` | `stage index reference` |
| A `FROM` variable without a default | `unresolved ARG variable` |
| A variable in `COPY --from=` | `unexpanded variable in COPY --from` |
| A variable in `RUN --mount=...,from=` | `unexpanded variable in RUN --mount from` |
| A `RUN --mount` written in neither supported syntax | `unsupported RUN --mount syntax` |

Skipped references are reported by `check` and left unchanged by `run`.

##### Unparsable Dockerfile

A file the Dockerfile parser rejects is reported as an error naming the file, and the other target files are still processed.

### Writing digests

**Overview:** The digest is written into the Dockerfile text right after the reference.

**Input:** The content, the references, and the digest chosen for each.

**Output:** The content in which each chosen reference reads `<reference as written without digest>@<digest>`, and the list of references that could not be found in the source.

#### Applicable conditions

An existing digest is replaced. For `COPY`, only the text after `--from=` is changed, so the same text elsewhere on the line stays as written.
For `RUN --mount`, only the text after `from=` inside that mount flag is changed, so the same text in another option, another mount, or the command stays as written.
Variables stay as written: `FROM image:${VERSION}` becomes `FROM image:${VERSION}@<digest>`.
All other text, including comments, continuations, and other lines, is unchanged.

## Data requirements

### Required data

- **Image reference:** The fields listed under Reading references. It exists only while the command runs.

### Relationships

Each reference belongs to one Dockerfile.

### Personal information

None.

## External service dependencies

None. Contacting registries belongs to Pinning and Checking.

## Security considerations

A reference that the build would read differently from dockerfile-pin must not be reported as pinned. The skip rules follow how BuildKit reads `FROM`, `COPY --from`, and `RUN --mount`, so that `check` does not report `OK` for a line that pulls an unpinned image.

A digest must never be written at the wrong place. The reader decides which written forms are supported, and for every other form nothing is rewritten; a form is added to the supported ones only when it is needed.

## Open questions

None.
