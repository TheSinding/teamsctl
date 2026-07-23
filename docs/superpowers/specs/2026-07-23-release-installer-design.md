# Release Installer Design

## Goal

Support both local source installs and the conventional streamed install:

```sh
wget -qO- https://raw.githubusercontent.com/TheSinding/teamsctl/main/scripts/install.sh | sh
```

## Behavior

- Install to `${HOME}/.local/bin/teamsctl` by default, respecting `PREFIX` and `BIN_DIR` overrides.
- When invoked from a teamsctl Git checkout, build the current checkout with Go.
- Otherwise, detect Linux or macOS and amd64 or arm64, find the latest GitHub release, and download its matching archive.
- Download `checksums.txt` and verify the archive with `sha256sum` or `shasum -a 256` before extracting it.
- Require Go only for checkout builds. Require either `curl` or `wget` for release installs.
- Use POSIX shell syntax so piping into `sh` works.
- Fail clearly for unsupported platforms, missing tools, unavailable releases, or checksum mismatches.

## Documentation

- Add the streamed installation command.
- Use the default installed location `${HOME}/.local/bin/teamsctl` in MCP setup examples instead of `/absolute/path/to/teamsctl` placeholders.

## Verification

- Validate shell syntax with `sh -n`.
- Run a checkout build into a temporary prefix and verify its version command.
- Pipe the script into `sh` from outside the checkout, install the latest release into a temporary prefix, and verify its version command.
- Run the Go test and vet suite.
