#!/usr/bin/env bash
# Bring one API Management instance in line with this repository.
#
#   scripts/apim-sync.sh --env dev              apply everything
#   scripts/apim-sync.sh --env dev --dry-run    print what would be applied
#   scripts/apim-sync.sh --env dev --only apis  one part: global, products or apis
#
# The order is fixed: shared configuration, then products, then APIs, because
# each depends on the one before. Every step creates or replaces, so a sync
# that fails half way is repaired by running it again.
#
# The sync adds and updates. It does not delete: an API removed from this
# repository stays in API Management until someone removes it there, and the
# sync says so.
set -euo pipefail
# shellcheck source=scripts/lib.sh
. "$(dirname "$0")/lib.sh"

env="" only="" flags=()
while [ $# -gt 0 ]; do
	case $1 in
	--env) env=${2:-}; shift 2 ;;
	--only) only=${2:-}; shift 2 ;;
	--dry-run) DRY_RUN=1; flags+=(--dry-run); shift ;;
	*) die "usage: apim-sync.sh --env ENV [--dry-run] [--only global|products|apis]" ;;
	esac
done
export DRY_RUN
load_instance "$env"
log "== $APIM_NAME in $APIM_RG"

want() { [ -z "$only" ] || [ "$only" = "$1" ]; }

if want global; then
	"$ROOT/scripts/apim-global.sh" --env "$COOL_ENV" "${flags[@]}"
fi

if want products; then
	for file in "$ROOT"/products/*.yaml; do
		"$ROOT/scripts/apim-product.sh" --env "$COOL_ENV" --file "$file" "${flags[@]}"
	done
fi

if want apis; then
	managed=()
	while IFS= read -r file; do
		dir=$(dirname "$file")
		"$ROOT/scripts/apim-api.sh" --env "$COOL_ENV" --dir "$dir" "${flags[@]}"
		managed+=("$(yaml2json "$file" | jq -rM .name)")
	done < <(find "$ROOT/apis" -name api.yaml | sort)

	# Report APIs that API Management has and this repository does not.
	if [ "$DRY_RUN" != 1 ] || [ "${APIM_LIST_IN_DRY_RUN:-0}" = 1 ]; then
		while IFS= read -r id; do
			case " ${managed[*]} " in
			*" $id "*) ;;
			*) echo "note: API $id is in $APIM_NAME but not in this repository; remove it there if it should go" ;;
			esac
		done < <(az apim api list --service-name "$APIM_NAME" --resource-group "$APIM_RG" --query '[].name' --output tsv 2>/dev/null || true)
	fi
fi
