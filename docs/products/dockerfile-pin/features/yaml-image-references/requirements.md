# YAML image references

[日本語](requirements.ja.md)

## About this document

This feature reads Docker image references from compose files, GitHub Actions files, and GitLab CI files, and writes a digest onto each reference at the place where it is written.
`run` pins the references it reads, and `check` reports on them.
It belongs to [dockerfile-pin](../../product.md).

### Scope

- The YAML positions read in each kind of file, and the references skipped there.
- How a reference that already has a digest is recognized.
- How a digest is written into the file text.
- How YAML anchors and aliases affect which references are read.

### Out of scope

- Finding target files and deciding their kind, which belongs to Target file discovery.
- Resolving digests from the registry, the dry-run and `--write` output of `run`, and the statuses and output of `check`, which belong to Pinning and Checking.
- Ignore patterns, `--update`, and `--min-age`, which belong to their own features.
- Dockerfiles, which belong to Dockerfile image references.
- Expanding variables. Compose `${VAR}` and GitHub Actions `${{ }}` expressions are read as written; GitLab CI values containing `$` are skipped as described below.

## User stories

### Pin the images of compose services

As a developer who maintains compose files, I want `run` to add digests to the `image` of each service so that the services keep using the images I pinned.

**Acceptance criteria:**

Scenario: When a service has an `image`, the digest is written after the tag

- Given: A compose file has `services.web.image: node:20.11.1`.
- When: The developer runs `dockerfile-pin run --write -f docker-compose.yml`.
- Then: The line becomes `image: node:20.11.1@sha256:<digest>`, and every other line of the file is unchanged.

Scenario: When a service also has `build`, its image is skipped

- Given: A service has both `build: .` and `image: myapp:latest`.
- When: The developer runs `dockerfile-pin check`.
- Then: The reference is reported as `SKIP` with `has build directive`, and `run` leaves the line unchanged.

Scenario: When a service has no `image`, nothing is reported for it

- Given: A service has only `build: .`.
- When: The developer runs `dockerfile-pin check`.
- Then: No result is printed for that service.

### Pin the images of GitHub Actions jobs and actions

As a developer who maintains GitHub Actions files, I want the container, service, and `docker://` step images to be pinned so that jobs run on the images I pinned.

**Acceptance criteria:**

Scenario: When a workflow names images in the read positions, each gets a digest

- Given: A workflow has `jobs.test.container.image: node:24`, `jobs.test.services.db.image: postgres:18`, and a step `uses: docker://ghcr.io/astral-sh/uv:latest`.
- When: The developer runs `dockerfile-pin run --write`.
- Then: The three values become `node:24@sha256:<digest>`, `postgres:18@sha256:<digest>`, and `docker://ghcr.io/astral-sh/uv:latest@sha256:<digest>`, keeping the `docker://` prefix.

Scenario: When a step uses an action that is not a Docker image, it is not read

- Given: A step has `uses: actions/checkout@v4`.
- When: The developer runs `dockerfile-pin check`.
- Then: No result is printed for that step, and `run` leaves it unchanged.

Scenario: When an action builds from a local Dockerfile, it is skipped

- Given: An `action.yml` has `runs.image: Dockerfile`.
- When: The developer runs `dockerfile-pin check`.
- Then: The reference is reported as `SKIP` with `local Dockerfile`.

### Pin the images of GitLab CI jobs

As a developer who maintains GitLab CI files, I want the `image` and `services` of jobs, `default`, and hidden jobs to be pinned so that pipelines run on the images I pinned.

**Acceptance criteria:**

Scenario: When a job names images as strings or mappings, each gets a digest

- Given: A file has `default.image.name: node:24`, and a job `test` with `services: [postgres:18, {name: redis:7, alias: cache}]`.
- When: The developer runs `dockerfile-pin run --write`.
- Then: `node:24`, `postgres:18`, and `redis:7` each get `@sha256:<digest>` at the place they are written.

Scenario: When an image is built from a CI variable, it is skipped

- Given: A job has `image: $CI_REGISTRY_IMAGE:latest`.
- When: The developer runs `dockerfile-pin check`.
- Then: The reference is reported as `SKIP` with `contains CI variable`, and `run` leaves it unchanged.

Scenario: When a CI component has a `spec:` header document, the jobs after it are read

- Given: A file starts with a `spec:` document, followed by `---` and a document with jobs.
- When: The developer runs `dockerfile-pin check`.
- Then: The images of the jobs in the second document are reported.

### Pin several references written on one line

As a developer who writes YAML in flow style, I want each reference on a line to get its own digest so that a reference that is a prefix of another is not corrupted.

**Acceptance criteria:**

Scenario: When two references on one line share a prefix, each gets its own digest

- Given: A GitLab CI job has `services: [postgres:18, postgres]`.
- When: The developer runs `dockerfile-pin run --write`.
- Then: The line becomes `services: [postgres:18@sha256:<digest of postgres:18>, postgres@sha256:<digest of postgres>]`.

### Read references written with YAML anchors

As a developer who shares settings with YAML anchors, I want to know which references are read so that I can tell whether an image is pinned.

**Acceptance criteria:**

Scenario: When an image in a read position carries an anchor, it is read there

- Given: A workflow has `jobs.a.container: &c {image: node:20}` and `jobs.b.container: *c`.
- When: The developer runs `dockerfile-pin check`.
- Then: One result is printed for the line of `jobs.a.container`, and none for `jobs.b`.

Scenario: When an image is reached only through a merge key, it is not read

- Given: A compose file has `x-alpine: &alpine` with `image: alpine:latest`, and a service `alpine-template` with only `<<: *alpine`.
- When: The developer runs `dockerfile-pin check`.
- Then: No result is printed for `alpine-template`, and `run` leaves the `x-alpine` line unchanged.

## KPIs

None. dockerfile-pin does not collect usage data, so no indicator of this feature is measured.

## Functional requirements

### Read positions

**Overview:** Each kind of file is read at fixed positions, and every scalar found there is one reference.

**Input:** The content of a target file whose kind is already decided.

**Output:** The references, each with the text as written, the image without its digest, the existing digest if any, its line and column, and whether it is skipped with a reason.

#### Applicable conditions

##### Compose files

The `image` of each entry under `services` is read.
Positions outside `services`, such as top-level `x-*` extension fields, are not read.

##### GitHub Actions files

A file with top-level `jobs` is read as a workflow, and a file with top-level `runs` is read as an action.
In a workflow, each job's `container` (a string or the `image` of a mapping), the `image` of each entry under `services`, and each step's `uses` that starts with `docker://` are read.
In an action, `runs.image` is read.

##### GitLab CI files

Every YAML document in the file is read.
The root's `image` and `services`, and the `image` and `services` of every other root mapping are read, including `default` and hidden jobs whose names start with `.`.
The global keywords `variables`, `workflow`, `include`, `stages`, and `spec` are not read as jobs.
`image` is either a string or a mapping whose `name` is read; each `services` entry is either a string or a mapping whose `name` is read.

##### Anchors and aliases

A scalar in a read position that carries an anchor (`&name`) is read there like any other scalar.
Aliases (`*name`) and merge keys (`<<`) are not followed, so an image is read only where its text is written in a read position.

#### Exceptional behavior

##### Skipped references

A compose service that has `build` is skipped with `has build directive`.
An action whose `runs.image` does not start with `docker://` is skipped with `local Dockerfile`.
A GitLab CI value containing `$` is skipped with `contains CI variable`.
Skipped references are reported by `check` and left unchanged by `run`.

##### Invalid YAML

A file that is not valid YAML is reported as an error naming the file, and the other target files are still processed.

### Existing digests

**Overview:** A reference that contains `@` is treated as already having a digest.

**Input:** The text of a reference.

**Output:** The image is the text before the first `@`, and the digest is the text after it.

### Writing digests

**Overview:** A digest is written into the text of the file at the place each reference is written.

**Input:** The file content, the references read from it, and the digest chosen for each reference.

**Output:** The file content in which each chosen reference reads `<image>@<digest>`.

#### Applicable conditions

##### What changes

Only the reference text changes. An existing digest is replaced, the `docker://` prefix is kept, and quoting, comments, indentation, and every other line stay as written.

##### References on one line

When several references share a line, each is changed at the column where it is written, so one reference's text never receives another reference's digest.

## Data requirements

### Required data

- **Image reference:** The text as written, the image without its digest, the existing digest, the line and column where the text starts, a readable location such as `jobs.test.container.image`, and a skip reason when skipped. It exists only while the command runs.

### Relationships

Each image reference belongs to one target file. `run` matches references to resolved digests by their image without digest.

### Personal information

None. The feature reads image names from files in the repository.

## External service dependencies

None. Contacting registries belongs to Pinning and Checking.

## Security considerations

`run --write` changes only the reference text, so the meaning of the rest of the file, such as commands and credentials referenced by CI, does not change.

## Open questions

### An alias used directly as a compose image

- **What to confirm:** In a compose file, `image: *img` is read as an image named `img`, the anchor's name, and `run --write` would write `*img@sha256:<digest>` when an image with that name resolves, which breaks the alias. Whether this is intended.
- **Candidate and its sources:** Unintended. The GitHub Actions and GitLab CI readers read only scalars and ignore an alias in the same position, and the compose reader does not check the kind of node.
- **Until decided:** The body does not describe this case.
- **Who decides:** The maintainer (azu).
