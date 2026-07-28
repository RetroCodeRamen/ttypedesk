#!/usr/bin/env bash
# Computes the current version string: MAJOR.MINOR.YYMMNN
#   MAJOR.MINOR — from the VERSION file (bumped by hand)
#   YY, MM      — current year/month
#   NN          — count of commits on this branch so far this calendar month
#                 (zero-padded to 2 digits)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

MAJOR_MINOR="$(tr -d '[:space:]' <"$ROOT/VERSION")"
YY="$(date +%y)"
MM="$(date +%m)"
FIRST_DAY="$(date +%Y-%m-01)"
NN="$(git -C "$ROOT" log --since="${FIRST_DAY} 00:00:00" --oneline | wc -l | tr -d ' ')"

printf '%s.%s%s%02d\n' "$MAJOR_MINOR" "$YY" "$MM" "$NN"
