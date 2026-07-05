#!/usr/bin/env bash
set -euo pipefail

push=0
if [[ "${1:-}" == "--push" ]]; then
  push=1
  shift
fi

generated_cask="${1:-}"
generated_formula="${2:-}"
tap="${3:-}"
if [[ -z "$generated_cask" || -z "$generated_formula" || -z "$tap" ]]; then
  echo "usage: $0 [--push] <generated-cask> <generated-formula> <tap-checkout>" >&2
  exit 2
fi
if [[ ! -f "$generated_cask" ]]; then
  echo "generated cask not found: $generated_cask" >&2
  exit 1
fi
if [[ ! -f "$generated_formula" ]]; then
  echo "generated formula not found: $generated_formula" >&2
  exit 1
fi
if [[ "$(basename "$generated_cask")" != "ratchet-cli.rb" ]]; then
  echo "generated cask must be ratchet-cli.rb: $generated_cask" >&2
  exit 1
fi
if [[ "$(basename "$generated_formula")" != "ratchet-cli.rb" ]]; then
  echo "generated formula must be ratchet-cli.rb: $generated_formula" >&2
  exit 1
fi
if ! git -C "$tap" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "tap checkout is not a git repository: $tap" >&2
  exit 1
fi
if [[ -n "$(git -C "$tap" status --porcelain)" ]]; then
  echo "tap checkout must be clean before publishing" >&2
  exit 1
fi

for token in ratchet-tui-smoke tui_smoke --tui-smoke ConnectSmokeUnix; do
  if grep -Fq -- "$token" "$generated_cask" "$generated_formula"; then
    echo "generated cask contains forbidden smoke token: $token" >&2
    exit 1
  fi
done
for want in 'cask "ratchet-cli"' 'binary "ratchet"'; do
  if ! grep -Fq -- "$want" "$generated_cask"; then
    echo "generated cask missing required content: $want" >&2
    exit 1
  fi
done
for want in 'class RatchetCli < Formula' 'bin.install "ratchet"'; do
  if ! grep -Fq -- "$want" "$generated_formula"; then
    echo "generated formula missing required content: $want" >&2
    exit 1
  fi
done

mkdir -p "$tap/Casks"
mkdir -p "$tap/Formula"
cp "$generated_cask" "$tap/Casks/ratchet-cli.rb"
cp "$generated_formula" "$tap/Formula/ratchet-cli.rb"

if git -C "$tap" diff --quiet -- Casks/ratchet-cli.rb Formula/ratchet-cli.rb; then
  cask_sha="$(git -C "$tap" log -n 1 --format=%H -- Casks/ratchet-cli.rb)"
  formula_sha="$(git -C "$tap" log -n 1 --format=%H -- Formula/ratchet-cli.rb)"
  if [[ -z "$cask_sha" ]]; then
    echo "tap cask is unchanged but has no path-changing commit" >&2
    exit 1
  fi
  if [[ -z "$formula_sha" ]]; then
    echo "tap formula is unchanged but has no path-changing commit" >&2
    exit 1
  fi
else
  git -C "$tap" add Casks/ratchet-cli.rb Formula/ratchet-cli.rb
  suffix="${RELEASE_TAG:-}"
  if [[ -n "$suffix" ]]; then
    suffix=" $suffix"
  fi
  git -C "$tap" commit -m "Update ratchet-cli Homebrew files${suffix}"
  if [[ "$push" == "1" ]]; then
    git -C "$tap" push origin HEAD:main
  fi
  cask_sha="$(git -C "$tap" log -n 1 --format=%H -- Casks/ratchet-cli.rb)"
  formula_sha="$(git -C "$tap" log -n 1 --format=%H -- Formula/ratchet-cli.rb)"
fi

line="RATCHET_RELEASE_GUARD_TAP_COMMITS=Casks/ratchet-cli.rb=$cask_sha,Formula/ratchet-cli.rb=$formula_sha"
if [[ -n "${GITHUB_ENV:-}" ]]; then
  printf '%s\n' "$line" >>"$GITHUB_ENV"
else
  printf '%s\n' "$line"
fi
