# shellcheck shell=bash
# shellcheck disable=SC2034  # variables here are used by the scripts that source this file
# Shared by run-local.sh and seed.sh. Source it; do not run it.
#
# It starts two local processes: a fake GitHub OIDC issuer and cool-api,
# writing to a throwaway Git repository instead of GitHub. Everything else is
# the real code: the same cool-api, the same coolctl, the same policy.
#
# The fake issuer signs any claim it is asked for. That is how one script can
# play an app's CI job in one line and an approved IssueOps job in the next,
# and it is why the issuer refuses to listen on anything but loopback.

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BIN=${BIN:-$ROOT/coolctl/bin}
ISSUER_ADDR=127.0.0.1:${COOL_DEMO_ISSUER_PORT:-9911}
API_ADDR=127.0.0.1:${COOL_DEMO_API_PORT:-9912}
ISSUES=https://github.com/CoolEngOrg/issueops/issues
PIPELINE=CoolEngOrg/cool-actions/.github/workflows/cool-pipeline.yml@refs/tags/v1
PIDS=()

export COOL_API_URL=http://$API_ADDR

say() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
note() { printf '   %s\n' "$*"; }
run() { printf '\033[2m$ %s\033[0m\n' "$*"; "$@"; }

# build compiles the five binaries into $BIN.
build() {
	command -v go >/dev/null || { echo "demo: Go 1.24 is required" >&2; exit 2; }
	(cd "$ROOT/coolctl" && CGO_ENABLED=0 go build -trimpath -o "$BIN/" ./cmd/...)
	export PATH=$BIN:$PATH
}

# wait_http URL waits up to five seconds for a 200.
wait_http() {
	local _
	for _ in $(seq 1 50); do
		curl -fsS -o /dev/null "$1" 2>/dev/null && return 0
		sleep 0.1
	done
	echo "demo: $1 did not come up; see the logs in $WORK" >&2
	return 1
}

# start GITOPS-DIR turns the directory into a Git repository and starts the
# issuer and cool-api against it.
start() {
	local gitops=$1
	if [ ! -d "$gitops/.git" ]; then
		git -C "$gitops" init -q -b main
		git -C "$gitops" -c user.name=demo -c user.email=demo@example.invalid add -A
		git -C "$gitops" -c user.name=demo -c user.email=demo@example.invalid commit -qm "seed: teams, clusters and Argo CD configuration"
	fi
	"$BIN/cool-oidc-fake" -addr "$ISSUER_ADDR" >"$WORK/issuer.log" 2>&1 &
	PIDS+=($!)
	COOL_ADDR=$API_ADDR COOL_STORE=dir:$gitops COOL_OIDC_ISSUER=http://$ISSUER_ADDR \
		"$BIN/cool-api" >"$WORK/cool-api.log" 2>&1 &
	PIDS+=($!)
	wait_http "http://$ISSUER_ADDR/.well-known/openid-configuration"
	wait_http "$COOL_API_URL/healthz"
}

stop() {
	local pid
	for pid in "${PIDS[@]:-}"; do
		[ -n "$pid" ] && kill "$pid" 2>/dev/null
	done
	wait 2>/dev/null || true
}

# token CLAIM=VALUE... asks the fake issuer for a token with those claims.
token() {
	local args=(--data-urlencode audience=cool-api) kv
	for kv in "$@"; do args+=(--data-urlencode "$kv"); done
	curl -fsS --get "http://$ISSUER_ADDR/token" "${args[@]}" | jq -rM .value
}

RUN_ID=9000000000
next_run() { RUN_ID=$((RUN_ID + 1)); }

# as_ci REPO COMMAND... runs a command as the pipeline job of an app repo.
as_ci() {
	local repo=$1
	shift
	next_run
	COOL_TOKEN=$(token "repository=$repo" "job_workflow_ref=$PIPELINE" "actor=octocat" "run_id=$RUN_ID") run "$@"
}

# as_ops TYPE ENV COMMAND... runs a command as the execute job of an approved
# IssueOps request of that type, in the GitHub environment cool-ENV.
as_ops() {
	local type=$1 env=$2
	shift 2
	next_run
	COOL_TOKEN=$(token "repository=CoolEngOrg/issueops" \
		"job_workflow_ref=CoolEngOrg/issueops/.github/workflows/execute-cool-$type.yml@refs/heads/main" \
		"environment=cool-$env" "actor=mona" "run_id=$RUN_ID") run "$@"
}

# digest LETTER and commit LETTER make recognisable stand-ins for an image
# digest and a commit: the release built from commit bbbbbbb... is rbbbbbbb.
digest() { printf 'sha256:%s' "$(printf "$1%.0s" $(seq 1 64))"; }
commit() { printf "$1%.0s" $(seq 1 40); }
