# Homebrew Tap Design

## Goal

Publish `teamsctl` through a public `TheSinding/homebrew-tap` repository so users can install it with:

```sh
brew install TheSinding/tap/teamsctl
```

## Release

- Add the standard Unlicense text to the `teamsctl` repository.
- Commit the license and this design on `main`.
- Create and push annotated tag `0.3`.
- Wait for the existing release workflow to publish the `0.3` release successfully.

## Tap

- Create the public GitHub repository `TheSinding/homebrew-tap` using Homebrew's standard tap structure.
- Add `Formula/teamsctl.rb` targeting the immutable `0.3` source archive and its SHA-256 checksum.
- Declare `license "Unlicense"` and `depends_on "go" => :build`.
- Build `./cmd/teamsctl` from source with the release version injected through the existing linker variable.
- Test that the installed binary reports version `0.3`.
- Document installation and upgrade commands in the tap README.

## Verification

- Run the `teamsctl` Go tests and vet before tagging.
- Confirm the `0.3` GitHub release completes.
- Run Homebrew syntax and audit checks on the formula.
- Install from source through the tap, run `brew test`, verify `teamsctl version`, and uninstall it.
- Push the tap only after local checks pass.

## Deferred

- Bottles and automated formula updates remain deferred until usage justifies their maintenance.
- Submission to `homebrew/core` remains deferred until the project meets its age, notability, and dependency requirements.
