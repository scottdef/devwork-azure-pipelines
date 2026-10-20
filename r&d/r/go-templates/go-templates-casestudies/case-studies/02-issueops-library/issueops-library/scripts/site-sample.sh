#!/usr/bin/env bash
# site-sample.sh: render the fixture events into site-sample/ so reviewers
# can open the library site from a checkout (open site-sample/index.html).
# Times are fixed, so the output is byte-for-byte reproducible.
set -euo pipefail
cd "$(dirname "$0")/.."
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p bin "$work/ledger"
go build -trimpath -o bin/pagegen ./cmd/pagegen
go build -trimpath -o bin/ledger ./cmd/ledger

pg() { bin/pagegen -site "$work/site" -items "$work/items" -ledger "$work/ledger/ledger.json" -time "$1" "${@:2}" >/dev/null; }
ld() { bin/ledger -f "$work/ledger/ledger.json" -items "$work/items" -time "$1" "${@:2}" >/dev/null; }

# Phase 1 of six sources, an hour apart.
h=8
for e in publish publish-workflow publish-nomad publish-terraform publish-issueform publish-readme; do
	pg "2026-10-01T$(printf %02d "$h"):00:00Z" -event "testdata/events/$e.json"
	h=$((h + 1))
done
# bob borrows payments-api and ships phase 2; carol borrows two more; one loan runs late.
ld 2026-10-02T09:00:00Z borrow -item payments-api/caddyfile/42 -by bob -days 14 -issue 50 -reason "phase 2 rollout"
pg 2026-10-03T10:00:00Z -event testdata/events/publish-phase2.json
ld 2026-10-04T09:00:00Z borrow -item docs-portal/readme/47 -by carol -days 7 -issue 55
ld 2026-10-15T09:00:00Z borrow -item infra-net/terraform-module/45 -by carol -days 14 -issue 56
ld 2026-10-16T09:00:00Z borrow -item edge-proxy/workflow/43 -by dave -days 7 -issue 57
ld 2026-10-17T09:00:00Z return -item edge-proxy/workflow/43 -by dave
# Final rebuild at a fixed "now" (docs-portal is overdue by then).
pg 2026-10-19T12:00:00Z

rm -rf site-sample
mv "$work/site" site-sample
cp "$work/ledger/ledger.json" site-sample/ledger/ledger.json
echo "site-sample: $(find site-sample -type f | wc -l) files; open site-sample/index.html"
