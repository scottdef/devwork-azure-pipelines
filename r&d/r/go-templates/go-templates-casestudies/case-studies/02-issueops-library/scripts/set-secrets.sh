#!/usr/bin/env bash
# set-secrets.sh: store the issueops-autoadmin-app private key as the
# APP_PRIVATE_KEY Actions secret on both repositories.
#
#   scripts/set-secrets.sh path/to/issueops-autoadmin-app.private-key.pem
#
# The key goes from the file to gh on stdin: never into argv (visible in
# ps), never into Terraform state, never into shell history.
set -euo pipefail
pem="${1:?usage: set-secrets.sh key.pem}"
owner="${OWNER:-CoolGitOrg}"
repos=("${INTAKE_REPO:-issueops-intake}" "${LIBRARY_REPO:-issueops-library}")
[[ -r "$pem" ]] || { echo "set-secrets: cannot read $pem" >&2; exit 1; }
grep -q 'BEGIN RSA PRIVATE KEY\|BEGIN PRIVATE KEY' "$pem" || { echo "set-secrets: $pem is not a PEM private key" >&2; exit 1; }
gh auth status >/dev/null
for r in "${repos[@]}"; do
	gh secret set APP_PRIVATE_KEY --repo "$owner/$r" <"$pem"
	echo "set-secrets: APP_PRIVATE_KEY set on $owner/$r"
done
echo "set-secrets: now delete or vault the local key file."
