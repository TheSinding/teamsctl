#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
PREFIX=${PREFIX:-"${HOME}/.local"}
BIN_DIR=${BIN_DIR:-"${PREFIX}/bin"}
VERSION=${VERSION:-$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || echo dev)}

if ! command -v go >/dev/null 2>&1; then
  echo "teamsctl: Go is required to build the binary" >&2
  exit 1
fi

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Building teamsctl..."
(
  cd "$ROOT_DIR"
  go build -trimpath -ldflags="-s -w -X thesinding/teamsctl/internal/version.Value=${VERSION}" -o "$TMP_DIR/teamsctl" ./cmd/teamsctl
)

mkdir -p "$BIN_DIR"
install -m 0755 "$TMP_DIR/teamsctl" "$BIN_DIR/teamsctl"
echo "Installed teamsctl to $BIN_DIR/teamsctl"

case ":${PATH}:" in
  *":${BIN_DIR}:"*) ;;
  *) echo "Add $BIN_DIR to PATH to run teamsctl directly." ;;
esac
