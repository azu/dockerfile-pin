# Target file discovery design

[日本語](design.ja.md)

## About this document

This design describes how [Target file discovery](requirements.md) is implemented in [dockerfile-pin](../../product.md).

### Audience

People and agents who change which files are found or how their kind is decided.

### Prerequisites

Glob patterns and `git ls-files`.

### Outside this document

Reading the files belongs to the image reference features.

## Implementation approach

`cmd/files.go` has two functions that `run` and `check` both call: `FindFiles` returns the paths, and `DetectFileType` maps one path to a kind.

### Finding files

`FindFiles` handles the three sources in order of precedence.
With `-f`, it checks that the file exists and returns it alone.
With `--glob`, it expands the pattern with `doublestar.FilepathGlob` and fails when nothing matches.
Otherwise it runs `git ls-files --cached --others --exclude-standard` and keeps the lines that match the default pattern, which is how `.gitignore` is respected.
When that command fails or yields nothing, it walks the current directory with the same pattern and drops paths that have `node_modules` or `vendor` as a segment.

The default search has alternated between git and a filesystem glob: `git ls-files` replaced a hardcoded skip list, then [#19](https://github.com/azu/dockerfile-pin/pull/19) proposed removing the dependency on a git repository. The current code keeps both, git first and the glob as the fallback.

### Deciding the kind

`DetectFileType` normalizes the path to forward slashes, lowercases the base name, and applies the rules in the order listed in the Requirements.
The compose names are checked before the GitLab component and `.gitlab/ci` layouts, because those layouts key on directory names that other tools also use, so a `templates/docker-compose.yml` stays a compose file.
The component layouts are matched from the root with `doublestar.Match` on the cleaned path, because GitLab publishes components only from a `templates` directory at the repository root.
Only `.gitlab-ci.yml` is in the default pattern, because `templates` is an ordinary directory name and `.gitlab/ci` is a convention GitLab does not define ([#49](https://github.com/azu/dockerfile-pin/pull/49)).

## Architecture and sequence diagrams

A diagram is unnecessary: two pure functions are called in sequence.

## Data model and ER diagrams

`FileType` is an enumeration: `FileTypeDockerfile`, `FileTypeCompose`, `FileTypeActions`, `FileTypeGitLab`.

## APIs and interfaces

`FindFiles(filePath, globPattern string) ([]string, error)` and `DetectFileType(path string) FileType`.
The errors are `file not found: <path>`, `invalid glob pattern`, `no files matched pattern: <pattern>`, and the nothing-found message of the default search.

## External service integration

None. `git` is run as a local command and its absence is not an error.

## Data handling

Paths are held in memory only.

## Authentication and authorization notes

None.

## Security considerations

None beyond the Requirements.

## Deployment notes

None.

## Alternatives considered

### Glob only, without git

Proposed in [#19](https://github.com/azu/dockerfile-pin/pull/19) to remove the dependency on a git repository; it does not respect `.gitignore`. The current code uses the glob only as the fallback.

### Deciding the kind from content

Not chosen. The places where these files live are fixed by their platforms, such as `.github/workflows/` for GitHub Actions, so the path already tells the kind, and reading the content of every file to guess it would be search work with no gain.

## Open questions

None.
