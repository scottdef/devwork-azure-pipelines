## Task

Add idempotency keys to POST /v2/refunds

## Objective

Retries from the mobile app occasionally create duplicate refunds. Add Idempotency-Key header support to POST /v2/refunds, persisting keys for 24 hours and returning the original response for duplicates.

## Acceptance criteria

- [ ] Duplicate requests with the same key return the original 201 response
- [ ] Keys expire after 24 hours
- [ ] Unit and integration tests cover duplicate, expired and missing keys
- [ ] OpenAPI spec updated

## Scope

- **In scope:** `src/main/java/com/coolengorg/payments/refunds/**, src/main/resources/openapi/refunds.yaml`
- **Do not change:** Do not change the ledger module or database migrations other than adding the idempotency table
- **Verify with:** `./gradlew test integrationTest`

---
Requested through IssueOps https://github.com/CoolEngOrg/issueops/issues/402 by gia-pay (data classification: `internal`, spec digest `e1519e9b8f48`).
