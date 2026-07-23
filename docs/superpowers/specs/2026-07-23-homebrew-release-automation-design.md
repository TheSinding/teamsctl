# Homebrew Release Automation Design

## Goal

Update `TheSinding/homebrew-tap` automatically after every successful `teamsctl` release.

## Authentication

- Generate a dedicated Ed25519 deploy key.
- Register only its public key on `TheSinding/homebrew-tap` with write access.
- Store its private key in the `teamsctl` repository secret `HOMEBREW_TAP_DEPLOY_KEY`.
- Delete all local key material after access is verified.
- Do not use a personal access token; the deploy key must be scoped to the tap repository only.

## Workflow

- Add an `update-homebrew` job to `.github/workflows/release.yml`.
- Run it only after the existing release job succeeds.
- Checkout `TheSinding/homebrew-tap` with the deploy key.
- Download the immutable source archive for `github.ref_name` with retry handling.
- Calculate its SHA-256 checksum.
- Replace the formula's `url` and `sha256` fields.
- Commit as `github-actions[bot]` and push to the tap's `main` branch.
- Treat download, checksum, checkout, commit, or push failures as release-workflow failures.
- Exit successfully without a commit if the formula already contains the release version and checksum.

## Verification

- Validate workflow syntax with `actionlint`.
- Simulate the formula replacement locally and inspect the resulting diff.
- Verify the deploy key can read the tap repository before storing it.
- Open a pull request, review its complete diff, and merge only after checks pass.

## Deferred

- Bottle generation and automated tap pull requests remain out of scope.
