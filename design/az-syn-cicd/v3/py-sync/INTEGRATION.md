###############################################################################
# INTEGRATION GUIDE — How to add workspace sync to the deploy workflows
#
# This file shows the exact changes needed in each repo's synapse-deploy.yml
# to run the artifact sync as a pre-deployment step.
#
# The sync runs AFTER azure login but BEFORE stopping triggers and deploying.
# If the sync fails, the deployment does not proceed.
###############################################################################

# ─────────────────────────────────────────────────────────────────────────────
# FILE: synapse-repo-dev/synapse-deploy.yml
#
# Add the sync script to the dev repo's scripts/ directory.
# In the deploy-dev job, add this step AFTER "Azure Login" and
# BEFORE "Stop triggers":
# ─────────────────────────────────────────────────────────────────────────────

#     - name: Sync linked services to dev workspace
#       run: |
#         python3 scripts/sync-workspace-artifacts.py \
#           --source synapse-workspace-dev \
#           --targets synapse-workspace-dev \
#           --artifact-types linked-service \
#           --compare
#       # Dev just validates — it IS the source of truth.
#       # Non-zero exit means test/prod have services dev doesn't.
#       # This is informational on dev; the real sync happens downstream.
#       continue-on-error: true


# ─────────────────────────────────────────────────────────────────────────────
# FILE: synapse-repo-test/.github/workflows/synapse-deploy.yml
#
# This is the critical integration point.  The sync runs in the deploy-test
# job after Azure login, before trigger management and artifact deployment.
# It ensures synapse-workspace-test contains every linked service that
# synapse-workspace-dev has, so the ARM template deploys cleanly.
#
# The following shows the full modified job with the sync step highlighted.
# ─────────────────────────────────────────────────────────────────────────────

# name: "Synapse Deploy"
#
# on:
#   workflow_dispatch:
#     inputs:
#       source_tag:
#         description: "Release tag from synapse-repo-dev"
#         required: true
#         type: string
#       deploy_mode:
#         ...
#
# jobs:
#   deploy-test:
#     name: "Deploy → Test"
#     runs-on: ubuntu-latest
#     environment: test
#     steps:
#       - name: Checkout test repo
#         uses: actions/checkout@v4
#         with:
#           path: test-repo
#
#       - name: Checkout dev repo at ${{ inputs.source_tag }}
#         uses: actions/checkout@v4
#         with:
#           repository: "${{ env.ORG }}/${{ env.DEV_REPO }}"
#           ref: ${{ inputs.source_tag }}
#           token: ${{ secrets.GH_APP_TOKEN }}
#           path: dev-artifacts
#
#       - name: Setup Python
#         uses: actions/setup-python@v5
#         with:
#           python-version: "3.11"
#
#       - name: Azure Login
#         uses: azure/login@v2
#         with:
#           creds: |
#             {
#               "clientId": "${{ secrets.SYNAPSE_SPN_ID }}",
#               "clientSecret": "${{ secrets.SYNAPSE_SPN_SECRET }}",
#               "subscriptionId": "${{ secrets.AZURE_SUBSCRIPTION_ID }}",
#               "tenantId": "${{ secrets.AZURE_TENANT_ID }}"
#             }
#
#       #┌──────────────────────────────────────────────────────────┐
#       #│  NEW: Sync linked services before deployment             │
#       #│                                                          │
#       #│  This ensures synapse-workspace-test contains every      │
#       #│  linked service name that synapse-workspace-dev has.     │
#       #│  Missing services are created as identical copies.       │
#       #│  They may not be functional (wrong connection targets)   │
#       #│  but that's OK — the ARM template deploy will update     │
#       #│  their properties via parameter overrides.               │
#       #└──────────────────────────────────────────────────────────┘
#       - name: Sync linked services (dev → test)
#         run: |
#           python3 test-repo/scripts/sync-workspace-artifacts.py \
#             --source "synapse-workspace-dev" \
#             --targets "synapse-workspace-test" \
#             --artifact-types linked-service \
#             --direction source-to-target
#
#       - name: Stop triggers
#         run: |
#           chmod +x test-repo/scripts/toggle-triggers.sh
#           ./test-repo/scripts/toggle-triggers.sh stop "${{ env.WORKSPACE }}"
#
#       - name: Deploy to test
#         uses: Azure/synapse-workspace-deployment@V1.9.1
#         with:
#           TargetWorkspaceName: ${{ env.WORKSPACE }}
#           ...
#
#       - name: Start triggers
#         if: always()
#         ...


# ─────────────────────────────────────────────────────────────────────────────
# FILE: synapse-repo-prod/.github/workflows/synapse-deploy.yml
#
# Identical pattern.  Sync dev → prod before deployment.
# ─────────────────────────────────────────────────────────────────────────────

#       - name: Sync linked services (dev → prod)
#         run: |
#           python3 prod-repo/scripts/sync-workspace-artifacts.py \
#             --source "synapse-workspace-dev" \
#             --targets "synapse-workspace-prod" \
#             --artifact-types linked-service \
#             --direction source-to-target


# ─────────────────────────────────────────────────────────────────────────────
# BIDIRECTIONAL SYNC (optional — for full homogeneity)
#
# If test or prod workspaces have linked services that dev doesn't have
# (because they were created manually or by a different team), you can
# sync them back to dev so the ARM template captures everything.
#
# This should be run as a scheduled job, not inline with deployment,
# because it modifies the source workspace.
# ─────────────────────────────────────────────────────────────────────────────

# name: "Bidirectional Workspace Sync"
# on:
#   schedule:
#     - cron: "0 5 * * 1"   # weekly on Mondays at 05:00 UTC
#   workflow_dispatch:
#
# jobs:
#   sync-all:
#     runs-on: ubuntu-latest
#     steps:
#       - uses: actions/checkout@v4
#       - uses: actions/setup-python@v5
#         with: { python-version: "3.11" }
#       - uses: azure/login@v2
#         with:
#           creds: '...'
#
#       - name: Bidirectional sync (dev ↔ test ↔ prod)
#         run: |
#           # Dev ↔ Test
#           python3 scripts/sync-workspace-artifacts.py \
#             --source synapse-workspace-dev \
#             --targets synapse-workspace-test \
#             --artifact-types linked-service \
#             --direction both
#
#           # Dev ↔ Prod
#           python3 scripts/sync-workspace-artifacts.py \
#             --source synapse-workspace-dev \
#             --targets synapse-workspace-prod \
#             --artifact-types linked-service \
#             --direction both
