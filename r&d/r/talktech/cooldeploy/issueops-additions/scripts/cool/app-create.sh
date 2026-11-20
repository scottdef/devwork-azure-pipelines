#!/usr/bin/env bash
# cool-app-create: a new app, from nothing to its first deploy.
#
#   1. create the repository from the preset's template
#   2. make its OIDC tokens name the workflow that runs, so Azure can trust
#      the platform's pipeline and nothing else in the repository
#   3. write the app's name into cool.json and go.mod, and push
#   4. create a managed identity per environment, federated with the app's
#      service account, with read access to the team's Key Vault
#   5. register the app with cool-api
#
# The push in step 3 starts the pipeline, which deploys to dev.
#
# Needs gh (GH_TOKEN: an App token that may create repositories), az (signed
# in with rights to create identities and role assignments) and coolctl.
# shellcheck source=scripts/cool/lib.sh
. "$(dirname "$0")/lib.sh"

ORG=${COOL_ORG:-CoolEngOrg}
REGION=${COOL_REGION:-eus2}

team=$(need team "$RE_TEAM")
name=$(need app "$RE_SLUG")
preset=$(need preset 'go-htmx|go-server|static')
expose=$(need expose 'internal|public')
description=$(field description | tr -cd '[:print:]' | cut -c1-200)
[[ ! $name =~ -pr-[0-9]+$ ]] || die "an app name may not end in -pr-<number>; previews use that"

case $preset in
go-server) template=cool-template-go-api ;;
go-htmx) template=cool-template-go-htmx ;;
static) template=cool-template-static-js ;;
esac
repo=$team-$name
full=$ORG/$repo

# capture PLACEHOLDER COMMAND... runs a command for its output; a dry run
# prints the command and yields the placeholder.
capture() {
	local placeholder=$1
	shift
	printf '+ %s\n' "$*" >&2
	if [ "$DRY_RUN" = 1 ]; then
		printf '%s' "$placeholder"
	else
		"$@"
	fi
}

# 1. The repository.
if [ "$DRY_RUN" != 1 ] && gh repo view "$full" >/dev/null 2>&1; then
	die "the repository $full already exists"
fi
run gh repo create "$full" --internal --template "$ORG/$template" --description "$description"

# 2. Its OIDC subject. By default the subject names the repository and the
# branch; this makes it name the workflow file that defines the job.
run gh api --method PUT "repos/$full/actions/oidc/customization/sub" \
	-F use_default=false -f 'include_claim_keys[]=job_workflow_ref'

# 3. Its name, in the files that carry one.
work=$(mktemp -d)
if [ "$DRY_RUN" = 1 ]; then
	echo "+ clone $full, set team/app/expose in cool.json, rewrite the module path, push" >&2
else
	# A repository made from a template is filled in a moment after it is created.
	for _ in $(seq 1 30); do
		gh repo clone "$full" "$work/repo" -- --quiet 2>/dev/null && [ -f "$work/repo/cool.json" ] && break
		rm -rf "$work/repo"
		sleep 2
	done
	[ -f "$work/repo/cool.json" ] || die "the new repository has no cool.json; is $ORG/$template a template repository?"
	jq -M --arg team "$team" --arg app "$name" --arg expose "$expose" \
		'.team = $team | .app = $app | .expose = $expose' "$work/repo/cool.json" >"$work/cool.json"
	mv "$work/cool.json" "$work/repo/cool.json"
	if [ -f "$work/repo/go.mod" ]; then
		old=$(awk '$1 == "module" { print $2; exit }' "$work/repo/go.mod")
		grep -rlF --include='*.go' --include=go.mod "$old" "$work/repo" |
			xargs -r sed -i "s|$old|github.com/$full|g"
	fi
	coolctl validate -f "$work/repo/cool.json" >&2
	git -C "$work/repo" add -A
	git -C "$work/repo" -c user.name="cooldeploy[bot]" -c user.email="cooldeploy[bot]@users.noreply.github.com" \
		commit --quiet -m "Name the app $team/$name"
	git -C "$work/repo" push --quiet origin HEAD
fi

# 4. One identity per environment.
ids=()
for env in dev tst prd; do
	rg=$env-$REGION-appdev-rg-001
	cluster=$(cluster_of "$env")
	identity=$env-$REGION-$team-$name-id-001
	vault=$env-$REGION-$team-kv-001
	client=$(capture "00000000-0000-4000-8000-0000000000${#ids[@]}0" \
		az identity create --resource-group "$rg" --name "$identity" --query clientId --output tsv)
	principal=$(capture "00000000-0000-4000-8000-0000000000${#ids[@]}1" \
		az identity show --resource-group "$rg" --name "$identity" --query principalId --output tsv)
	issuer=$(capture "https://oidc.example/$cluster/" \
		az aks show --resource-group "$rg" --name "$cluster" --query oidcIssuerProfile.issuerUrl --output tsv)
	vault_id=$(capture "/subscriptions/0/resourceGroups/$rg/providers/Microsoft.KeyVault/vaults/$vault" \
		az keyvault show --name "$vault" --query id --output tsv)
	# The app's pods run as service account <app> in namespace <team>-<app>.
	run az identity federated-credential create --resource-group "$rg" --identity-name "$identity" \
		--name "$cluster" --issuer "$issuer" \
		--subject "system:serviceaccount:$team-$name:$name" \
		--audiences api://AzureADTokenExchange --output none
	run az role assignment create --assignee-object-id "$principal" --assignee-principal-type ServicePrincipal \
		--role "Key Vault Secrets User" --scope "$vault_id" --output none
	ids+=(-identity "$env=$client")
done

# 5. The registration.
out=$(cool register -app "$team/$name" -repo "$full" "${ids[@]}")

report "Created \`$team/$name\`."
report ""
report "| | |"
report "| --- | --- |"
report "| Repository | https://github.com/$full |"
report "| Template | \`$template\` (\`$preset\`) |"
report "| Namespace | \`$team-$name\` in dev, tst and prd |"
report "| Dev URL | https://$name.dev.${COOL_DOMAIN:-coolapps.example} |"
report ""
report '```'
report "$out"
report '```'
report ""
report "The first pipeline run is under way and deploys to dev. From there:"
report ""
report "- every push to \`main\` deploys to dev, and every pull request gets a preview;"
report "- **Promote a release** takes a release to tst and prd;"
report "- secrets go in the Key Vault \`<env>-$REGION-$team-kv-001\` as \`$name-<NAME>\`, and their names in \`cool.json\`."
