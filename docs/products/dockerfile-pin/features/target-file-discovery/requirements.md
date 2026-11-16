# Target file discovery

[日本語](requirements.ja.md)

## About this document

This feature decides which files `run` and `check` read, and what kind each file is.
It belongs to [dockerfile-pin](../../product.md).

### Scope

- Choosing files from `-f`, `--glob`, or the default search.
- Deciding whether a file is a Dockerfile, a compose file, a GitHub Actions file, or a GitLab CI file from its path.

### Out of scope

- Reading references from a file, which belongs to [Dockerfile image references](../dockerfile-image-references/requirements.md) and [YAML image references](../yaml-image-references/requirements.md).
- Looking at a file's content to decide its kind. The kind is decided from the path only.

## User stories

### Check a repository without naming files

As a developer, I want `check` and `run` to find the target files by themselves so that I can run them in CI without listing files.

**Acceptance criteria:**

Scenario: When no file option is given in a git repository, tracked and untracked files that are not ignored are searched

- Given: A git repository has `Dockerfile`, `services/api/Dockerfile.dev`, `docker-compose.yml`, `.github/workflows/ci.yml`, `.gitlab-ci.yml`, and a `Dockerfile` in a directory listed in `.gitignore`.
- When: The developer runs `dockerfile-pin check` at the repository root.
- Then: The first five files are read, and the ignored `Dockerfile` is not.

Scenario: When no file option is given outside a git repository, the default pattern is searched from the current directory

- Given: A directory that is not a git repository has `Dockerfile` and `node_modules/pkg/Dockerfile`.
- When: The developer runs `dockerfile-pin check`.
- Then: `Dockerfile` is read, and the one under `node_modules` is not.

Scenario: When nothing is found, the command fails with a message

- Given: A directory has no file matching the default pattern.
- When: The developer runs `dockerfile-pin check`.
- Then: The command exits with an error saying `no Dockerfiles, compose files, GitHub Actions files, or GitLab CI files found`.

### Name the files to read

As a developer, I want to name one file or a glob pattern so that I can read files the default search does not find.

**Acceptance criteria:**

Scenario: When `-f` names a file, only that file is read

- Given: `path/to/Dockerfile` exists.
- When: The developer runs `dockerfile-pin run -f path/to/Dockerfile`.
- Then: Only that file is read, even if `--glob` is also given.

Scenario: When `-f` names a missing file, the command fails

- Given: `missing.yml` does not exist.
- When: The developer runs `dockerfile-pin check -f missing.yml`.
- Then: The command exits with `file not found: missing.yml`.

Scenario: When `--glob` matches files, those files are read

- Given: Several directories have `Dockerfile` and `Dockerfile.prod`.
- When: The developer runs `dockerfile-pin check --glob '**/{Dockerfile,Dockerfile.*}'`.
- Then: Every matching file is read; a pattern that matches nothing exits with `no files matched pattern`.

### Have each file read as the right kind

As a developer, I want a file's kind decided from its path so that each file is read with the right rules.

**Acceptance criteria:**

Scenario: When a path is given, its kind follows the rules in order

- Given: The paths `.github/workflows/ci.yml`, `action.yml`, `ci/build.gitlab-ci.yml`, `templates/docker-compose.yml`, `templates/deploy.yml`, `.gitlab/ci/test.yml`, `stack.yml`, and `Dockerfile.dev`.
- When: Each is passed with `-f`.
- Then: They are read as GitHub Actions, GitHub Actions, GitLab CI, compose, GitLab CI, GitLab CI, compose, and Dockerfile respectively.

## KPIs

None. dockerfile-pin does not collect usage data.

## Functional requirements

### Choosing files

**Overview:** `-f` wins over `--glob`, and `--glob` wins over the default search.

**Input:** The `-f` and `--glob` options and the current directory.

**Output:** The list of files, or an error.

#### Applicable conditions

##### Default search

The default pattern is `**/{Dockerfile,Dockerfile.*,docker-compose*.yml,docker-compose*.yaml,compose.yml,compose.yaml,action.yml,action.yaml,.gitlab-ci.yml,.github/workflows/*.yml,.github/workflows/*.yaml}`.
Inside a git repository, the files git knows and the untracked files that `.gitignore` does not exclude are filtered by this pattern.
When git is unavailable, the directory is not a repository, or that search finds nothing, the pattern is searched on the filesystem from the current directory, skipping `node_modules` and `vendor`.

##### Glob syntax

`--glob` uses doublestar syntax, with `**` and brace expansion.

### Deciding the kind

**Overview:** The first matching rule decides the kind.

**Input:** A file path.

**Output:** One of Dockerfile, compose, GitHub Actions, GitLab CI.

#### Applicable conditions

| Order | Path | Kind |
| --- | --- | --- |
| 1 | `.yml` or `.yaml` under `.github/workflows/` | GitHub Actions |
| 2 | `action.yml` or `action.yaml` | GitHub Actions |
| 3 | Name ending in `.gitlab-ci.yml` | GitLab CI |
| 4 | `compose.yml`, `compose.yaml`, `docker-compose*.yml`, `docker-compose*.yaml` | compose |
| 5 | `templates/<name>.yml` or `templates/<name>/template.yml`, from the root | GitLab CI |
| 6 | `.yml` or `.yaml` under `.gitlab/ci/` | GitLab CI |
| 7 | Any other `.yml` or `.yaml` | compose |
| 8 | Anything else | Dockerfile |

File names are compared without regard to case.
The component rows (5) match only a path relative to the repository root, not an absolute path or one containing `..`.
Only `.gitlab-ci.yml` is in the default search; rows 3, 5, and 6 apply to files named with `-f` or `--glob`.

## Data requirements

### Required data

- **Target file:** Its path and kind. Nothing is stored.

### Relationships

None.

### Personal information

None.

## External service dependencies

None. The `git` command is used when present, and the search works without it.

## Security considerations

The default search does not read files that `.gitignore` excludes, such as dependency directories.

## Open questions

None.
