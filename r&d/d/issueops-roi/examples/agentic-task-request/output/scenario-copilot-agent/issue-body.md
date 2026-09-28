### Agent backend

copilot-cloud-agent

### Target repository

CoolEngOrg/payments-api

### Task title

Add idempotency keys to POST /v2/refunds

### Objective

Retries from the mobile app occasionally create duplicate refunds. Add Idempotency-Key header support to POST /v2/refunds, persisting keys for 24 hours and returning the original response for duplicates.

### Acceptance criteria

- Duplicate requests with the same key return the original 201 response
- Keys expire after 24 hours
- Unit and integration tests cover duplicate, expired and missing keys
- OpenAPI spec updated

### Catalog workflow

none

### Data classification

internal

### Acknowledgements

- [X] I will review all agent output (pull requests, documents) before it is merged or relied on
- [X] The objective and answers contain no secrets, credentials, or personal data
