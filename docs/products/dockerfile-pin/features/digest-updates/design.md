# Digest updates design

[日本語](design.ja.md)

## About this document

This design describes how [Digest updates](requirements.md) is implemented in [dockerfile-pin](../../product.md).

### Audience

People and agents who change `--update` or `--min-age`.

### Prerequisites

The three phases of `run` described in its Requirements and the OCI image configuration.

### Outside this document

The phases of `run` themselves.

## Implementation approach

Both options are conditions inside the phases of `run`.

`--update` removes one condition. A reference with a digest is normally excluded when images are collected and again when digests are assigned; with `--update` both exclusions are lifted, and the rewriters replace the text after `@`. Both places must agree: an earlier version lifted only the first, so already pinned images were resolved but never rewritten ([#22](https://github.com/azu/dockerfile-pin/pull/22)).

`--min-age` adds a step to resolution. The cutoff is the current time minus N days. After an image's digest is resolved, its creation time is read with `GetImageCreatedTime`, which fetches the image and reads `Created` from its configuration file. When that time is after the cutoff, the image is left out of the digest map and a `skipped` line is printed, so the later phases treat it like an image that failed to resolve. An error reading the time prints a warning and pins anyway, and a zero time is treated as old enough.
`GetImageCreatedTime` is a package variable so tests can replace it.

The effective period is the flag when it was given on the command line, otherwise the configuration file's `min-age` when present, otherwise 0. It is validated as non-negative before any file is read.

## Architecture and sequence diagrams

A diagram is unnecessary: the feature adds conditions to an existing flow.

## Data model and ER diagrams

No types are added. `Config.MinAge` is a pointer so that an absent key differs from 0.

## APIs and interfaces

`-u/--update` (boolean) and `--min-age` (integer days) on `run`; `min-age` in `.dockerfile-pin.yaml`.

## External service integration

Reading the creation time fetches the image manifest and configuration with the same keychain and 30 second timeout as resolution.

## Data handling

Creation times are compared and discarded.

## Authentication and authorization notes

None beyond resolution.

## Security considerations

None beyond the Requirements.

## Deployment notes

None.

## Alternatives considered

### Leaving updates entirely to Renovate

The original design listed digest updates as out of scope. `--update` was kept as a manual re-resolve, and `--min-age` was added in [#44](https://github.com/azu/dockerfile-pin/pull/44) as a cooldown; it is most useful for floating tags such as `nginx:latest`, since specific patch tags are rarely pushed again.

## Open questions

None.
