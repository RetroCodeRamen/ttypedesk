#!/usr/bin/env bash
# One-line installer:
#   curl -fsSL https://raw.githubusercontent.com/RetroCodeRamen/ttypedesk/master/install.sh | bash
#
# Installs build dependencies (best-effort outside Debian/Ubuntu), bootstraps
# a current Go toolchain per-user if needed, clones/updates the repo, builds
# it with the existing build.sh, and drops the binary in ~/.local/bin.
set -uo pipefail

REPO_URL="https://github.com/RetroCodeRamen/ttypedesk.git"
BRANCH="master"
SRC_DIR="$HOME/.local/share/ttypedesk-src"
GOROOT_DIR="$HOME/.local/share/ttypedesk-go"
BIN_DIR="$HOME/.local/bin"

echo "== TTYPE Desk installer =="

if [ "$(uname -s)" != "Linux" ]; then
  echo "ERROR: this installer only supports Linux."
  exit 1
fi

# ---- build dependencies (git, gcc, ar) ----
have_build_tools() {
  command -v git >/dev/null 2>&1 && command -v gcc >/dev/null 2>&1 && command -v ar >/dev/null 2>&1
}

if ! have_build_tools; then
  echo "-- Installing build dependencies (git, gcc, binutils) --"
  # Root (common in minimal containers/servers) has no sudo binary at all —
  # don't assume one exists just because we're not already privileged.
  SUDO=""
  if [ "$(id -u)" != "0" ]; then
    if command -v sudo >/dev/null 2>&1; then
      SUDO="sudo"
      echo "-- This runs as a normal command in this window, so you'll be prompted for your sudo password right here if it's needed. --"
    else
      echo "ERROR: not running as root and 'sudo' isn't available — install git, gcc, and binutils yourself (or re-run as root), then re-run this script."
      exit 1
    fi
  fi
  if command -v apt-get >/dev/null 2>&1; then
    $SUDO apt-get update && $SUDO apt-get install -y git gcc make binutils ca-certificates curl
  elif command -v dnf >/dev/null 2>&1; then
    $SUDO dnf install -y git gcc make binutils ca-certificates curl
  elif command -v pacman >/dev/null 2>&1; then
    $SUDO pacman -Sy --noconfirm git gcc make binutils ca-certificates curl
  elif command -v zypper >/dev/null 2>&1; then
    $SUDO zypper install -y git gcc make binutils ca-certificates curl
  elif command -v apk >/dev/null 2>&1; then
    $SUDO apk add git gcc make binutils build-base ca-certificates curl
  else
    echo "WARN: no known package manager found (looked for apt-get/dnf/pacman/zypper/apk)."
    echo "WARN: install git, gcc, and binutils (ar) yourself, then re-run this script."
  fi
fi

if ! have_build_tools; then
  echo "ERROR: git/gcc/ar still missing after attempted install. Install them manually and re-run."
  exit 1
fi
echo "-- Build tools OK: $(command -v git), $(command -v gcc), $(command -v ar) --"

# ---- Go toolchain (per-user, no sudo — go.mod's toolchain directive will
# auto-fetch the exact pinned version over the network as needed, this just
# needs to be recent enough to bootstrap that) ----
go_ok() {
  command -v go >/dev/null 2>&1 || return 1
  v="$(go env GOVERSION 2>/dev/null || true)" # e.g. "go1.24.4"
  v="${v#go}"
  major="${v%%.*}"
  rest="${v#*.}"
  minor="${rest%%.*}"
  [ "${major:-0}" -gt 1 ] 2>/dev/null && return 0
  [ "${major:-0}" -eq 1 ] 2>/dev/null && [ "${minor:-0}" -ge 21 ] 2>/dev/null
}

if ! go_ok; then
  echo "-- No sufficiently recent Go found — installing one into $GOROOT_DIR (per-user, no sudo) --"
  GO_VER="$(curl -fsSL https://go.dev/VERSION?m=text | head -1)"
  if [ -z "$GO_VER" ]; then
    echo "ERROR: couldn't determine latest Go version from go.dev"
    exit 1
  fi
  case "$(uname -m)" in
    x86_64) GOARCH=amd64 ;;
    aarch64 | arm64) GOARCH=arm64 ;;
    armv7l | armv6l) GOARCH=armv6l ;;
    *)
      echo "ERROR: unsupported architecture $(uname -m)"
      exit 1
      ;;
  esac
  rm -rf "$GOROOT_DIR"
  mkdir -p "$GOROOT_DIR"
  if ! curl -fsSL "https://go.dev/dl/${GO_VER}.linux-${GOARCH}.tar.gz" | tar -xz -C "$GOROOT_DIR" --strip-components=1; then
    echo "ERROR: failed to download/extract Go toolchain"
    exit 1
  fi
  export PATH="$GOROOT_DIR/bin:$PATH"
  if ! go_ok; then
    echo "ERROR: Go toolchain install didn't produce a working 'go' binary"
    exit 1
  fi
fi
echo "-- Go OK: $(command -v go) ($(go env GOVERSION)) --"

# ---- clone or update ----
# Full history, not --depth 1: scripts/version.sh counts commits so far this
# calendar month to build the version string, which needs real history to
# count against — a shallow clone would always see just the 1 commit it has.
if [ -d "$SRC_DIR/.git" ]; then
  echo "-- Updating existing checkout in $SRC_DIR --"
  if ! git -C "$SRC_DIR" fetch origin "$BRANCH" || ! git -C "$SRC_DIR" reset --hard "origin/$BRANCH"; then
    echo "ERROR: failed to update $SRC_DIR"
    exit 1
  fi
else
  echo "-- Cloning $REPO_URL into $SRC_DIR --"
  if ! git clone -b "$BRANCH" "$REPO_URL" "$SRC_DIR"; then
    echo "ERROR: git clone failed"
    exit 1
  fi
fi

# ---- build ----
echo "-- Building TTYPE Desk --"
if ! (cd "$SRC_DIR" && ./build.sh); then
  echo "ERROR: build failed"
  exit 1
fi

# ---- install ----
# Stage to a temp file and rename over the target rather than copying
# straight onto it: `ttypedesk -update` runs this script from the *running*
# binary, and overwriting a busy executable in place fails with "Text file
# busy" — rename() replaces the directory entry instead of touching the
# original (still-running) inode, so it works even while ttypedesk is live.
mkdir -p "$BIN_DIR"
TMP_BIN="$(mktemp "$BIN_DIR/.ttypedesk.XXXXXX")"
if ! cp "$SRC_DIR/bin/ttypedesk" "$TMP_BIN"; then
  echo "ERROR: failed to stage the new binary"
  rm -f "$TMP_BIN"
  exit 1
fi
chmod +x "$TMP_BIN"
mv -f "$TMP_BIN" "$BIN_DIR/ttypedesk"

echo "== Installed: $BIN_DIR/ttypedesk =="
case ":$PATH:" in
  *":$BIN_DIR:"*)
    echo "-- $BIN_DIR is already on your PATH. Run: ttypedesk --"
    ;;
  *)
    echo "-- $BIN_DIR isn't on your PATH yet. Add it, e.g.:"
    echo "     echo 'export PATH=\"$BIN_DIR:\$PATH\"' >> ~/.bashrc && source ~/.bashrc"
    echo "   Or just run it directly for now: $BIN_DIR/ttypedesk"
    ;;
esac
