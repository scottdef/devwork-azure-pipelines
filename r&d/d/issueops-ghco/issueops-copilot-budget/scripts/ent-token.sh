#!/usr/bin/env bash
# ent-token.sh - mint an enterprise installation token by hand. Fallback only.
#
# actions/create-github-app-token does this with `enterprise:`. Use this if the
# action cannot find the installation (actions/create-github-app-token#373).
#
# env: APP_CLIENT_ID APP_PRIVATE_KEY (PEM)   stdout: token
# shellcheck source=lib.sh
. "$(dirname "$0")/lib.sh"
need curl jq openssl
: "${APP_CLIENT_ID:?}" "${APP_PRIVATE_KEY:?}"

b64() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }
now=$(date +%s)
h=$(printf '{"alg":"RS256","typ":"JWT"}' | b64)
c=$(printf '{"iat":%d,"exp":%d,"iss":"%s"}' $((now - 60)) $((now + 540)) "$APP_CLIENT_ID" | b64)
s=$(printf '%s.%s' "$h" "$c" | openssl dgst -sha256 -sign <(printf '%s\n' "$APP_PRIVATE_KEY") -binary | b64)
jwt="$h.$c.$s"

gh_() { curl -fsS -H "Authorization: Bearer $jwt" -H "Accept: application/vnd.github+json" \
	-H "X-GitHub-Api-Version: $API_VERSION" "$@"; }

id=$(gh_ "https://api.github.com/app/installations?per_page=100" |
	jq -r --arg e "$(lc "$ENTERPRISE")" \
	'.[] | select(.target_type == "Enterprise" and ((.account.slug // .account.login // "") | ascii_downcase) == $e) | .id' | head -n1)
[ -n "$id" ] || die "no enterprise installation of this app on $ENTERPRISE"
tok=$(gh_ -X POST "https://api.github.com/app/installations/$id/access_tokens" | jq -r .token)
[ -n "$tok" ] && [ "$tok" != null ] || die "no token returned"
[ -z "${GITHUB_ACTIONS:-}" ] || echo "::add-mask::$tok"
printf '%s\n' "$tok"
