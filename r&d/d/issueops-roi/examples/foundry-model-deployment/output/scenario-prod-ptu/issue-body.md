### Foundry resource

aif-cooleng-prod-eus2

### Model name

gpt-4.1

### Model version

2025-04-14

### Model format (publisher)

OpenAI

### Deployment type (SKU)

GlobalProvisionedManaged

### Capacity

100

### Deployment name

gpt-4.1-agents

### Version upgrade option

NoAutoUpgrade

### Intended agentic use

Production backing model for the AKS agent runner (agentic-task-request, aks-foundry-agent backend) and the incident-summary agent. Predictable latency is required during incident response, so provisioned throughput is requested.

### Consuming repositories or services

CoolEngOrg/issueops (agent runner), CoolEngOrg/incident-bot

### Chargeback cost center

CC-3000-PLATFORM

### Review date

2027-03-31

### Acknowledgements

- [X] Only data classified Internal or lower will be sent to this deployment unless it is approved for Confidential data
- [X] I understand provisioned (PTU) deployments are billed hourly whether or not they are used
