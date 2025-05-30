# Network Testing in AKS Cluster using kubectl

This guide shows how to run `nslookup` and `curl` commands within your AKS cluster using only kubectl. These commands help test DNS resolution and external connectivity from within the cluster network.

## Method 1: Using Temporary Pods with kubectl run

### Run nslookup within the cluster

```bash
# Create temporary pod with busybox (includes nslookup)
kubectl run dns-test --image=busybox --rm -it --restart=Never -- nslookup google.com

# Alternative: Run nslookup for a specific service within cluster
kubectl run dns-test --image=busybox --rm -it --restart=Never -- nslookup kubernetes.default.svc.cluster.local

# Test DNS resolution for different domains
kubectl run dns-test --image=busybox --rm -it --restart=Never -- nslookup github.com
```

### Run curl to github.com within the cluster

```bash
# Create temporary pod with curl
kubectl run curl-test --image=curlimages/curl --rm -it --restart=Never -- curl -I https://github.com

# Test with verbose output
kubectl run curl-test --image=curlimages/curl --rm -it --restart=Never -- curl -v https://github.com

# Test connectivity and download speed
kubectl run curl-test --image=curlimages/curl --rm -it --restart=Never -- curl -w "Total time: %{time_total}s\n" -o /dev/null -s https://github.com
```

## Method 2: Using Network Troubleshooting Pod (Recommended)

### Create a comprehensive network testing pod

```bash
# Run nicolaka/netshoot pod (includes nslookup, curl, dig, ping, etc.)
kubectl run netshoot --image=nicolaka/netshoot --rm -it --restart=Never -- /bin/bash

# Inside the pod, you can run:
# nslookup github.com
# curl -I https://github.com
# dig github.com
# ping -c 3 8.8.8.8
# exit
```

### Individual commands with netshoot

```bash
# Run nslookup
kubectl run netshoot --image=nicolaka/netshoot --rm -it --restart=Never -- nslookup github.com

# Run curl to github.com
kubectl run netshoot --image=nicolaka/netshoot --rm -it --restart=Never -- curl -I https://github.com

# Run multiple network tests
kubectl run netshoot --image=nicolaka/netshoot --rm -it --restart=Never -- sh -c "nslookup github.com && curl -I https://github.com"
```

## Method 3: Using Existing Pods (if available)

### If you have the Grafana pod running

```bash
# Check if Grafana pod exists
kubectl get pods -n grafana

# Exec into Grafana pod and run commands
kubectl exec -it -n grafana deployment/grafana -- /bin/sh

# Inside the pod:
# nslookup github.com  # (if available)
# wget -O- https://github.com  # (wget might be available instead of curl)
```

### If using Alpine-based pods

```bash
# Create Alpine pod and install tools
kubectl run alpine-test --image=alpine --rm -it --restart=Never -- sh

# Inside the pod, install tools:
# apk add --no-cache curl bind-tools
# nslookup github.com
# curl -I https://github.com
```

## Method 4: Running Commands Without Interactive Shell

### Non-interactive nslookup commands

```bash
# Test DNS resolution for github.com
kubectl run dns-test-github --image=busybox --rm --restart=Never -- nslookup github.com

# Test internal Kubernetes DNS
kubectl run dns-test-internal --image=busybox --rm --restart=Never -- nslookup kubernetes.default

# Test specific DNS server
kubectl run dns-test-custom --image=busybox --rm --restart=Never -- nslookup github.com 8.8.8.8
```

### Non-interactive curl commands

```bash
# Simple connectivity test to github.com
kubectl run curl-github --image=curlimages/curl --rm --restart=Never -- curl -s -o /dev/null -w "HTTP Status: %{http_code}\nTotal Time: %{time_total}s\n" https://github.com

# Test with specific timeout
kubectl run curl-timeout --image=curlimages/curl --rm --restart=Never -- curl --connect-timeout 10 -I https://github.com

# Test GitHub API
kubectl run curl-api --image=curlimages/curl --rm --restart=Never -- curl -s https://api.github.com/zen
```

## Method 5: Persistent Network Testing Pod

### Create a long-running network testing pod

```bash
# Create persistent network testing deployment
cat > network-test-pod.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: network-test
  namespace: default
spec:
  containers:
  - name: netshoot
    image: nicolaka/netshoot
    command: ["/bin/sleep"]
    args: ["3600"]  # Sleep for 1 hour
  restartPolicy: Never
EOF

# Apply the pod
kubectl apply -f network-test-pod.yaml

# Wait for pod to be ready
kubectl wait --for=condition=Ready pod/network-test --timeout=60s

# Now run commands in the persistent pod
kubectl exec -it network-test -- nslookup github.com
kubectl exec -it network-test -- curl -I https://github.com

# Run multiple tests
kubectl exec -it network-test -- sh -c "
echo '=== DNS Test ==='
nslookup github.com
echo '=== Curl Test ==='
curl -I https://github.com
echo '=== Connectivity Test ==='
curl -w 'Total: %{time_total}s\n' -o /dev/null -s https://github.com
"

# Clean up when done
kubectl delete pod network-test
```

## Method 6: Testing Cluster Network Policies and Egress

### Test network connectivity from different namespaces

```bash
# Test from default namespace
kubectl run test-default --image=curlimages/curl --rm --restart=Never -- curl -I https://github.com

# Test from grafana namespace (if exists)
kubectl run test-grafana -n grafana --image=curlimages/curl --rm --restart=Never -- curl -I https://github.com

# Test from kube-system namespace
kubectl run test-system -n kube-system --image=curlimages/curl --rm --restart=Never -- curl -I https://github.com
```

### Comprehensive network connectivity test

```bash
# Create comprehensive test script
kubectl run network-comprehensive --image=nicolaka/netshoot --rm -it --restart=Never -- sh -c "
echo '===================='
echo 'Comprehensive Network Test'
echo '===================='
echo 'DNS Resolution Tests:'
nslookup github.com
nslookup google.com
nslookup kubernetes.default.svc.cluster.local
echo
echo 'HTTP Connectivity Tests:'
curl -I https://github.com
curl -I https://google.com
echo
echo 'Internal Service Tests:'
curl -I http://kubernetes.default.svc.cluster.local
echo
echo 'Network Interface Info:'
ip addr show eth0
echo
echo 'Routing Table:'
ip route
echo '===================='
"
```

## Quick Reference Commands

```bash
# Quick nslookup
kubectl run dns --image=busybox --rm -it --restart=Never -- nslookup github.com

# Quick curl
kubectl run curl --image=curlimages/curl --rm -it --restart=Never -- curl -I https://github.com

# Interactive network troubleshooting
kubectl run debug --image=nicolaka/netshoot --rm -it --restart=Never -- /bin/bash

# Clean up any failed pods
kubectl delete pods --field-selector=status.phase=Failed
```

## Common Docker Images for Network Testing

| Image | Description | Tools Included |
|-------|-------------|----------------|
| `busybox` | Minimal Linux with basic utilities | `nslookup`, `wget`, `ping` |
| `curlimages/curl` | Minimal image with curl | `curl` |
| `nicolaka/netshoot` | Network troubleshooting toolkit | `curl`, `nslookup`, `dig`, `ping`, `netstat`, `ss`, `tcpdump` |
| `alpine` | Minimal Linux distribution | Package manager to install tools |

## Explanation of kubectl Flags

- `--rm`: Automatically delete the pod when it exits
- `-it`: Interactive terminal (stdin + tty)
- `--restart=Never`: Don't restart the pod if it fails
- `--image`: Docker image to use for the pod
- `--`: Separates kubectl options from the command to run in the container

## Expected Output Examples

### Successful nslookup output:
```
Server:    10.0.0.10
Address 1: 10.0.0.10 kube-dns.kube-system.svc.cluster.local

Name:      github.com
Address 1: 140.82.114.4
```

### Successful curl output:
```
HTTP/2 200
server: GitHub.com
content-type: text/html; charset=utf-8
...
```

These commands help verify that your AKS cluster has proper DNS resolution and external network connectivity, which are essential for downloading Helm charts from private repositories and accessing external services.
