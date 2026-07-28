#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
LV="$ROOT/third_party/libvterm-0.3.3"
mkdir -p "$LV/build"
if [[ ! -f "$LV/build/libvterm.a" ]]; then
  echo "Building vendored libvterm..."
  (cd "$LV/build" && gcc -std=c99 -O2 -I../include -I../src -c ../src/*.c && ar rcs libvterm.a *.o)
fi
cd "$ROOT"
if [[ -d .git ]] && [[ "$(git config --get core.hooksPath || true)" != ".githooks" ]]; then
  git config core.hooksPath .githooks
  echo "-- Set core.hooksPath = .githooks (auto-tags each commit with its version) --"
fi
VERSION="$("$ROOT/scripts/version.sh")"
go build -ldflags "-X main.version=$VERSION" -o bin/ttypedesk ./cmd/ttypedesk
echo "Built $ROOT/bin/ttypedesk ($VERSION)"
