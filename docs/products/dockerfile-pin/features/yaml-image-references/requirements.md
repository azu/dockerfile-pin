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
- How references reached through anchors, aliases, and merge keys are read and changed.

### Out of scope

- Finding target files and deciding their kind, which belongs to Target file discovery.
- Resolving digests from the registry, the dry-run and `--write` output of `run`, and the statuses and output of `check`, which belong to Pinning and Checking.
- Ignore patterns, `--update`, and `--min-age`, which belong to their own features.
- Dockerfiles, which belong to Dockerfile image references.
- Expanding variables. Compose `${VAR}` and GitHub Actions `${{ }}` expressions are read as written; GitLab CI values containing `$` are skipped as described below.
- Anchored mappings that no read position uses, such as an `x-*` template that no service merges. Their images are not read.

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

### Pin images shared through YAML anchors

As a developer who shares settings with YAML anchors and merge keys, I want the images that services and jobs receive through them to be checked and pinned so that a shared template does not let an unpinned image pass.

**Acceptance criteria:**

Scenario: When a service receives its image through a merge key, the image is checked where it is written

- Given: A compose file has `x-alpine: &alpine` on line 1 with `image: alpine:latest` on line 2, and a service `alpine-template` with only `<<: *alpine`.
- When: The developer runs `dockerfile-pin check -f test.yml`.
- Then: `alpine:latest` is reported as `FAIL` with `missing digest` on line 2.

Scenario: When a service receives its image through a merge key, the digest is written in the template

- Given: The same compose file as above.
- When: The developer runs `dockerfile-pin run --write -f test.yml`.
- Then: Line 2 becomes `image: alpine:latest@sha256:<digest>`, and the `<<: *alpine` line and the rest of the file are unchanged.

Scenario: When several services merge the same template, the digest is written once

- Given: Services `a` and `b` both have only `<<: *alpine`.
- When: The developer runs `dockerfile-pin run --write`.
- Then: Line 2 becomes `image: alpine:latest@sha256:<digest>` with exactly one `@sha256:` in it.

Scenario: When several services merge the same template, `check` prints one result for the definition

- Given: Services `a` and `b` both have only `<<: *alpine`, and line 2 has `image: alpine:latest`.
- When: The developer runs `dockerfile-pin check`.
- Then: Exactly one result is printed for `alpine:latest`, and it names line 2.

Scenario: When a service writes its own `image` next to a merge key, its own image is used

- Given: A service has `<<: *alpine` and `image: node:20`, and no other service merges `*alpine`.
- When: The developer runs `dockerfile-pin check`.
- Then: `node:20` is reported for that service, and nothing is reported for line 2.

Scenario: When a merge key lists several templates, the earlier template wins

- Given: Templates `&a` with `image: a:1` and `&b` with `image: b:1`, and a service with only `<<: [*a, *b]`.
- When: The developer runs `dockerfile-pin check`.
- Then: `a:1` is reported on the line where it is written, and `b:1` is not reported.

Scenario: When a value is an alias, the image is read where its anchor is written

- Given: A workflow has `jobs.a.container: &c {image: node:20}` and `jobs.b.container: *c`, or a compose file has `x-img: &img alpine:3.20` and a service with `image: *img`.
- When: The developer runs `dockerfile-pin run --write`.
- Then: The digest is written after `node:20` or `alpine:3.20` on the anchor's line, and the line with the alias is unchanged.

Scenario: When a service receives `build` through a merge key, its image is skipped

- Given: A template `&built` has `build: .`, and a service has `<<: *built` and `image: myapp:latest`.
- When: The developer runs `dockerfile-pin check`.
- Then: `myapp:latest` is reported as `SKIP` with `has build directive`.

Scenario: When no read position uses a template, its image is not read

- Given: A compose file has `x-unused: &unused` with `image: alpine:latest`, and no service merges or aliases it.
- When: The developer runs `dockerfile-pin check`.
- Then: No result is printed for that line.

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

##### Anchors, aliases, and merge keys

The read positions are looked up in the YAML as compose and GitLab CI interpret it.
An alias (`*name`) stands for the node its anchor (`&name`) marks, both as a whole value and as a service, job, or mapping.
A merge key (`<<`) adds the keys of the mapping it names, or of each mapping in the sequence it names, to the mapping that holds it, following the [YAML merge key type](https://yaml.org/type/merge.html): the mapping's own keys win, and an earlier mapping in the sequence wins over a later one.
Merged mappings may themselves contain merge keys.
This applies to `build` in compose files as well as to the image positions.

A reference reached through aliases or merge keys is reported and changed at the line and column where its text is written, which is inside the anchored node.
When one written reference is reached from several services or jobs, it is one reference: `check` prints one result for it, at the position of the definition, because a result names the place to fix, and its digest is written once.
Whether a shared reference is skipped is decided from the merged result of each use: when at least one service or job that reaches it would be checked, for example a compose service without `build`, the reference is checked and pinned, and the result still names the definition.

The same lookup applies to GitHub Actions files.
GitHub Actions accepts anchors and aliases but does not document merge keys ([changelog](https://github.blog/changelog/2025-09-18-actions-yaml-anchors-and-non-public-workflow-templates/), [documentation](https://docs.github.com/en/actions/reference/workflows-and-actions/reusing-workflow-configurations#yaml-anchors-and-aliases)), so in practice only aliases occur there.

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

None.
