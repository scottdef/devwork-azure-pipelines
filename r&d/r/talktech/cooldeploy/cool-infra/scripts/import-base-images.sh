#!/usr/bin/env bash
# Mirror the base images that app builds start from into the platform registry.
#
#   scripts/import-base-images.sh
#
# Builds run inside the registry and pull only from it, so a build cannot be
# broken or changed by a public registry. This script is the one place where
# an outside image comes in. Run it when a base image should move forward,
# for instance to pick up a Go patch release.
#
#   base/golang:1.24                    what Go apps are compiled with
#   base/distroless-static:nonroot      what they run on
set -euo pipefail

cd "$(dirname "$0")/.."
REGISTRY=${REGISTRY:-$(jq -rM .registry config/platform.json)}

import() { # import SOURCE TARGET
	echo "import $1 as $REGISTRY.azurecr.io/$2"
	az acr import --name "$REGISTRY" --source "$1" --image "$2" --force
}

import docker.io/library/golang:1.24 base/golang:1.24
import gcr.io/distroless/static-debian12:nonroot base/distroless-static:nonroot

echo
echo "The platform's own images are built from the coolctl repository:"
echo "  make -C ../coolctl images VERSION=1.0.0"
