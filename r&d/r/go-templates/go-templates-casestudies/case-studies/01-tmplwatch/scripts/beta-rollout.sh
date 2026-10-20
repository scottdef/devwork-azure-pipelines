#!/usr/bin/env bash
# beta-rollout: stage a tmplwatch release onto one or two beta hosts.
#
#   scripts/beta-rollout.sh [-n] [-v VERSION] host1 [host2]
#
# For each host, over ssh:
#   1. copy the linux binary and config to /usr/local/lib/tmplwatch/VERSION/;
#   2. run `tmplwatch -once -dry` there and show the diff it would apply;
#   3. run the validate command (default: caddy validate) on a -once
#      render into a scratch directory;
#   4. only without -n: flip the /usr/local/bin/tmplwatch symlink and
#      restart the unit, then wait for /healthz.
# -n (the default until you pass -apply) stops after step 3.
# Two hosts maximum: beta is a canary, not a fleet deploy.
set -euo pipefail

cd "$(dirname "$0")/.."
DRY=1
VERSION=${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}
VALIDATE=${VALIDATE:-"caddy validate --config /var/tmp/tmplwatch-beta/out/Caddyfile"}
ARCH=${ARCH:-amd64}
CONFIG=${CONFIG:-config/tmplwatch.json}

usage() { echo "usage: $0 [-n|-apply] [-v VERSION] host1 [host2]" >&2; exit 2; }
while [[ $# -gt 0 ]]; do
	case $1 in
	-n) DRY=1; shift ;;
	-apply) DRY=0; shift ;;
	-v) VERSION=$2; shift 2 ;;
	-h|--help) usage ;;
	-*) usage ;;
	*) break ;;
	esac
done
[[ $# -ge 1 && $# -le 2 ]] || usage

BIN=dist/tmplwatch_linux_${ARCH}
if [[ ! -x $BIN ]]; then
	mkdir -p dist
	GOFLAGS=${GOFLAGS:--mod=mod} GOPROXY=${GOPROXY:-off} CGO_ENABLED=0 GOOS=linux GOARCH=$ARCH \
		go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$BIN" ./cmd/tmplwatch
fi

for host in "$@"; do
	echo "== $host: staging $VERSION"
	dest=/usr/local/lib/tmplwatch/$VERSION
	ssh "$host" "sudo mkdir -p '$dest' /var/tmp/tmplwatch-beta/out"
	scp -q "$BIN" "$host:/var/tmp/tmplwatch-beta/tmplwatch"
	scp -q "$CONFIG" "$host:/var/tmp/tmplwatch-beta/tmplwatch.json"
	scp -qr templates "$host:/var/tmp/tmplwatch-beta/"
	ssh "$host" "sudo install -m 0755 /var/tmp/tmplwatch-beta/tmplwatch '$dest/tmplwatch'"

	echo "== $host: dry run against live destinations"
	# shellcheck disable=SC2029 # expand locally on purpose
	ssh "$host" "cd /etc/tmplwatch && sudo -E '$dest/tmplwatch' -c tmplwatch.json -once -dry -a off" || {
		rc=$?
		[[ $rc == 3 ]] && echo "$host: templates incomplete (missing keys); not proceeding" >&2
		exit "$rc"
	}

	echo "== $host: render to scratch and validate"
	ssh "$host" "cd /var/tmp/tmplwatch-beta && sed 's#\"out/#\"/var/tmp/tmplwatch-beta/out/#g' tmplwatch.json > beta.json &&
		'$dest/tmplwatch' -c beta.json -once -a off && $VALIDATE"

	if [[ $DRY == 1 ]]; then
		echo "== $host: dry run only (pass -apply to switch)"
		continue
	fi
	echo "== $host: switching to $VERSION"
	ssh "$host" "sudo ln -sfn '$dest/tmplwatch' /usr/local/bin/tmplwatch && sudo systemctl restart tmplwatch &&
		for i in \$(seq 30); do /usr/local/bin/tmplwatch -healthcheck && exit 0; sleep 1; done; exit 1"
	echo "== $host: healthy on $VERSION"
done
