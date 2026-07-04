#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "--manifest-only" ]]; then
  dist="${2:-}"
  if [[ -z "$dist" ]]; then
    echo "usage: $0 --manifest-only <dist>" >&2
    exit 2
  fi
  RATCHET_RELEASE_GUARD_MODE=manifest \
    RATCHET_RELEASE_GUARD_DIST="$dist" \
    go test ./internal/releaseguard -run TestManifestGuard -count=1
  exit 0
fi

goreleaser check
goreleaser release --snapshot --clean --skip=publish
"$0" --manifest-only dist
