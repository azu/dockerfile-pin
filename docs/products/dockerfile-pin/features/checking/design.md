# Checking design

[日本語](design.ja.md)

## About this document

This design describes how [Checking](requirements.md) is implemented in [dockerfile-pin](../../product.md).

### Audience

People and agents who change the `check` command.

### Prerequisites

The image reference readers and the registry resolver.

### Outside this document

Reading references, file discovery, and ignore rules.

## Implementation approach

`check` (`cmd/check.go`) works in three phases, like `run`, so that all registry requests run in parallel after parsing.

In the first phase, one function per file kind reads the file with that kind's reader and turns each reference into a `CheckResult`, applying rows 1 to 4 of the status table. A reference that needs the registry gets the temporary status `pending`, with its digest stored in the message field. The original text is the Dockerfile instruction, or the key and value for YAML: `image: ` for compose, the last segment of the location for GitHub Actions, and the key or a sequence dash for GitLab CI.

In the second phase, each pending result is verified concurrently, at most twice the number of CPUs at a time. Each goroutine owns its own result, so no lock is needed. Verification asks the resolver whether `<image>@<digest>` exists, and with `--verify-tag` then resolves `<image>` and compares digests.
The resolver is a `CachedResolver`, which remembers successful `Resolve` and `Exists` answers in separate maps for the run. Errors are not cached, so a transient failure on one reference does not become the answer for the same image elsewhere ([#41](https://github.com/azu/dockerfile-pin/pull/41)).
`Exists` returns false only for a 404 from the registry; any other error is returned as an error, which becomes `WARN`.

In the third phase the results are printed as text or JSON, and the process exits with the configured code when any result failed.

## Architecture and sequence diagrams

A diagram is unnecessary: the phases mirror `run`, with `Exists` in place of `Resolve` and printing in place of rewriting.

## Data model and ER diagrams

`CheckResult` has `File`, `Line`, `Image`, `Status`, `Message`, and `Original`, serialized with lowercase JSON keys.

## APIs and interfaces

Flags: `-f/--file`, `--glob`, `--syntax-only`, `--format` (`text` or `json`; any value other than `json` prints text), `--ignore-images`, `--exit-code`, `--verify-tag`.
The text line format is the status padded to five characters, `file:line`, the original padded to fifty characters, and the message.

## External service integration

Registries are reached with HEAD requests through go-containerregistry, with a 30 second timeout per request inside a 10 minute limit for the run.

## Data handling

Results are held in memory and printed.

## Authentication and authorization notes

The Docker credential configuration is used, as in `run`.

## Security considerations

A registry error is `WARN` rather than `FAIL` or `OK`, so an outage neither blocks CI nor reports an unverified digest as verified.

## Deployment notes

None.

## Alternatives considered

### Making `--verify-tag` the default

Tags are mutable and Docker Official Images are rebuilt when their base images change, so the check would fail without any change in the repository. It is opt-in ([#57](https://github.com/azu/dockerfile-pin/pull/57)).

## Open questions

None.
