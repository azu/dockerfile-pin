# Dockerfile image references design

[日本語](design.ja.md)

## About this document

This design describes how the references defined in [Dockerfile image references](requirements.md) are read and rewritten in [dockerfile-pin](../../product.md).

### Audience

People and agents who change how Dockerfiles are read or rewritten.

### Prerequisites

Dockerfile syntax, multi-stage builds, and how BuildKit resolves `FROM`, `COPY --from`, and `RUN --mount`.

### Outside this document

File discovery, digest resolution, and command output belong to their own features.

## Implementation approach

`internal/dockerfile` has a reader, `Parse`, and a rewriter, `RewriteFileReport`.
`run` and `check` call `Parse` and decide what to do with each reference; `run` passes the chosen digests to the rewriter.

### Reading

`Parse` uses BuildKit's Dockerfile parser (`github.com/moby/buildkit/frontend/dockerfile/parser`) and walks the top-level instructions in order.
Using the same parser as `docker build` gives the instruction's line span, its text with continuations joined, its flags, and the escape character, without reimplementing Dockerfile syntax.

Three pieces of state decide how a name is classified.
`ARG` defaults are collected as the walk proceeds, so a `FROM` sees only the `ARG`s above it, as Docker scopes them.
Stage names seen so far are collected the same way for `FROM`, because BuildKit resolves each `FROM` only against stages declared above it.
For `COPY --from`, all stage names in the file are collected before the walk: using a stage declared later is a build error about stage order, not an unpinned image, so there is nothing to pin.
`RUN --mount` uses the same full set of stage names, because BuildKit also resolves a mount source against all stages and reports a later one as a stage order error.

`ONBUILD` hangs its wrapped instruction on a child node whose position is unset, so the reader copies the `ONBUILD` node's line span and text onto the child and classifies it with no stage names.

Classification runs in the order BuildKit uses: `scratch`, then an exact stage-name match on the whole value, then splitting a digest at the last `@`.
For `COPY --from`, a stage index (anything `strconv.Atoi` accepts) and a `$` in the value are checked first, because BuildKit reads the value verbatim and does not expand variables there ([moby/buildkit#2374](https://github.com/moby/buildkit/issues/2374)).

For `RUN`, the parser returns each leading flag with its quotes removed, so the reader takes the flags that start with `--mount=`, splits each value as one CSV record, and uses the value of the last `from` option, with the key lowercased for comparison as BuildKit's `parseMount` does ([commands_runmount.go](https://github.com/moby/buildkit/blob/v0.16.0/frontend/dockerfile/instructions/commands_runmount.go)).
The same function rejects a `$` in `from` with `'from' doesn't support variable expansion, define alias stage instead`, which is why such a value is skipped.
It then classifies the value like `COPY --from` but without the stage index check: BuildKit calls only `findStateByName` for a mount source and otherwise builds a stage from the value as an image name ([convert_runmount.go](https://github.com/moby/buildkit/blob/v0.16.0/frontend/dockerfile/dockerfile2llb/convert_runmount.go)).
Each reference records the position of its `--mount` flag among the instruction's `--mount` flags, which the rewriter needs.

### Rewriting

The rewriter works on the file as lines and edits them in place.
For each chosen reference it joins the lines the instruction spans the way the parser does, dropping the escape character and what follows it on continued lines and leaving out comment lines between them, and records for each byte of the joined text the line and column it came from.
It searches the joined text for the reference, prefixed by `--from=` for `COPY`, then maps the position back to the lines: it removes the bytes of any existing digest and inserts `@<digest>` where the reference without digest ends.
For `RUN --mount`, the reference is located by reading the text instead of searching for it.
The rewriter splits the instruction into words, keeping quoted spaces inside a word, and takes the words between the instruction name and the first word that does not start with `--` as the flags; the `--mount=` flag with the recorded position is the one to change.
It removes the quotes and escape characters from that flag as the parser does, remembering where each remaining byte came from, because that unquoted text is what BuildKit splits at commas; quotes may enclose the whole option list, one option, or one value.
It takes the last option whose key is `from` without regard to case and writes the digest at the source position of its value.
A search for `from=<reference>` would be wrong in three cases: two mounts that name the same image, where it finds the first one twice; a repeated `from`, where BuildKit uses the last one and the first would be pinned; and the same text inside another option such as `source=`.
Searching the joined text is what makes a reference split by a continuation findable; searching each physical line would miss it and leave the file silently unchanged.

When the reference is not found, the rewriter returns the reference's index in a list of unrewritten references, so `run` can warn and leave it out of its count.

## Architecture and sequence diagrams

A diagram is unnecessary: the flow is `Parse` → caller decides digests → `RewriteFileReport`, the same shape as the YAML readers.

## Data model and ER diagrams

`FromInstruction` holds `RawRef`, `ImageRef`, `Digest`, `Platform`, `StageName`, `StartLine`, `EndLine`, `Original`, `Skip`, `SkipReason`, `IsCopyFrom`, `IsMountFrom`, `MountIndex`, and `EscapeToken`.
Skip reasons are exported constants so callers and tests do not repeat the strings.

## APIs and interfaces

`Parse(r io.Reader)` returns the references or the parser's error.
`RewriteFileReport(content, instructions, digests)` returns the new content and the indexes that could not be rewritten; `digests` is keyed by reference index. `RewriteFile` is the same without the list.

## External service integration

None. The BuildKit parser is a library dependency, chosen for accurate parsing of `FROM` lines.

## Data handling

References exist only in memory while the command runs.

## Authentication and authorization notes

None. Only local files are read and written.

## Security considerations

Each rule follows BuildKit's own reading of the instruction rather than a guess, so that a line Docker resolves from a registry is never classified as a stage and skipped.

## Deployment notes

None.

## Alternatives considered

### Expanding variables in `COPY --from`

It would pin `COPY --from=image:${TAG}` to the image the variable names.
BuildKit does not expand the value and the build fails, so a digest would be written onto a line that can never build and `check` would call it OK. It is not chosen ([#54](https://github.com/azu/dockerfile-pin/pull/54)).

### Reading mounts with BuildKit's `instructions` package

`instructions.Parse` would return each mount already parsed, with its `From` field.
It validates the whole Dockerfile and fails on instructions it does not accept, and it gives no position inside the line for the rewriter, so the flags from the parser already in use are read instead.

### Treating a number in `from=` as a stage index

It would match `COPY --from=0`. BuildKit does not read a mount source that way, so `check` would skip a value that the build pulls as an image. It is not chosen.

### Separate functions for `FROM` and `COPY --from`

Both callers want both kinds and the package is internal, so one `Parse` with `IsCopyFrom` is kept, and `RUN --mount` joins it with `IsMountFrom` ([#54](https://github.com/azu/dockerfile-pin/pull/54)).

## Open questions

None.
