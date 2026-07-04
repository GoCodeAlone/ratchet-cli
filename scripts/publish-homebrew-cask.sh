#!/usr/bin/env bash
set -euo pipefail

push=0
if [[ "${1:-}" == "--push" ]]; then
  push=1
  shift
fi

generated="${1:-}"
tap="${2:-}"
if [[ -z "$generated" || -z "$tap" ]]; then
  echo "usage: $0 [--push] <generated-cask> <tap-checkout>" >&2
  exit 2
fi
if [[ ! -f "$generated" ]]; then
  echo "generated cask not found: $generated" >&2
  exit 1
fi
if [[ "$(basename "$generated")" != "ratchet-cli.rb" ]]; then
  echo "generated cask must be ratchet-cli.rb: $generated" >&2
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
  if grep -Fq -- "$token" "$generated"; then
    echo "generated cask contains forbidden smoke token: $token" >&2
    exit 1
  fi
done
for want in 'cask "ratchet-cli"' 'binary "ratchet"'; do
  if ! grep -Fq -- "$want" "$generated"; then
    echo "generated cask missing required content: $want" >&2
    exit 1
  fi
done

mkdir -p "$tap/Casks"
cp "$generated" "$tap/Casks/ratchet-cli.rb"

if git -C "$tap" diff --quiet -- Casks/ratchet-cli.rb; then
  cask_sha="$(git -C "$tap" log -n 1 --format=%H -- Casks/ratchet-cli.rb)"
  if [[ -z "$cask_sha" ]]; then
    echo "tap cask is unchanged but has no path-changing commit" >&2
    exit 1
  fi
else
  git -C "$tap" add Casks/ratchet-cli.rb
  suffix="${RELEASE_TAG:-}"
  if [[ -n "$suffix" ]]; then
    suffix=" $suffix"
  fi
  git -C "$tap" commit -m "Update ratchet-cli cask${suffix}"
  if [[ "$push" == "1" ]]; then
    git -C "$tap" push origin HEAD:main
  fi
  cask_sha="$(git -C "$tap" log -n 1 --format=%H -- Casks/ratchet-cli.rb)"
fi

line="RATCHET_RELEASE_GUARD_TAP_COMMITS=Casks/ratchet-cli.rb=$cask_sha"
if [[ -n "${GITHUB_ENV:-}" ]]; then
  printf '%s\n' "$line" >>"$GITHUB_ENV"
else
  printf '%s\n' "$line"
fi
