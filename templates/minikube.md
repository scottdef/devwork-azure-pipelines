# Complete Minikube Guide for CKAs on Ubuntu 18.04

Minikube provides an excellent local development environment that differs significantly from Azure Kubernetes Service in architecture, networking, and operational patterns. This comprehensive guide covers installation through advanced deployment scenarios using kubectl v1.30, Go v1.21, and Grafana OSS v12, with practical examples for Certified Kubernetes Administrators transitioning between local development and cloud production environments.

**Key differences from AKS**: Minikube operates as a single-node cluster with host-only networking requiring tunneling for external access, while AKS provides managed multi-node clusters with native Azure integration. Understanding these architectural differences enables better development workflows and smoother production transitions.

## Installation and initial setup on Ubuntu 18.04

### System prerequisites and Docker installation

Ubuntu 18.04 requires specific preparation steps for minikube compatibility. **Virtualization support** must be enabled in BIOS, verifiable with `grep -E --color 'vmx|svm' /proc/cpuinfo`.

```bash
# Update system and install dependencies
sudo apt update && sudo apt upgrade -y
sudo apt install -y curl wget apt-transport-https ca-certificates gnupg lsb-release

# Install Docker as container runtime
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
echo "deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt update && sudo apt install -y docker-ce docker-ce-cli containerd.io
sudo usermod -aG docker $USER && newgrp docker
sudo systemctl enable docker && sudo systemctl start docker
```

### Installing specific software versions

**Minikube installation** uses binary download for consistent deployment:

```bash
curl -LO https://github.com/kubernetes/minikube/releases/latest/download/minikube-linux-amd64
sudo install minikube-linux-amd64 /usr/local/bin/minikube && rm minikube-linux-amd64
```

**kubectl v1.30 installation** requires version-specific procedures:

```bash
# Download specific version
curl -LO "https://dl.k8s.io/release/v1.30.0/bin/linux/amd64/kubectl"
chmod +x kubectl && sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# Alternative: Using Kubernetes repository
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.30/deb/Release.key | sudo gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.30/deb/ /' | sudo tee /etc/apt/sources.list.d/kubernetes.list
sudo apt update && sudo apt install -y kubectl
```

**Go v1.21 installation** for application development:

```bash
# Remove existing Go installation
sudo rm -rf /usr/local/go

# Install Go 1.21
wget https://golang.org/dl/go1.21.12.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.12.linux-amd64.tar.gz && rm go1.21.12.linux-amd64.tar.gz

# Configure environment
echo 'export GOROOT=/usr/local/go' >> ~/.profile
echo 'export GOPATH=$HOME/go' >> ~/.profile
echo 'export PATH=$GOPATH/bin:$GOROOT/bin:$PATH' >> ~/.profile
source ~/.profile
```

## Advanced minikube configuration and cluster management

### Cluster initialization with optimal settings

**Production-like configuration** balances local resources with realistic testing scenarios:

```bash
# Start with recommended CKA settings
minikube start \
  --driver=docker \
  --kubernetes-version=v1.30.0 \
  --memory=4096 \
  --cpus=2 \
  --disk-size=20g

# Enable essential add-ons
minikube addons enable dashboard metrics-server ingress storage-provisioner
```

**Persistent configuration** prevents repeated setup:

```bash
minikube config set driver docker
minikube config set memory 4096
minikube config set cpus 2
minikube config view  # Verify settings
```

### Troubleshooting Ubuntu 18.04 specific issues

**Docker driver privileges** commonly cause startup failures:

```bash
# Ensure user is properly configured for Docker
sudo usermod -aG docker $USER && newgrp docker
docker system prune -f && minikube delete && minikube start --driver=docker
```

**Memory and resource constraints** require monitoring:

```bash
# Check resource allocation
free -h && df -h
minikube start --memory=2048 --cpus=2  # Adjust if needed
```

## kubectl mastery for CKA exam efficiency

### Essential command patterns and aliases

**Speed optimization** is crucial for CKA success. These aliases are pre-configured in the exam environment:

```bash
alias k=kubectl
alias kg='kubectl get'
alias kd='kubectl delete' 
alias kc='kubectl create'
alias ka='kubectl apply'
```

**Imperative commands** provide faster resource creation than YAML manifests:

```bash
# Rapid deployment creation
kubectl create deployment nginx --image=nginx:1.20
kubectl expose deployment nginx --type=NodePort --port=80
kubectl scale deployment nginx --replicas=3

# Generate YAML templates
kubectl create deployment app --image=nginx --dry-run=client -o yaml > deployment.yaml
kubectl expose deployment app --port=80 --dry-run=client -o yaml > service.yaml
```

**Resource inspection and debugging** patterns:

```bash
# Comprehensive resource viewing
kubectl get pods -o wide --show-labels --sort-by='.metadata.creationTimestamp'
kubectl get events --sort-by='.lastTimestamp'
kubectl describe pod <pod-name>
kubectl logs -f deployment/app

# JSONPath for specific data extraction
kubectl get pods -o jsonpath='{.items[*].metadata.name}'
kubectl get service nginx -o jsonpath='{.spec.ports[0].nodePort}'
```

### Context management and namespace operations

```bash
# Verify minikube context
kubectl config current-context  # Should display "minikube"
kubectl config use-context minikube

# Namespace management
kubectl create namespace development
kubectl config set-context --current --namespace=development
```

## Helm integration and advanced package management

### Installation and repository management

```bash
# Install Helm 3
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Configure common repositories
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo add hashicorp https://helm.releases.hashicorp.com
helm repo update
```

**Practical deployment patterns** for development environments:

```bash
# Deploy PostgreSQL with custom configuration
helm install pg-dev bitnami/postgresql \
  --set auth.postgresPassword=devpassword \
  --set persistence.size=1Gi \
  --set resources.requests.memory=256Mi

# Multi-environment deployments
helm install app-dev ./mychart --set environment=development --set replicas=1
helm install app-prod ./mychart --set environment=production --set replicas=3 --namespace prod --create-namespace
```

## Understanding minikube networking architecture

### Service types and external access patterns

**NodePort services** provide the primary method for external access in minikube:

```bash
# Create and expose NodePort service
kubectl create deployment webapp --image=nginx
kubectl expose deployment webapp --type=NodePort --port=80

# Access via minikube IP
minikube ip  # Get cluster IP (e.g., 192.168.49.2)
kubectl get svc webapp  # Get NodePort assignment
curl $(minikube ip):$(kubectl get svc webapp -o jsonpath='{.spec.ports[0].nodePort}')

# Shortcut for service access
minikube service webapp --url  # Returns direct URL
minikube service webapp        # Opens in browser
```

**LoadBalancer simulation** requires minikube tunnel:

```bash
# Create LoadBalancer service
kubectl expose deployment webapp --type=LoadBalancer --port=80

# Start tunnel (separate terminal required)
minikube tunnel  # Requires sudo password, creates routes

# Service now gets external IP
kubectl get svc webapp  # Shows actual external IP
curl <EXTERNAL-IP>:80
```

### Port forwarding mechanics and best practices

**Port forwarding syntax** supports multiple target types:

```bash
# Forward to different resource types
kubectl port-forward pod/webapp-pod 8080:80
kubectl port-forward deployment/webapp 8080:80  # Auto-selects pod
kubectl port-forward service/webapp 8080:80

# Advanced forwarding options
kubectl port-forward service/webapp 8080:80 --address 0.0.0.0  # Listen on all interfaces
kubectl port-forward pod/webapp :80  # Random local port assignment
kubectl port-forward service/webapp 8080:80 8443:443  # Multiple ports
```

**Database access patterns** commonly used in development:

```bash
# PostgreSQL access
kubectl port-forward service/postgres 5432:5432
# Connect: psql -h localhost -p 5432 -U postgres

# Background forwarding with process management
kubectl port-forward service/webapp 8080:80 &
kill %1  # Terminate background port forward
```

## Key differences between minikube and Azure Kubernetes Service

### Architectural and operational distinctions

**Single-node limitations** in minikube prevent testing multi-node scenarios like pod anti-affinity, node failures, and cluster-wide networking patterns. **AKS provides managed multi-node clusters** with separated control plane and worker nodes across availability zones.

**Networking models** differ fundamentally:
- **Minikube**: Host-only networking requiring tunneling via `minikube tunnel` or NodePort services
- **AKS**: Native Azure Virtual Network integration with real load balancers and ingress controllers

**Storage and persistence** capabilities:
- **Minikube**: Local hostPath volumes with no persistence across cluster restarts
- **AKS**: Azure Managed Disks, Azure Files, and Container Storage with backup and replication

### Migration considerations for production deployment

**Configuration adaptations** required when moving from minikube to AKS:

```yaml
# Minikube hostPath volume
volumes:
- name: data
  hostPath:
    path: /mnt/data

# AKS Azure Disk equivalent
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: app-data
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: managed-csi-premium
  resources:
    requests:
      storage: 100Gi
```

**Service exposure** changes significantly:

```yaml
# Minikube NodePort service
spec:
  type: NodePort
  ports:
  - port: 80
    nodePort: 30080

# AKS LoadBalancer with Azure integration
spec:
  type: LoadBalancer
  annotations:
    service.beta.kubernetes.io/azure-load-balancer-internal: "false"
  ports:
  - port: 80
    targetPort: 8080
```

## Three practical deployment examples

### Example 1: Go application deployment with port forwarding

**Go HTTP server source code** (main.go):

```go
package main

import (
    "fmt"
    "net/http"
    "log"
    "os"
)

func main() {
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }
    
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "Hello from Go %s HTTP Server in Kubernetes!\nHost: %s\n", 
                   "1.21", r.Host)
    })
    
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, "OK")
    })
    
    fmt.Printf("Starting server on port %s\n", port)
    log.Fatal(http.ListenAndServe(":"+port, nil))
}
```

**Multi-stage Dockerfile** for optimal image size:

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Production stage
FROM scratch
COPY --from=builder /app/main .
EXPOSE 8080
ENTRYPOINT ["./main"]
```

**Complete Kubernetes manifest** (go-app.yaml):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: go-app
  labels:
    app: go-app
spec:
  replicas: 2
  selector:
    matchLabels:
      app: go-app
  template:
    metadata:
      labels:
        app: go-app
    spec:
      containers:
      - name: go-app
        image: go-app:latest
        imagePullPolicy: Never  # Use local minikube image
        ports:
        - containerPort: 8080
        resources:
          requests:
            memory: "64Mi"
            cpu: "250m"
          limits:
            memory: "128Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5

---
apiVersion: v1
kind: Service
metadata:
  name: go-app-service
spec:
  selector:
    app: go-app
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP

---
apiVersion: v1
kind: Service
metadata:
  name: go-app-nodeport
spec:
  selector:
    app: go-app
  type: NodePort
  ports:
  - port: 80
    targetPort: 8080
    nodePort: 30080
```

**Deployment workflow**:

```bash
# Build in minikube Docker environment
eval $(minikube docker-env)
docker build -t go-app:latest .

# Deploy to cluster
kubectl apply -f go-app.yaml

# Verify deployment
kubectl get pods -l app=go-app
kubectl rollout status deployment/go-app

# Access via port forwarding
kubectl port-forward service/go-app-service 8080:80
curl http://localhost:8080

# Access via NodePort
minikube service go-app-nodeport --url
```

### Example 2: Helm deployment of the same Go application

**Custom Helm chart structure**:

```bash
# Create Helm chart
helm create go-app-chart
cd go-app-chart
```

**values.yaml** configuration:

```yaml
replicaCount: 2

image:
  repository: go-app
  pullPolicy: Never  # Use local minikube image
  tag: "latest"

service:
  type: NodePort
  port: 80
  targetPort: 8080
  nodePort: 30080

resources:
  requests:
    memory: "64Mi"
    cpu: "250m"
  limits:
    memory: "128Mi"
    cpu: "500m"

healthCheck:
  enabled: true
  path: "/health"
  port: 8080

ingress:
  enabled: false

nodeSelector: {}
tolerations: []
affinity: {}
```

**templates/deployment.yaml** with health checks:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "go-app-chart.fullname" . }}
  labels:
    {{- include "go-app-chart.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "go-app-chart.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "go-app-chart.selectorLabels" . | nindent 8 }}
    spec:
      containers:
        - name: {{ .Chart.Name }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - name: http
              containerPort: {{ .Values.service.targetPort }}
              protocol: TCP
          {{- if .Values.healthCheck.enabled }}
          livenessProbe:
            httpGet:
              path: {{ .Values.healthCheck.path }}
              port: {{ .Values.healthCheck.port }}
            initialDelaySeconds: 30
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: {{ .Values.healthCheck.path }}
              port: {{ .Values.healthCheck.port }}
            initialDelaySeconds: 5
            periodSeconds: 5
          {{- end }}
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
```

**Helm deployment commands**:

```bash
# Install chart
helm install go-app ./go-app-chart

# Verify installation
helm list
kubectl get pods -l app.kubernetes.io/name=go-app-chart

# Upgrade with new values
helm upgrade go-app ./go-app-chart --set replicaCount=3

# Access application
minikube service go-app-go-app-chart --url
```

### Example 3: Grafana OSS v12 minimal deployment via kubectl

**Complete Grafana manifest** (grafana.yaml) with minimal resource footprint:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: monitoring

---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: grafana-pvc
  namespace: monitoring
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi  # Minimal storage for development

---
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-config
  namespace: monitoring
data:
  grafana.ini: |
    [server]
    root_url = http://localhost:3000
    
    [security]
    admin_user = admin
    admin_password = admin123
    
    [users]
    allow_sign_up = false
    
    [auth.anonymous]
    enabled = false
    
    [log]
    mode = console
    level = info

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grafana
  namespace: monitoring
  labels:
    app: grafana
spec:
  replicas: 1
  selector:
    matchLabels:
      app: grafana
  template:
    metadata:
      labels:
        app: grafana
    spec:
      securityContext:
        fsGroup: 472
        supplementalGroups: [0]
      containers:
      - name: grafana
        image: grafana/grafana:12.0.0
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 3000
          name: http-grafana
          protocol: TCP
        readinessProbe:
          httpGet:
            path: /robots.txt
            port: 3000
          initialDelaySeconds: 10
          periodSeconds: 30
        livenessProbe:
          tcpSocket:
            port: 3000
          initialDelaySeconds: 30
          periodSeconds: 10
        resources:
          requests:
            cpu: 100m      # Minimal CPU request
            memory: 128Mi  # Minimal memory request
          limits:
            cpu: 200m      # Conservative CPU limit
            memory: 256Mi  # Conservative memory limit
        volumeMounts:
        - mountPath: /var/lib/grafana
          name: grafana-storage
        - mountPath: /etc/grafana
          name: grafana-config
      volumes:
      - name: grafana-storage
        persistentVolumeClaim:
          claimName: grafana-pvc
      - name: grafana-config
        configMap:
          name: grafana-config

---
apiVersion: v1
kind: Service
metadata:
  name: grafana-service
  namespace: monitoring
spec:
  ports:
  - port: 3000
    protocol: TCP
    targetPort: http-grafana
  selector:
    app: grafana
  type: ClusterIP

---
apiVersion: v1
kind: Service
metadata:
  name: grafana-nodeport
  namespace: monitoring
spec:
  type: NodePort
  selector:
    app: grafana
  ports:
  - port: 3000
    targetPort: 3000
    nodePort: 30300
```

**Deployment and access workflow**:

```bash
# Deploy Grafana
kubectl apply -f grafana.yaml

# Verify deployment
kubectl get pods --namespace=monitoring
kubectl rollout status deployment/grafana --namespace=monitoring

# Access via port forwarding (recommended for security)
kubectl port-forward service/grafana-service --namespace=monitoring 3000:3000

# Access via NodePort (alternative)
minikube service grafana-nodeport --namespace=monitoring

# Login credentials: admin / admin123
# Access at http://localhost:3000
```

## Conclusion

This comprehensive guide provides production-ready patterns for minikube development workflows while highlighting critical differences from Azure Kubernetes Service. **The key insight for CKAs**: minikube excels as a local development platform but requires architectural adaptations for cloud production deployment, particularly around networking, storage, and service exposure patterns.

**Practical takeaways** include using port-forwarding for secure development access, NodePort services for team testing, and understanding that storage and networking patterns must be redesigned when transitioning to managed Kubernetes services. The provided examples demonstrate complete deployment workflows from Go source code through Kubernetes manifests, enabling efficient local development that translates well to production environments.
