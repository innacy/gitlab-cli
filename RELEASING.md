# Releasing gitlab-cli

Releases are automatic. Merge to `master` and the version, the tag and the
release notes follow from the commits. **Do not create tags by hand.**

## How a release happens

Every push to `master` runs `.github/workflows/ci.yml`:

1. **test**: `go vet` and `go test -race`, on the Go version in `go.mod` and
   on the latest stable Go. It also checks `gofmt` and builds the `gitlab-ai`
   binary.
2. **release**, only if `test` passed:
   [semantic-release](https://github.com/semantic-release/semantic-release)
   reads the commits since the last tag and decides the next version. It then
   pushes the tag `vX.Y.Z` and creates a
   [GitHub release](https://github.com/innacy/gitlab-cli/releases) whose
   notes are generated from those commits.

Install from source against a release tag:

```bash
go install gitlab-ai@vX.Y.Z
# or, from a clone:
git checkout vX.Y.Z && make install
```

## How the version is decided

semantic-release takes the largest bump any commit since the last tag asks
for ([Conventional Commits](https://www.conventionalcommits.org/)):

| Commit | Example | Release |
| --- | --- | --- |
| `feat` | `feat(ship): add fallback model (#12)` | **minor**, 1.2.0 → 1.3.0 |
| `fix`, `perf` | `fix(dashboard): keep prompt hidden (#1)` | **patch**, 1.2.0 → 1.2.1 |
| `!` after the type, or a `BREAKING CHANGE:` footer | `feat(repl)!: rename mr-status (#20)` | **major**, 1.2.0 → 2.0.0 |
| `docs`, `test`, `ci`, `chore`, `refactor`, `build`, `style` | `docs(readme): fix example` | none |

A push that contains only non-releasing commits produces no release. The job
still passes.

**The commit type is a release decision.** A change a caller can notice is a
`fix` or a `feat`, never a `chore` or a `refactor`. Otherwise it does not ship
until something else triggers a release.

Every commit on `master` counts, including each commit of a merged pull
request, not only its title. If a branch's history does not describe what
should be released, squash-merge it and give it a conventional title.

## Why tags must not be made by hand

semantic-release only ever tags a commit that passed the tests, with the next
version in sequence. Hand-made tags skip that guarantee and can collide with
the next automated version.

## Setup

Nothing to configure. The job uses the workflow's built-in `GITHUB_TOKEN`
with `contents: write`, and no secrets. A tag pushed with that token does not
trigger further workflows.

Release comments on issues and pull requests, and "released" labels, are
turned off in `.releaserc.json`, so the job needs no permissions beyond
`contents: write`.

## First release

There are no `v*` tags yet. The next push to `master` that contains a
`feat` / `fix` / `perf` / breaking commit will create `v1.0.0` (or whatever
the analyzer decides from the full history). If you want the first automated
version to start from a specific number, create that bootstrap tag once on
the current tip before merging releasing commits:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## Verifying a release

```bash
gh release view vX.Y.Z --repo innacy/gitlab-cli
git fetch --tags && git tag -l 'v*'
```

## Pinned versions

The actions are pinned by commit SHA and the npm packages by exact version:
`semantic-release@25.0.9` and `conventional-changelog-conventionalcommits@9.3.1`.
semantic-release's own plugins resolve within the ranges that version
declares. Upgrade deliberately, and let the next release prove it.

The preset and semantic-release have to be upgraded together.
`conventional-changelog-conventionalcommits` 10.x renders only with
`conventional-changelog-writer` 9. semantic-release 25.0.9 ships
`@semantic-release/release-notes-generator` 14, which loads writer 8, so a 10.x
preset fails at "generateNotes" before anything is tagged.

To test an upgrade locally, run a dry run with the real `.releaserc.json`.
Do not override plugins with `--plugins`: that drops the preset options and
tests the default preset instead.
