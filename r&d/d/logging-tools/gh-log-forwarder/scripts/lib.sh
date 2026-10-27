#!/usr/bin/env bash
# lib.sh - shared helpers. Source it; do not execute it.
# shellcheck shell=bash

# die prints a GitHub-annotated error and exits (default status 2).
die() {
  local code=2
  if [[ "${1:-}" =~ ^[0-9]+$ ]]; then code="$1"; shift; fi
  printf '::error::%s\n' "$*" >&2
  exit "$code"
}

warn() { printf '::warning::%s\n' "$*" >&2; }
note() { printf '%s\n' "$*" >&2; }

# need asserts that commands exist.
need() {
  local c
  for c in "$@"; do
    command -v "$c" >/dev/null 2>&1 || die "missing required command: $c"
  done
}

# need_env asserts that variables are set and non-empty, without
# ever printing their values.
need_env() {
  local v
  for v in "$@"; do
    [[ -n "${!v:-}" ]] || die "required environment variable $v is not set"
  done
}

# valid_repo checks owner/name syntax.
valid_repo() { [[ "$1" =~ ^[A-Za-z0-9][A-Za-z0-9-]{0,38}/[A-Za-z0-9._-]{1,100}$ && "$1" != *..* ]]; }

# arch maps uname -m to Go's GOARCH.
goarch() {
  case "$(uname -m)" in
    x86_64 | amd64) printf 'amd64' ;;
    aarch64 | arm64) printf 'arm64' ;;
    *) die "unsupported architecture: $(uname -m)" ;;
  esac
}

goos() {
  case "$(uname -s)" in
    Linux) printf 'linux' ;;
    Darwin) printf 'darwin' ;;
    *) die "unsupported OS: $(uname -s)" ;;
  esac
}
