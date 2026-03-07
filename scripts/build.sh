#!/usr/bin/env bash
# build.sh — local cross-compile helper for ratchet
# Usage: ./scripts/build.sh [version]
set -euo pipefail

VERSION="${1:-dev}"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS="-s -w \
  -X github.com/GoCodeAlone/ratchet-cli/internal/version.Version=${VERSION} \
  -X github.com/GoCodeAlone/ratchet-cli/internal/version.Commit=${COMMIT} \
  -X github.com/GoCodeAlone/ratchet-cli/internal/version.Date=${DATE}"

mkdir -p dist

TARGETS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
)

for target in "${TARGETS[@]}"; do
  GOOS="${target%/*}"
  GOARCH="${target#*/}"
  output="dist/ratchet_${GOOS}_${GOARCH}"
  echo "Building ${output}..."
  CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" \
    go build -ldflags "${LDFLAGS}" -o "${output}" ./cmd/ratchet
done

echo "Done. Binaries in dist/:"
ls -lh dist/
