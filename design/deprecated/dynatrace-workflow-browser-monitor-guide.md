# Dynatrace Workflow — Create Synthetic Browser Monitor

**Usage and Post-Creation Editing Guide**

> Go 1.21 | kubectl 1.30 | Azure Kubernetes Service

---

## Table of Contents

1. [Overview](#1-overview)
2. [Prerequisites](#2-prerequisites)
3. [Workflow Architecture](#3-workflow-architecture)
4. [Running the Workflow](#4-running-the-workflow)
5. [Editing the Monitor in the Dynatrace UI](#5-editing-the-monitor-in-the-dynatrace-ui)
6. [Troubleshooting](#6-troubleshooting)
7. [Quick Reference](#7-quick-reference)

---

## 1. Overview

The "Create Synthetic Browser Monitor" workflow automates the provisioning of a Dynatrace synthetic browser monitor via the Dynatrace Automations platform. Rather than manually configuring a monitor through the Dynatrace UI, the workflow accepts structured parameters, validates them, constructs the full API request body, and calls the Synthetic Monitors REST API on your behalf.

Once the workflow completes successfully, the new monitor appears immediately in the Dynatrace Synthetics catalog. From that point forward, all further configuration changes — URLs, frequency, locations, anomaly thresholds, and script steps — are made directly through the Dynatrace UI as described in [Section 5](#5-editing-the-monitor-in-the-dynatrace-ui).

### 1.1 Included Files

| File | Type | Purpose |
|------|------|---------|
| `dynatrace-workflow-create-browser-monitor.json` | Workflow Definition | Import into Dynatrace Automations to register the workflow. |
| `workflow-submitter.go` | Go 1.21 CLI Tool | Upserts the workflow and triggers a live execution from the command line or a Kubernetes Job. |
| `dynatrace-workflow-submitter.yaml` | Kubernetes Manifests | AKS Job, Secret, ConfigMap, and ServiceAccount for running the submitter on-cluster. |
| `go.mod` | Go Module File | Declares the Go module. No external dependencies — standard library only. |

---

## 2. Prerequisites

### 2.1 Dynatrace API Token

An API token with the following scopes is required. Tokens are created under **Settings > Access tokens** in the Dynatrace UI.

| Scope | Token Permission Name | Used For |
|-------|-----------------------|----------|
| `ExternalSyntheticIntegration` | Create and read synthetic monitors | `POST /api/v1/synthetic/monitors` |
| `WriteConfig` | Write configuration | Upsert workflow via Automations API |
| `ReadConfig` | Read configuration | Check for existing workflow by title |

> **⚠️ WARNING**
> Do not embed the API token in source code or commit it to version control. Supply it via an environment variable or a Kubernetes Secret as described in [Section 4](#4-running-the-workflow).

### 2.2 Required Environment Variables

Two environment variables must be set before running the workflow submitter. They may be passed as CLI flags (`-tenant-url`, `-api-token`) or read from the environment.

| Variable | Description |
|----------|-------------|
| `DT_TENANT_URL` | Full Dynatrace tenant base URL. Example: `https://abc12345.live.dynatrace.com` |
| `DT_API_TOKEN` | Dynatrace API token with `ExternalSyntheticIntegration` and `WriteConfig` scopes. |

### 2.3 Dynatrace Location Entity IDs

Each synthetic monitor must be assigned to at least one Dynatrace location. Locations are identified by entity IDs of the form `GEOLOCATION-XXXXXXXXXXXXXXXX` or `SYNTHETIC_LOCATION-XXXXXXXXXXXXXXXX`. To find your available IDs, navigate to **Synthetic > Locations** in the Dynatrace UI, or query the API:

```
GET https://{tenant-url}/api/v1/synthetic/locations
```

### 2.4 Application Entity ID

The monitor is linked to a Dynatrace application entity. Application IDs take the form `APPLICATION-XXXXXXXXXXXXXXXX` and can be found under **Applications & Microservices > Applications** in the Dynatrace UI.

### 2.5 Go 1.21 (CLI usage only)

The Go submitter requires Go 1.21 or later and uses only the standard library. Install from <https://go.dev/dl> or via your package manager. This prerequisite is not needed when deploying via the AKS Kubernetes Job — the init container handles the build on-cluster.

---

## 3. Workflow Architecture

### 3.1 Task Execution Graph

The workflow is a linear pipeline with a parallel error handler. Tasks run sequentially from top to bottom; any task failure routes execution to `handle_error` via an OR-linked condition.

```
validate_inputs  →  build_monitor_payload  →  create_synthetic_monitor  →  verify_and_log_result
                                                                                       ↓
                                                          Any ERROR  →  handle_error ──┘
```

| Step | Task Name | Responsibility |
|------|-----------|----------------|
| 1 | `validate_inputs` | Reads `monitorName` from `exec.variables` and remaining fields from `exec.params`. Throws a descriptive error for any missing or invalid value before any API call is made. |
| 2 | `build_monitor_payload` | Assembles the full JSON body for `POST /api/v1/synthetic/monitors`, sourcing `monitorName` from the workflow variable and all other fields from execution parameters. |
| 3 | `create_synthetic_monitor` | Executes the HTTP POST to `/api/v1/synthetic/monitors` using the `dynatrace.automations:http-function` action. Credentials are injected from `DT_TENANT_URL` and `DT_API_TOKEN` environment bindings. |
| 4 | `verify_and_log_result` | Inspects the HTTP response status code. On HTTP 200/201 it extracts and returns the `entityId` and a direct URL to the new monitor. On any other status it throws with the API error message. |
| 5 | `handle_error` | Runs if **any** upstream task reaches an ERROR state (OR condition). Iterates over all tasks, logs the failed state and result payload to the execution console for operator review. |

### 3.2 Workflow Variable: `monitorName`

The monitor display name is stored as a workflow-level variable rather than an execution parameter. This means it can be changed in the Dynatrace Variables panel without modifying execution payloads or redeploying the submitter.

| Field | Value |
|-------|-------|
| Variable name | `monitorName` |
| Default value | `Browser Monitor Example` |
| Where to edit | Dynatrace UI → Automations → [workflow] → **Variables** panel |
| Validation | `validate_inputs` verifies the variable is non-empty before proceeding. |

### 3.3 Execution Parameters

The following parameters are supplied at execution time — either via the Go CLI flags or the Kubernetes Job environment variables. All other monitor settings use the defaults shown.

| Parameter | CLI Flag | Default / Notes |
|-----------|----------|-----------------|
| `targetUrl` | `-url` | **Required.** Full URL to navigate to, e.g. `https://app.example.com` |
| `frequencyMin` | `-freq` | **Required.** Integer >= 5. Monitor polling interval in minutes. |
| `locationIds` | `-locations` | **Required.** Comma-separated `GEOLOCATION-...` or `SYNTHETIC_LOCATION-...` IDs. |
| `applicationId` | `-app-id` | **Required.** `APPLICATION-...` entity ID. |
| `tags` | `-tags` | Optional. Comma-separated strings, e.g. `production,aks`. |
| `deviceName` | `-device` | Default: `Desktop`. Browser device profile name. |
| `waitFor` | `-wait-for` | Default: `page_complete`. Also accepts `network_idle`. |
| `loadingTimeThresholdMs` | `-load-time-ms` | Default: `10000` ms. Loading time anomaly detection threshold. |
| `loadActionKpm` | *(not exposed)* | Default: `VISUALLY_COMPLETE`. Key performance metric for load actions. |
| `xhrActionKpm` | *(not exposed)* | Default: `VISUALLY_COMPLETE`. Key performance metric for XHR actions. |
| `localOutageAffectedLocations` | *(not exposed)* | Default: `1`. Locations affected before local outage is declared. |
| `localOutageConsecutiveRuns` | *(not exposed)* | Default: `3`. Consecutive failures before local outage is declared. |

---

## 4. Running the Workflow

### 4.1 Step 1 — Import the Workflow into Dynatrace

Before the submitter can trigger an execution, the workflow definition must be registered in your Dynatrace tenant. The Go submitter performs this step automatically (see [Section 4.3](#43-step-3a--run-via-the-go-cli-submitter)), but it can also be done manually.

#### Manual import via the Dynatrace UI

1. Log in to your Dynatrace tenant.
2. Navigate to **Automations** in the left-side navigation menu.
3. Click the **Upload** or **Import** button in the top-right area of the Automations page.
4. Select the file `dynatrace-workflow-create-browser-monitor.json`.
5. Confirm the import. The workflow titled **"Create Synthetic Browser Monitor"** appears in your workflow list.

> **📝 NOTE**
> The Go submitter performs an upsert — if a workflow with the title "Create Synthetic Browser Monitor" already exists it is updated via `PUT`; otherwise a new one is created via `POST`. Manual import is therefore optional when using the submitter.

---

### 4.2 Step 2 — Set the `monitorName` Variable

The monitor display name is controlled by a workflow variable. Before the first run, verify or update this value.

1. In the Dynatrace UI, open **Automations** and locate **"Create Synthetic Browser Monitor"**.
2. Click the workflow title to open the workflow editor.
3. In the left panel, click **Variables**.
4. Locate the `monitorName` variable. The default value is `"Browser Monitor Example"`.
5. Click the pencil icon to edit. Enter the desired monitor name and click **Save**.

> **📝 NOTE**
> Changes to the variable take effect immediately for all subsequent executions. No redeployment or code change is required.

---

### 4.3 Step 3A — Run via the Go CLI Submitter

The Go CLI tool compiles with no external dependencies and can be run from any workstation with Go 1.21 installed.

#### Build the binary

```bash
go build -o workflow-submitter ./cmd/workflow-submitter
```

#### Execute (all required flags shown)

```bash
export DT_TENANT_URL="https://abc12345.live.dynatrace.com"
export DT_API_TOKEN="dt0c01.SAMPLE_TOKEN"

./workflow-submitter \
  -workflow ./dynatrace-workflow-create-browser-monitor.json \
  -name     "My App Health Check" \
  -url      "https://app.example.com" \
  -freq     5 \
  -locations "GEOLOCATION-9999453BE4BDB3CD,SYNTHETIC_LOCATION-DF80ACFB688C583B" \
  -app-id   "APPLICATION-4ADF0EF407C7C545" \
  -tags     "production,aks"
```

#### Dry-run mode (no API calls)

Add the `-dry-run` flag to print the workflow definition and resolved execution parameters to stdout without contacting Dynatrace. This is useful for pre-flight validation.

```bash
./workflow-submitter -dry-run \
  -workflow ./dynatrace-workflow-create-browser-monitor.json \
  -name "Test" -url "https://test.example.com" -freq 5 \
  -locations "GEOLOCATION-..." -app-id "APPLICATION-..."
```

#### All available flags

| Flag | Environment Variable | Description |
|------|----------------------|-------------|
| `-tenant-url` | `DT_TENANT_URL` | Dynatrace tenant base URL. Required unless `-dry-run` is set. |
| `-api-token` | `DT_API_TOKEN` | Dynatrace API token. Required unless `-dry-run` is set. |
| `-workflow` | — | Path to workflow JSON file. Default: `dynatrace-workflow-create-browser-monitor.json` |
| `-name` | — | Monitor name override. Required. |
| `-url` | — | Target URL to monitor. Required. |
| `-freq` | — | Polling frequency in minutes (>= 5). Default: `5` |
| `-locations` | — | Comma-separated location entity IDs. Required. |
| `-app-id` | — | Dynatrace `APPLICATION-...` entity ID. Required. |
| `-tags` | — | Comma-separated tags. Optional. |
| `-device` | — | Browser device profile. Default: `Desktop` |
| `-wait-for` | — | Wait condition: `page_complete` or `network_idle`. Default: `page_complete` |
| `-load-time-ms` | — | Loading time anomaly threshold (ms). Default: `10000` |
| `-poll-sec` | — | Execution poll interval in seconds. Default: `10` |
| `-timeout-sec` | — | Maximum wait for execution completion (seconds). Default: `300` |
| `-dry-run` | — | Print resolved config without calling the API. Default: `false` |

---

### 4.4 Step 3B — Run via Kubernetes Job on AKS

The Kubernetes manifests in `dynatrace-workflow-submitter.yaml` deploy the submitter as a one-shot Job on AKS. An init container compiles the Go binary from source; the main container executes it.

#### 1. Encode credentials as base64

```bash
DT_URL_B64=$(echo -n "https://abc12345.live.dynatrace.com" | base64)
DT_TOK_B64=$(echo -n "dt0c01.YOUR_TOKEN"                   | base64)
```

#### 2. Patch the Secret fields in the manifest

```bash
sed -i "s|REPLACE_WITH_BASE64_ENCODED_TENANT_URL|${DT_URL_B64}|" dynatrace-workflow-submitter.yaml
sed -i "s|REPLACE_WITH_BASE64_ENCODED_API_TOKEN|${DT_TOK_B64}|"  dynatrace-workflow-submitter.yaml
```

#### 3. Update monitor parameters in the Job env block

Edit the `env` section of the main container in the Job spec. The relevant variables are:

| Variable | Description |
|----------|-------------|
| `MONITOR_NAME` | Display name of the monitor. Also update the `monitorName` workflow variable in the Dynatrace UI. |
| `MONITOR_URL` | Target URL to navigate to. |
| `MONITOR_FREQ` | Polling frequency in minutes. Minimum: `5`. |
| `MONITOR_LOCATIONS` | Comma-separated `GEOLOCATION-...` or `SYNTHETIC_LOCATION-...` entity IDs. |
| `MONITOR_APP_ID` | `APPLICATION-...` entity ID. |
| `MONITOR_TAGS` | Comma-separated tag strings. May be left empty. |
| `MONITOR_DEVICE` | Browser device profile. Default: `Desktop`. |
| `MONITOR_LOAD_TIME_MS` | Loading time anomaly threshold in milliseconds. Default: `10000`. |

#### 4. Apply the manifests

```bash
kubectl apply -f dynatrace-workflow-submitter.yaml
```

#### 5. Monitor Job progress

```bash
kubectl get job -n dynatrace-ops dynatrace-workflow-submitter -w
```

#### 6. View submitter logs

```bash
kubectl logs -n dynatrace-ops \
  -l app.kubernetes.io/name=dynatrace-workflow-submitter \
  -c submit --follow
```

> **📝 NOTE**
> The Job has `backoffLimit: 2`, meaning Kubernetes will retry the pod up to two times on transient failures. Completed pods are automatically cleaned up after one hour (`ttlSecondsAfterFinished: 3600`).

---

### 4.5 Step 4 — Verify the Execution

When the submitter completes successfully, its final log lines will include the monitor `entityId` and a direct URL:

```
[workflow-submitter] Execution completed successfully: status=SUCCEEDED
[workflow-submitter] Monitor 'My App Health Check' provisioned at https://abc12345.live.dynatrace.com
```

To verify within Dynatrace:

1. Open **Automations** and locate **"Create Synthetic Browser Monitor"**.
2. Click the **Execution history** (clock icon) to view past runs.
3. Click the latest execution to see per-task status and log output.
4. The `verify_and_log_result` task output contains the `entityId` and a direct link to the new monitor.

---

## 5. Editing the Monitor in the Dynatrace UI

After the workflow creates a synthetic monitor, all subsequent configuration changes are made through the Dynatrace Synthetics UI. No workflow re-run is needed to modify an existing monitor.

### 5.1 Navigating to the Monitor

1. Log in to your Dynatrace tenant.
2. In the left navigation, expand **Synthetic** and click **Synthetic monitors**. Alternatively, type the monitor name in the global search bar at the top of the screen.
3. Locate your monitor by name in the monitor list.
4. Click the monitor name to open its detail view.
5. Click the **Edit** button (pencil icon) in the top-right corner of the detail page to open the monitor editor.

> **💡 TIP**
> If you know the `entityId` from the workflow output (format: `SYNTHETIC_TEST-XXXXXXXXXXXXXXXX`), you can navigate directly to:
> `https://{tenant}/ui/synthetic/monitors/{entityId}`

---

### 5.2 Editing the Monitor Name

1. Open the monitor editor as described in [Section 5.1](#51-navigating-to-the-monitor).
2. The monitor name field appears at the very top of the editor, above the script area.
3. Click the name field and edit the text directly.
4. Click **Save** at the bottom of the page. The new name is reflected immediately across the Dynatrace UI.

> **📝 NOTE**
> Changing the monitor name in the UI does not update the `monitorName` workflow variable. If you plan to re-run the workflow in future to provision another environment, update the variable in the Variables panel to match your desired name.

---

### 5.3 Changing the Target URL

1. Open the monitor editor.
2. Locate the **Script** section. This displays the event steps that make up the browser script.
3. Find the `navigate` event step — its description will read `Loading of "[your URL]"`.
4. Click the step to expand it.
5. Edit the **URL** field with the new target address.
6. Click **Save**.

---

### 5.4 Changing Frequency and Locations

1. Open the monitor editor.
2. Click the **Settings** tab within the editor (not the global Settings menu).
3. Under **Frequency**, use the dropdown to select a new polling interval. The minimum supported value is 5 minutes.
4. Under **Locations**, use the search box to add or remove monitoring locations. At least one location must remain selected.
5. Click **Save**.

> **📝 NOTE**
> Removing all locations and saving will disable the monitor. Dynatrace requires at least one active location for a monitor to run.

---

### 5.5 Editing Anomaly Detection Settings

1. Open the monitor editor.
2. Click the **Anomaly detection** tab within the editor.
3. **Outage handling:** Toggle the **Global outage** and **Local outage** switches as required. For local outage, set the number of affected locations and consecutive failed runs that must occur before an alert is raised.
4. **Loading time thresholds:** Click **Add threshold** to define a new threshold, or click an existing threshold to edit its value. The threshold type `TOTAL` measures the full page load time in milliseconds.
5. Click **Save**.

---

### 5.6 Editing the Browser Script

The browser script defines the sequence of actions the synthetic agent performs on each execution. The workflow creates a single `navigate` event; you can add further steps in the UI.

1. Open the monitor editor.
2. In the **Script** section, the existing `navigate` step is shown. To add a step, click **Add event** below the last step.
3. Choose an event type from the dialog. Common types include:
   - `navigate` — load a new URL
   - `click` — click a CSS selector or element
   - `keystrokes` — type text into a field
   - `selectOption` — choose a value in a dropdown
   - `javaScript` — execute arbitrary JavaScript on the page
4. Configure the required fields for the chosen event type (URL, selector, value, etc.).
5. To reorder steps, drag the step handle on the left side of each step row.
6. To delete a step, click the trash icon on the right side of the step row.
7. Click **Save** when finished.

> **💡 TIP**
> Use the **Record** button at the top of the script editor to capture interactions from a live browser session. Recorded steps are automatically converted into script events.

---

### 5.7 Managing Tags

1. Open the monitor editor.
2. Scroll to the **Tags** section at the bottom of the **Settings** tab.
3. To add a tag, click **Add tag** and type the desired label. Press Enter to confirm.
4. To remove a tag, click the **X** next to the tag name.
5. Click **Save**.

---

### 5.8 Assigning or Changing the Application

1. Open the monitor editor.
2. Navigate to the **Settings** tab.
3. Under **Linked applications**, click the search field.
4. Search for the application by name. Select it to create the association.
5. To remove the existing application link, click the **X** next to the current application entry.
6. Click **Save**.

---

### 5.9 Enabling and Disabling the Monitor

A monitor can be paused without deleting it. This is useful during maintenance windows or to temporarily stop alerting.

1. Open the monitor detail view (without entering the editor).
2. Use the **Enabled / Disabled** toggle in the top-right area of the page.
3. The monitor stops executing immediately when disabled and resumes on the next scheduled interval when re-enabled.

> **📝 NOTE**
> Disabling a monitor does not affect historical data. All past execution results remain visible in the execution history.

---

## 6. Troubleshooting

### 6.1 Workflow Execution Failures

| Symptom | Likely Cause | Resolution |
|---------|--------------|------------|
| `validate_inputs` fails: `"monitorName is not set"` | The `monitorName` workflow variable is empty or was cleared. | Open the workflow editor → Variables panel. Set `monitorName` to a non-empty string and save. |
| `validate_inputs` fails: `"Missing required parameters"` | One or more execution parameters (`targetUrl`, `frequencyMin`, `locationIds`, `applicationId`) were not supplied. | Ensure all required CLI flags or Kubernetes env vars are set before triggering the execution. |
| `create_synthetic_monitor` fails: HTTP `401` | The `DT_API_TOKEN` is missing, expired, or lacks the required scopes. | Create a new token with `ExternalSyntheticIntegration` and `WriteConfig` scopes. Update the Secret or env var. |
| `create_synthetic_monitor` fails: HTTP `400` | The request body is malformed or contains an invalid location/application entity ID. | Verify location IDs with `GET /api/v1/synthetic/locations` and the application ID in the Applications UI. |
| `create_synthetic_monitor` fails: HTTP `403` | The API token does not have permission to create synthetic monitors. | Add the `ExternalSyntheticIntegration` scope to the token. |
| Execution times out (`status=TIMEOUT`) | The poll timeout expired before the workflow reached a terminal state. | Increase `-timeout-sec` (CLI) or check the Dynatrace Automations execution log for stuck tasks. |

### 6.2 Kubernetes Job Failures

| Symptom | Resolution |
|---------|------------|
| Init container (`build`) fails with `"go: command not found"` | The `golang:1.21-alpine` image was not pulled. Check AKS node internet egress and ACR pull permissions. |
| Main container exits with `"open workflow file: no such file"` | The ConfigMap volume was not mounted correctly. Run `kubectl describe pod -n dynatrace-ops <pod-name>` and verify `volumeMounts`. |
| Pod stuck in `Pending` state | Insufficient cluster resources. Run `kubectl describe node` and `kubectl describe quota -n dynatrace-ops`. |
| Secret values not decoded | Secret `data` values must be base64-encoded. Re-encode with `echo -n "value" \| base64` and patch the Secret. |

### 6.3 Viewing Workflow Execution Logs

1. Open **Automations** in the Dynatrace UI.
2. Click the three-dot menu on the **"Create Synthetic Browser Monitor"** workflow.
3. Select **Execution history**.
4. Click a specific execution to open its detail view.
5. Click any task name to expand its log output. The `handle_error` task will show which upstream task failed and the associated API error message if applicable.

---

## 7. Quick Reference

### 7.1 API Endpoints Used

| Method | Endpoint | Purpose |
|--------|----------|---------|
| `GET` | `/platform/classic/environment-api/v2/workflows` | List existing workflows (upsert check). |
| `POST` | `/platform/classic/environment-api/v2/workflows` | Create a new workflow. |
| `PUT` | `/platform/classic/environment-api/v2/workflows/{id}` | Update an existing workflow. |
| `POST` | `/platform/classic/environment-api/v2/workflows/{id}/run` | Trigger a manual workflow execution. |
| `GET` | `/platform/classic/environment-api/v2/workflows/{id}/executions/{execId}` | Poll execution status. |
| `POST` | `/api/v1/synthetic/monitors` | Create the synthetic browser monitor (called inside the workflow). |

### 7.2 Monitor Entity ID Format

| Type | Format | Used In |
|------|--------|---------|
| Synthetic test | `SYNTHETIC_TEST-XXXXXXXXXXXXXXXX` | Returned in `entityId` from `verify_and_log_result`. |
| Geolocation | `GEOLOCATION-XXXXXXXXXXXXXXXX` | `locationIds` parameter. |
| Private location | `SYNTHETIC_LOCATION-XXXXXXXXXXXXXXXX` | `locationIds` parameter. |
| Application | `APPLICATION-XXXXXXXXXXXXXXXX` | `applicationId` parameter. |

### 7.3 Minimum CLI Command

The shortest valid invocation requires exactly five flags plus credentials:

```bash
export DT_TENANT_URL="https://abc12345.live.dynatrace.com"
export DT_API_TOKEN="dt0c01.YOUR_TOKEN"

./workflow-submitter \
  -name      "My Monitor"                       \
  -url       "https://example.com"              \
  -freq      5                                  \
  -locations "GEOLOCATION-XXXXXXXXXXXXXXXXX"    \
  -app-id    "APPLICATION-XXXXXXXXXXXXXXXXX"
```

---

*End of Document*
