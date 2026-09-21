#!/usr/bin/env bash
# aw-lint.sh - cheap pre-compile checks for gh-aw workflow sources.
# Not a substitute for `gh aw compile --strict`; it catches the mistakes
# the compiler cannot (prompt-body issues) and the ones worth catching
# before you have the extension installed.
#
# usage: aw-lint.sh [file.md ...]      default: .github/workflows/*.md
# exit:  0 clean, 1 failures, 2 usage
set -u

[ $# -eq 0 ] && set -- .github/workflows/*.md
rc=0
say() { printf '%s: %s: %s\n' "$1" "$2" "$3"; }

for f in "$@"; do
	[ -f "$f" ] || { say "$f" FAIL "no such file"; rc=1; continue; }
	[ "$(head -n1 "$f")" = "---" ] || { say "$f" FAIL "no YAML frontmatter"; rc=1; continue; }

	fm=$(awk 'NR==1{next} /^---[ \t]*$/{exit} {print}' "$f")
	body=$(awk 'n>=2{print} /^---[ \t]*$/{n++}' "$f")

	# No on: means shared component (imported, never compiled). Skip.
	printf '%s\n' "$fm" | grep -q '^on:' || { say "$f" SKIP "shared component (no on:)"; continue; }

	# 1. agent job must be read-only; writes go through safe-outputs.
	w=$(printf '%s\n' "$fm" | awk '/^permissions:/{p=1;next} p&&/^[^ \t]/{p=0} p&&/:[ \t]*write/' | grep -v 'copilot-requests' || true)
	[ -n "$w" ] && { say "$f" FAIL "write permission on agent job: $(echo $w) -- use safe-outputs"; rc=1; }
	printf '%s\n' "$fm" | grep -Eq '^permissions:[ \t]*write-all' && { say "$f" FAIL "permissions: write-all"; rc=1; }

	# 2. raw event text bypasses sanitization.
	if grep -Eq 'github\.event\.(issue|comment|pull_request|discussion|review)\.(body|title)' "$f"; then
		say "$f" FAIL 'raw github.event.*.body/title -- use ${{ needs.activation.outputs.text }}'; rc=1
	fi

	# 3. strict mode wants explicit egress.
	printf '%s\n' "$fm" | grep -q '^network:' || say "$f" WARN "no network: block (strict mode requires one)"
	printf '%s\n' "$fm" | awk '/^network:/{p=1;next} p&&/^[^ \t]/{p=0} p' | grep -Eq '"\*"|- \*$' &&
		say "$f" WARN "bare wildcard in network allow-list"

	# 4. silent completion is the #1 runtime failure.
	printf '%s\n' "$body" | grep -qi 'noop' || say "$f" WARN "prompt never mentions noop; agent may complete silently"

	# 5. unrestricted shell.
	printf '%s\n' "$fm" | grep -Eq '^[ \t]+-?[ \t]*"?(:\*|\*)"?[ \t]*$' &&
		printf '%s\n' "$fm" | grep -q 'bash' && say "$f" WARN "possible unrestricted bash (\":*\" or \"*\")"

	# 6. browser workflows: manual trigger only.
	if printf '%s\n' "$fm" | grep -q '^[ \t]*playwright:'; then
		t=$(printf '%s\n' "$fm" | awk '/^on:/{p=1;next} p&&/^[^ \t]/{p=0} p&&/^  [a-z_]+:/{sub(/:.*/,"");gsub(/ /,"");print}')
		[ "$t" = "workflow_dispatch" ] || say "$f" WARN "playwright with triggers [$(echo $t)]; prefer workflow_dispatch only"
	fi

	# 7. stale or missing lock.
	l="${f%.md}.lock.yml"
	if [ ! -f "$l" ]; then say "$f" WARN "no ${l##*/}; run gh aw compile"
	elif [ "$f" -nt "$l" ]; then say "$f" WARN "${l##*/} older than source; recompile"; fi
done
exit $rc
