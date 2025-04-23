# Installing Dynatrace Operator on Azure Kubernetes Service (AKS)

This guide provides instructions for installing the Dynatrace Operator on Azure Kubernetes Service (AKS) using Helm, along with required network configurations.

## Prerequisites

Before installing Dynatrace Operator on your AKS cluster, ensure you meet the following requirements:

- Your `kubectl` CLI is connected to the AKS cluster you want to monitor
- You have sufficient privileges on the cluster to run `kubectl` commands
- You must allow egress for Dynatrace pods (default: `dynatrace` namespace) to your Dynatrace environment URL
- Helm version 3 is installed

## Installation Instructions for Helm Chart

### 1. Create the Dynatrace namespace

```bash
kubectl create namespace dynatrace
```

### 2. Apply the Custom Resource Definition (CRD)

```bash
kubectl apply -f https://github.com/Dynatrace/dynatrace-operator/releases/latest/download/dynatrace.com_dynakubes.yaml
```

### 3. Add the Dynatrace Helm repository

```bash
helm repo add dynatrace https://raw.githubusercontent.com/Dynatrace/dynatrace-operator/main/config/helm/repos/stable
helm repo update
```

### 4. Generate API and PaaS tokens in your Dynatrace environment

You'll need to generate the following tokens in your Dynatrace environment:
- API Token
- PaaS Token

For information on creating these tokens, refer to the [Dynatrace documentation](https://www.dynatrace.com/support/help/reference/dynatrace-concepts/why-do-i-need-an-environment-id/#create-user-generated-access-tokens).

### 5. Install the Dynatrace Operator using Helm

```bash
helm install dynatrace-operator oci://public.ecr.aws/dynatrace/dynatrace-operator \
  -n dynatrace \
  --set apiUrl="https://YOUR_ENVIRONMENT_ID.live.dynatrace.com/api" \
  --set apiToken="YOUR_API_TOKEN" \
  --set paasToken="YOUR_PAAS_TOKEN"
```

> **Note:** Replace `YOUR_ENVIRONMENT_ID`, `YOUR_API_TOKEN`, and `YOUR_PAAS_TOKEN` with your actual values.

Alternatively, you can use a values.yaml file:

1. Create a values.yaml file with your configuration
2. Run the installation command:

```bash
helm install dynatrace-operator oci://public.ecr.aws/dynatrace/dynatrace-operator \
  -n dynatrace \
  -f values.yaml
```

### 6. Create a DynaKube custom resource

Create a DynaKube custom resource to configure the Dynatrace Operator. The configuration depends on your monitoring needs (application monitoring, full-stack monitoring, or host monitoring).

Example for full-stack monitoring:

```bash
kubectl -n dynatrace create secret generic dynakube \
  --from-literal="apiToken=YOUR_API_TOKEN" \
  --from-literal="dataIngestToken=YOUR_DATA_INGEST_TOKEN"

# Apply the DynaKube resource
kubectl apply -f dynakube.yaml
```

Example dynakube.yaml for full-stack monitoring:

```yaml
apiVersion: dynatrace.com/v1beta1
kind: DynaKube
metadata:
  name: dynakube
  namespace: dynatrace
spec:
  apiUrl: https://YOUR_ENVIRONMENT_ID.live.dynatrace.com/api
  cloudNativeFullStack:
    enabled: true
  skipCertCheck: false
  networkZone: YOUR_NETWORK_ZONE  # Optional
```

## Required Network Traffic Configurations

To ensure the Dynatrace Operator functions correctly, configure the following network settings:

### 1. Egress Requirements

All resources in the Dynatrace Operator namespace (default: `dynatrace`) need to be able to resolve DNS requests. The default port is TCP 443, but this may vary depending on your setup.

Key egress requirements:

- Allow egress from the Dynatrace namespace to your Dynatrace environment URL (typically via port 443)
- Allow DNS resolution for all resources in the Dynatrace namespace
- Allow communication with Kubernetes API server

### 2. Network Policies

If you're using network policies in your AKS cluster (especially a deny-all policy), you need to create policies to allow Dynatrace Operator components to communicate properly.

Create the following network policies to ensure proper operation:

#### Dynatrace Operator egress policy

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: dynatrace-operator-egress
  namespace: dynatrace
spec:
  podSelector: {}
  policyTypes:
  - Egress
  egress:
  - to:
    - ipBlock:
        cidr: YOUR_DYNATRACE_ENVIRONMENT_IP/32
    ports:
    - protocol: TCP
      port: 443  # Adjust if your environment uses a different port
```

#### DNS resolution policy

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-dns-egress
  namespace: dynatrace
spec:
  podSelector: {}
  policyTypes:
  - Egress
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: "kube-system"
      podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - port: 53
      protocol: UDP
    - port: 53
      protocol: TCP
```

#### Kubernetes API access policy

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-api-server-egress
  namespace: dynatrace
spec:
  podSelector: {}
  policyTypes:
  - Egress
  egress:
  - to:
    - ipBlock:
        cidr: YOUR_KUBERNETES_API_SERVER_IP/32
    ports:
    - protocol: TCP
      port: 443  # Default API server port, adjust if needed
```

### 3. AKS-specific network considerations

For AKS clusters, ensure that:

If you have an app or solution that needs to talk to the API server, you must either add an additional network rule to allow TCP communication to port 443 of your API server's IP OR, if you have a layer 7 firewall configured to allow traffic to the API Server's domain name, set kubernetes.azure.com/set-kube-service-host-fqdn in your pod specs.

## Verifying the Installation

To verify that the Dynatrace Operator has been installed correctly:

```bash
kubectl get pods -n dynatrace
```

You should see pods for the Dynatrace Operator and, depending on your configuration, pods for OneAgent and/or ActiveGate.

To check when Dynatrace Operator components finish initialization:

```bash
kubectl -n dynatrace wait pod --for=condition=ready --selector=app.kubernetes.io/name=dynatrace-operator,app.kubernetes.io/component=webhook --timeout=300s
```

## Troubleshooting

If you encounter issues:

1. Check the Dynatrace Operator logs:
   ```bash
   kubectl logs -n dynatrace -l app.kubernetes.io/name=dynatrace-operator
   ```

2. Verify network connectivity:
   ```bash
   # From a pod in the dynatrace namespace
   kubectl exec -it -n dynatrace POD_NAME -- curl -k https://YOUR_ENVIRONMENT_ID.live.dynatrace.com/api
   ```

3. Check for events in the dynatrace namespace:
   ```bash
   kubectl get events -n dynatrace
   ```

## Additional Resources

- [Dynatrace Operator Documentation](https://docs.dynatrace.com/docs/setup-and-configuration/setup-on-k8s)
- [Dynatrace Operator GitHub Repository](https://github.com/Dynatrace/dynatrace-operator)
- [Dynatrace Helm Charts Repository](https://github.com/Dynatrace/helm-charts)
