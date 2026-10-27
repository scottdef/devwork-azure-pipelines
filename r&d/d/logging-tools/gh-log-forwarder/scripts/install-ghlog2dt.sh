#!/usr/bin/env bash
# install-ghlog2dt.sh - put a verified ghlog2dt binary on PATH.
#
#   GHLOG_VERSION=v1.2.3   download that release asset + SHA256SUMS from
#                          GHLOG_REPO (default CoolGitOrg/gh-log-forwarder)
#                          and refuse it unless the checksum matches.
#   GHLOG_VERSION=source   build from the checked-out source with make.
#
# GH_TOKEN must allow reading releases of GHLOG_REPO (the workflow's own
# GITHUB_TOKEN is enough in the forwarder repo). INSTALL_DIR defaults to
# /usr/local/bin when writable, else $HOME/.local/bin (added to
# GITHUB_PATH when running in Actions).
set -euo pipefail
IFS=$'\n\t'

# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

version="${GHLOG_VERSION:-source}"
repo="${GHLOG_REPO:-CoolGitOrg/gh-log-forwarder}"
dir="${INSTALL_DIR:-}"
if [[ -z "$dir" ]]; then
  if [[ -w /usr/local/bin ]]; then dir=/usr/local/bin; else dir="$HOME/.local/bin"; fi
fi
mkdir -p "$dir"

if [[ "$version" == "source" ]]; then
  need go make
  root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  make -C "$root" build >&2
  install -m 0755 "$root/bin/ghlog2dt" "$dir/ghlog2dt"
else
  [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || die "GHLOG_VERSION '$version' is not vX.Y.Z or 'source'"
  valid_repo "$repo" || die "GHLOG_REPO '$repo' is not owner/name"
  need gh sha256sum install
  need_env GH_TOKEN
  asset="ghlog2dt_$(goos)_$(goarch)"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  gh release download "$version" --repo "$repo" --pattern "$asset" --pattern SHA256SUMS --dir "$tmp"
  # Verify exactly the asset we will install; --strict fails on bad lines.
  ( cd "$tmp" && grep -E "  ${asset}\$" SHA256SUMS | sha256sum --check --strict --status - ) \
    || die 3 "checksum mismatch for $asset $version - refusing to install"
  install -m 0755 "$tmp/$asset" "$dir/ghlog2dt"
fi

if [[ -n "${GITHUB_PATH:-}" && ":$PATH:" != *":$dir:"* ]]; then
  printf '%s\n' "$dir" >>"$GITHUB_PATH"
fi
"$dir/ghlog2dt" version >&2
