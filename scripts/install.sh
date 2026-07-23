#!/bin/sh
set -eu

PREFIX=${PREFIX:-"${HOME}/.local"}
BIN_DIR=${BIN_DIR:-"${PREFIX}/bin"}

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' 0 HUP INT TERM

is_checkout() {
  [ -f "$1/go.mod" ] || return 1
  command -v git >/dev/null 2>&1 || return 1
  git -C "$1" rev-parse --is-inside-work-tree >/dev/null 2>&1 || return 1
  grep -q '^module thesinding/teamsctl$' "$1/go.mod"
}

checkout=""
if is_checkout "$PWD"; then
  checkout=$PWD
elif [ -f "$0" ]; then
  script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
  candidate=$(CDPATH= cd -- "$script_dir/.." && pwd)
  if is_checkout "$candidate"; then
    checkout=$candidate
  fi
fi

if [ -n "$checkout" ]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "teamsctl: Go is required to build from a checkout" >&2
    exit 1
  fi
  version=${VERSION:-$(git -C "$checkout" describe --tags --always --dirty 2>/dev/null || echo dev)}
  echo "Building teamsctl ${version}..."
  (
    cd "$checkout"
    go build -trimpath -ldflags="-s -w -X thesinding/teamsctl/internal/version.Value=${version}" -o "$TMP_DIR/teamsctl" ./cmd/teamsctl
  )
else
  download() {
    if command -v curl >/dev/null 2>&1; then
      curl -fsSL "$1" -o "$2"
    elif command -v wget >/dev/null 2>&1; then
      wget -O "$2" "$1"
    else
      echo "teamsctl: curl or wget is required" >&2
      exit 1
    fi
  }

  os=$(uname -s)
  case "$os" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) echo "teamsctl: unsupported operating system: $os" >&2; exit 1 ;;
  esac

  arch=$(uname -m)
  case "$arch" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) echo "teamsctl: unsupported architecture: $arch" >&2; exit 1 ;;
  esac

  download "https://api.github.com/repos/TheSinding/teamsctl/releases/latest" "$TMP_DIR/release.json"
  version=$(sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' "$TMP_DIR/release.json")
  if [ -z "$version" ]; then
    echo "teamsctl: unable to determine latest release" >&2
    exit 1
  fi

  archive="teamsctl-${version}-${os}-${arch}.tar.gz"
  release_url="https://github.com/TheSinding/teamsctl/releases/download/${version}"
  echo "Downloading teamsctl ${version} for ${os}/${arch}..."
  download "$release_url/$archive" "$TMP_DIR/$archive"
  download "$release_url/checksums.txt" "$TMP_DIR/checksums.txt"

  expected=$(awk -v archive="$archive" '$2 == archive { print $1 }' "$TMP_DIR/checksums.txt")
  if [ -z "$expected" ]; then
    echo "teamsctl: checksum missing for $archive" >&2
    exit 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$TMP_DIR/$archive" | awk '{ print $1 }')
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$TMP_DIR/$archive" | awk '{ print $1 }')
  else
    echo "teamsctl: sha256sum or shasum is required" >&2
    exit 1
  fi
  if [ "$actual" != "$expected" ]; then
    echo "teamsctl: checksum verification failed for $archive" >&2
    exit 1
  fi

  tar -xzf "$TMP_DIR/$archive" -C "$TMP_DIR"
fi

mkdir -p "$BIN_DIR"
install -m 0755 "$TMP_DIR/teamsctl" "$BIN_DIR/teamsctl"
echo "Installed teamsctl to $BIN_DIR/teamsctl"

case ":${PATH}:" in
  *":${BIN_DIR}:"*) ;;
  *) echo "Add $BIN_DIR to PATH to run teamsctl directly." ;;
esac
