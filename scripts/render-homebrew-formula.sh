#!/usr/bin/env bash
set -euo pipefail

dist="${1:-}"
out="${2:-}"
if [[ -z "$dist" || -z "$out" ]]; then
  echo "usage: $0 <dist-dir> <output-formula>" >&2
  exit 2
fi
if [[ -z "${RELEASE_TAG:-}" ]]; then
  echo "RELEASE_TAG is required" >&2
  exit 1
fi
checksums="$dist/checksums.txt"
if [[ ! -f "$checksums" ]]; then
  echo "checksums file not found: $checksums" >&2
  exit 1
fi

checksum_for() {
  local name="$1"
  awk -v want="$name" '$2 == want { print $1 }' "$checksums"
}

require_checksum() {
  local name="$1"
  local sum
  sum="$(checksum_for "$name")"
  if [[ -z "$sum" ]]; then
    echo "checksums.txt missing $name" >&2
    exit 1
  fi
  printf '%s' "$sum"
}

version="${RELEASE_TAG#v}"
base="https://github.com/GoCodeAlone/ratchet-cli/releases/download/${RELEASE_TAG}"
darwin_amd64="$(require_checksum ratchet_darwin_amd64.tar.gz)"
darwin_arm64="$(require_checksum ratchet_darwin_arm64.tar.gz)"
linux_amd64="$(require_checksum ratchet_linux_amd64.tar.gz)"
linux_arm64="$(require_checksum ratchet_linux_arm64.tar.gz)"

mkdir -p "$(dirname "$out")"
cat >"$out" <<EOF
class RatchetCli < Formula
  desc "Interactive AI agent CLI"
  homepage "https://github.com/GoCodeAlone/ratchet-cli"
  version "$version"
  license "Apache-2.0"

  on_macos do
    on_intel do
      url "$base/ratchet_darwin_amd64.tar.gz"
      sha256 "$darwin_amd64"
    end

    on_arm do
      url "$base/ratchet_darwin_arm64.tar.gz"
      sha256 "$darwin_arm64"
    end
  end

  on_linux do
    on_intel do
      url "$base/ratchet_linux_amd64.tar.gz"
      sha256 "$linux_amd64"
    end

    on_arm do
      url "$base/ratchet_linux_arm64.tar.gz"
      sha256 "$linux_arm64"
    end
  end

  def install
    bin.install "ratchet"
  end

  test do
    assert_match "ratchet", shell_output("#{bin}/ratchet --version")
  end
end
EOF
