### Agent backend

aks-foundry-agent

### Target repository

platform-infra

### Task title

Runbook: zero-downtime AKS node-pool upgrade to Kubernetes 1.30

### Objective

Produce an on-call runbook for upgrading the production AKS cluster's user node pools to Kubernetes 1.30 without dropping traffic, including pre-checks, surge/drain settings, stuck-drain handling and rollback.

### Acceptance criteria

- Uses az CLI and kubectl 1.30 commands only
- Covers PodDisruptionBudget checks before draining
- Includes a rollback section
- Fits on two printed pages

### Catalog workflow

none

### Data classification

confidential

### Acknowledgements

- [X] I will review all agent output (pull requests, documents) before it is merged or relied on
- [X] The objective and answers contain no secrets, credentials, or personal data
