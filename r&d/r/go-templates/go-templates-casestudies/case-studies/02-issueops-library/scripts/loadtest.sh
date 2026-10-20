#!/usr/bin/env bash
# loadtest.sh: load-test the published library site (see docs/LOADTEST.md).
# Thin wrapper over issueops-library/scripts/loadtest.sh; pass -local to
# test a local render of site-sample/ instead of GitHub Pages.
set -euo pipefail
exec "$(dirname "$0")/../issueops-library/scripts/loadtest.sh" "$@"
