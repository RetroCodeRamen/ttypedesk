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
go build -o bin/ttypedesk ./cmd/ttypedesk
echo "Built $ROOT/bin/ttypedesk"
