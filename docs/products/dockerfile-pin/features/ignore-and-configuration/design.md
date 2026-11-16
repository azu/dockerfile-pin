# Ignore rules and configuration design

[日本語](design.ja.md)

## About this document

This design describes how [Ignore rules and configuration](requirements.md) is implemented in [dockerfile-pin](../../product.md).

### Audience

People and agents who change ignore patterns or the configuration file.

### Prerequisites

doublestar glob syntax.

### Outside this document

How `run` and `check` use the result.

## Implementation approach

`cmd/ignore.go` holds four small functions that `run` and `check` call in the same order at start-up.

`LoadConfig` tries `.dockerfile-pin.yaml` and then `.dockerfile-pin.yml` in the current directory, decodes the first that exists into `Config`, and returns an empty `Config` when neither exists.
`MergeIgnorePatterns` appends the command-line patterns after the file's patterns. Because matching is last-match-wins, this order is what gives the command line precedence.
`ValidatePatterns` strips a leading `!` and tries each pattern against an empty string, so a malformed pattern stops the command before any file is read.
`IsIgnored` walks the patterns in order and, on each match, sets the answer to ignored, or to not ignored for a negation; the final answer is returned. This is the same model as `.gitignore`.

The callers pass the image without digest. `check` reports an ignored reference as `SKIP` with `ignored`; `run` leaves it out of the images to resolve, so it never receives a digest.

Glob matching replaced substring matching in [#30](https://github.com/azu/dockerfile-pin/pull/30), which also added the configuration file, added `--ignore-images` to `run`, and made each use of the flag one pattern instead of a comma-separated list.

## Architecture and sequence diagrams

A diagram is unnecessary: the functions are called in sequence.

## Data model and ER diagrams

`Config` has `IgnoreImages []string` (`ignore-images`) and `MinAge *int` (`min-age`).

## APIs and interfaces

`--ignore-images <pattern>` (repeatable) on `run` and `check`, and the two keys of the configuration file.
Errors: `reading <file>`, `parsing <file>`, and `invalid ignore pattern "<pattern>"`.

## External service integration

None.

## Data handling

The configuration is read once per command and held in memory.

## Authentication and authorization notes

None.

## Security considerations

None beyond the Requirements.

## Deployment notes

None.

## Alternatives considered

### Substring matching

The first implementation ignored an image when it contained the given text. It could not express registries or organizations precisely and had no way to make exceptions, and it was replaced by glob patterns with negation ([#30](https://github.com/azu/dockerfile-pin/pull/30)).

## Open questions

None.
