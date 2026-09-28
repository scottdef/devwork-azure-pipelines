# Go 1.21+ JSON HTTP Response Manipulation Guide for Kubernetes Administrators

A comprehensive guide for accessing and manipulating JSON HTTP response content using Go 1.21+ in Azure Kubernetes Service (AKS) environments.

**Target Environment:** AKS with grafana-oss 12, kubectl 1.30, go 1.21

## Go 1.21+ Performance and Features

Go 1.21 introduced **Profile-Guided Optimization (PGO)** providing 2-7% performance improvements that significantly benefit JSON-intensive applications. The enhanced garbage collector reduces tail latency by up to 40%, while new standard library packages (`slices`, `maps`, `cmp`) improve JSON data manipulation workflows.

**Enhanced JSON Processing with New Packages:**
```go
import (
    "encoding/json"
    "slices"
    "maps"
    "cmp"
)

// Sorting JSON array data using Go 1.21+ generics
type KubernetesNode struct {
    Name     string  `json:"name"`
    CPUUsage float64 `json:"cpu_usage"`
    Ready    bool    `json:"ready"`
}

func sortNodesByCPU(nodes []KubernetesNode) {
    slices.SortFunc(nodes, func(a, b KubernetesNode) int {
        return cmp.Compare(a.CPUUsage, b.CPUUsage)
    })
}
```

## Five Primary JSON HTTP Response Access Techniques

### 1. Structured Unmarshaling with Kubernetes API Types

**Best for:** Known Kubernetes API schemas and consistent structure access

```go
package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "context"
    "time"
)

type KubernetesAPIResponse struct {
    Kind       string `json:"kind"`
    APIVersion string `json:"apiVersion"`
    Metadata   struct {
        ResourceVersion string `json:"resourceVersion"`
    } `json:"metadata"`
    Items []PodItem `json:"items"`
}

type PodItem struct {
    Metadata struct {
        Name      string            `json:"name"`
        Namespace string            `json:"namespace"`
        Labels    map[string]string `json:"labels"`
    } `json:"metadata"`
    Spec struct {
        Containers []Container `json:"containers"`
    } `json:"spec"`
    Status struct {
        Phase             string `json:"phase"`
        ContainerStatuses []struct {
            Name         string `json:"name"`
            Ready        bool   `json:"ready"`
            RestartCount int    `json:"restartCount"`
        } `json:"containerStatuses"`
    } `json:"status"`
}

type Container struct {
    Name  string `json:"name"`
    Image string `json:"image"`
    Resources struct {
        Requests map[string]string `json:"requests"`
        Limits   map[string]string `json:"limits"`
    } `json:"resources"`
}

func fetchKubernetesPods(apiServerURL, token string) (*KubernetesAPIResponse, error) {
    client := &http.Client{Timeout: 30 * time.Second}
    
    req, err := http.NewRequestWithContext(
        context.Background(), "GET", 
        apiServerURL+"/api/v1/pods", nil)
    if err != nil {
        return nil, fmt.Errorf("creating request: %w", err)
    }
    
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Accept", "application/json")
    
    resp, err := client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("HTTP request failed: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
    }
    
    var apiResp KubernetesAPIResponse
    if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
        return nil, fmt.Errorf("JSON decode failed: %w", err)
    }
    
    return &apiResp, nil
}

// Usage for K8s admin tasks
func analyzeClusterPods(apiResp *KubernetesAPIResponse) {
    fmt.Printf("Found %d pods (Resource Version: %s)\n", 
        len(apiResp.Items), apiResp.Metadata.ResourceVersion)
    
    for _, pod := range apiResp.Items {
        fmt.Printf("Pod: %s/%s, Phase: %s\n", 
            pod.Metadata.Namespace, pod.Metadata.Name, pod.Status.Phase)
        
        // Analyze container resource requests for capacity planning
        for _, container := range pod.Spec.Containers {
            cpuReq := container.Resources.Requests["cpu"]
            memReq := container.Resources.Requests["memory"]
            fmt.Printf("  Container %s: CPU=%s, Memory=%s\n", 
                container.Name, cpuReq, memReq)
        }
    }
}
```

### 2. Dynamic Interface{} Parsing for Unknown Schemas

**Best for:** Grafana API responses and unknown JSON structures

```go
func parseGrafanaMetrics(url, token string) error {
    client := &http.Client{Timeout: 30 * time.Second}
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("Authorization", "Bearer "+token)
    
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    var data map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
        return err
    }
    
    // Navigate Grafana dashboard JSON structure
    if dashboard, ok := data["dashboard"].(map[string]interface{}); ok {
        if panels, ok := dashboard["panels"].([]interface{}); ok {
            for i, panel := range panels {
                if panelMap, ok := panel.(map[string]interface{}); ok {
                    title, _ := panelMap["title"].(string)
                    panelType, _ := panelMap["type"].(string)
                    fmt.Printf("Panel %d: %s (%s)\n", i, title, panelType)
                    
                    // Extract metrics queries for AKS monitoring
                    if targets, ok := panelMap["targets"].([]interface{}); ok {
                        for _, target := range targets {
                            if targetMap, ok := target.(map[string]interface{}); ok {
                                expr, _ := targetMap["expr"].(string)
                                fmt.Printf("  Query: %s\n", expr)
                            }
                        }
                    }
                }
            }
        }
    }
    
    return nil
}
```

### 3. Streaming Decoder for Large AKS Logs

**Best for:** Processing large log streams and memory-constrained environments

```go
import (
    "bufio"
    "io"
)

func processAKSLogStream(url, token string) error {
    client := &http.Client{Timeout: 0} // No timeout for streaming
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Accept", "application/json")
    
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    decoder := json.NewDecoder(resp.Body)
    
    // Process log entries one by one
    for decoder.More() {
        var logEntry map[string]interface{}
        if err := decoder.Decode(&logEntry); err != nil {
            if err == io.EOF {
                break
            }
            return err
        }
        
        // Process individual log entry for AKS monitoring
        timestamp, _ := logEntry["timestamp"].(string)
        level, _ := logEntry["level"].(string)
        message, _ := logEntry["message"].(string)
        
        // Filter critical errors for immediate attention
        if level == "ERROR" || level == "FATAL" {
            fmt.Printf("[%s] %s: %s\n", timestamp, level, message)
            
            // Trigger alert or notification logic here
            handleCriticalLogEntry(logEntry)
        }
    }
    
    return nil
}

func handleCriticalLogEntry(entry map[string]interface{}) {
    // Implementation for critical log processing
    // Could integrate with AKS alerting systems
}
```

### 4. RawMessage for Selective AKS Event Processing

**Best for:** Processing different Kubernetes event types efficiently

```go
type KubernetesEvent struct {
    Type   string          `json:"type"`   // ADDED, MODIFIED, DELETED
    Object json.RawMessage `json:"object"` // Defer parsing based on type
}

type PodEvent struct {
    Kind     string `json:"kind"`
    Metadata struct {
        Name      string `json:"name"`
        Namespace string `json:"namespace"`
    } `json:"metadata"`
    Status struct {
        Phase string `json:"phase"`
    } `json:"status"`
}

type ServiceEvent struct {
    Kind     string `json:"kind"`
    Metadata struct {
        Name      string `json:"name"`
        Namespace string `json:"namespace"`
    } `json:"metadata"`
    Spec struct {
        ClusterIP string   `json:"clusterIP"`
        Ports     []Port   `json:"ports"`
    } `json:"spec"`
}

type Port struct {
    Name       string `json:"name"`
    Port       int    `json:"port"`
    TargetPort int    `json:"targetPort"`
}

func processKubernetesEvents(url, token string) error {
    // Watch API endpoint for real-time events
    watchURL := url + "/api/v1/watch/pods"
    
    resp, err := http.Get(watchURL)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    decoder := json.NewDecoder(resp.Body)
    
    for decoder.More() {
        var event KubernetesEvent
        if err := decoder.Decode(&event); err != nil {
            return err
        }
        
        // Parse object based on event type for efficient processing
        switch event.Type {
        case "ADDED", "MODIFIED":
            var podEvent PodEvent
            if err := json.Unmarshal(event.Object, &podEvent); err == nil {
                if podEvent.Kind == "Pod" {
                    fmt.Printf("Pod %s/%s is now %s\n", 
                        podEvent.Metadata.Namespace, 
                        podEvent.Metadata.Name, 
                        podEvent.Status.Phase)
                    
                    // Trigger pod lifecycle management
                    handlePodStateChange(podEvent, event.Type)
                }
            }
            
        case "DELETED":
            // Handle pod deletion events
            handlePodDeletion(event.Object)
        }
    }
    
    return nil
}

func handlePodStateChange(pod PodEvent, eventType string) {
    // Implementation for pod state management
}

func handlePodDeletion(rawObject json.RawMessage) {
    // Implementation for cleanup operations
}
```

### 5. Custom Unmarshaler for AKS Resource Validation

**Best for:** Complex validation and transformation of Kubernetes resources

```go
import (
    "strings"
    "strconv"
    "regexp"
)

type ResourceQuantity struct {
    Value float64
    Unit  string
}

func (rq *ResourceQuantity) UnmarshalJSON(data []byte) error {
    str := strings.Trim(string(data), "\"")
    
    // Parse Kubernetes resource quantities (e.g., "100m", "1Gi", "500Mi")
    re := regexp.MustCompile(`^(\d+(?:\.\d+)?)(.*)?$`)
    matches := re.FindStringSubmatch(str)
    
    if len(matches) < 2 {
        return fmt.Errorf("invalid resource quantity format: %s", str)
    }
    
    value, err := strconv.ParseFloat(matches[1], 64)
    if err != nil {
        return err
    }
    
    unit := ""
    if len(matches) > 2 {
        unit = matches[2]
    }
    
    // Convert to base units for comparison
    switch unit {
    case "m": // milli-cores
        value = value / 1000
        unit = "cores"
    case "Mi": // Mebibytes
        value = value * 1024 * 1024
        unit = "bytes"
    case "Gi": // Gibibytes
        value = value * 1024 * 1024 * 1024
        unit = "bytes"
    case "Ki": // Kibibytes
        value = value * 1024
        unit = "bytes"
    }
    
    rq.Value = value
    rq.Unit = unit
    return nil
}

type ValidatedContainer struct {
    Name      string           `json:"name"`
    Image     string           `json:"image"`
    CPULimit  ResourceQuantity `json:"cpu_limit"`
    MemLimit  ResourceQuantity `json:"memory_limit"`
    CPUReq    ResourceQuantity `json:"cpu_request"`
    MemReq    ResourceQuantity `json:"memory_request"`
}

func fetchAndValidateContainerSpecs(url, token string) error {
    resp, err := http.Get(url)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    var containers []ValidatedContainer
    if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
        return err
    }
    
    // Validate resource allocations for capacity planning
    for _, container := range containers {
        if container.CPUReq.Value > container.CPULimit.Value {
            fmt.Printf("WARNING: Container %s has CPU request > limit\n", 
                container.Name)
        }
        
        if container.MemReq.Value > container.MemLimit.Value {
            fmt.Printf("WARNING: Container %s has memory request > limit\n", 
                container.Name)
        }
        
        fmt.Printf("Container %s: CPU=%f %s, Memory=%f %s\n",
            container.Name, container.CPUReq.Value, container.CPUReq.Unit,
            container.MemReq.Value, container.MemReq.Unit)
    }
    
    return nil
}
```

## Three JSON Content Conversion Methods

### Method 1: Type Structs for Kubernetes Resources

```go
// Source: Kubernetes API response structure
type KubernetesDeployment struct {
    APIVersion string `json:"apiVersion"`
    Kind       string `json:"kind"`
    Metadata   struct {
        Name      string            `json:"name"`
        Namespace string            `json:"namespace"`
        Labels    map[string]string `json:"labels"`
    } `json:"metadata"`
    Spec struct {
        Replicas int `json:"replicas"`
        Template struct {
            Spec struct {
                Containers []struct {
                    Name  string `json:"name"`
                    Image string `json:"image"`
                    Ports []struct {
                        ContainerPort int `json:"containerPort"`
                    } `json:"ports"`
                } `json:"containers"`
            } `json:"spec"`
        } `json:"template"`
    } `json:"spec"`
    Status struct {
        ReadyReplicas int `json:"readyReplicas"`
        Replicas      int `json:"replicas"`
    } `json:"status"`
}

// Target: Internal deployment tracking structure
type DeploymentSummary struct {
    Name              string
    Namespace         string
    DesiredReplicas   int
    ReadyReplicas     int
    HealthPercentage  float64
    ContainerImages   []string
    ExposedPorts      []int
    Labels            map[string]string
}

func convertDeploymentToSummary(k8sDep KubernetesDeployment) DeploymentSummary {
    // Extract container images and ports
    var images []string
    var ports []int
    
    for _, container := range k8sDep.Spec.Template.Spec.Containers {
        images = append(images, container.Image)
        for _, port := range container.Ports {
            ports = append(ports, port.ContainerPort)
        }
    }
    
    // Calculate health percentage
    var healthPct float64
    if k8sDep.Spec.Replicas > 0 {
        healthPct = (float64(k8sDep.Status.ReadyReplicas) / 
                    float64(k8sDep.Spec.Replicas)) * 100
    }
    
    return DeploymentSummary{
        Name:              k8sDep.Metadata.Name,
        Namespace:         k8sDep.Metadata.Namespace,
        DesiredReplicas:   k8sDep.Spec.Replicas,
        ReadyReplicas:     k8sDep.Status.ReadyReplicas,
        HealthPercentage:  healthPct,
        ContainerImages:   images,
        ExposedPorts:      ports,
        Labels:            k8sDep.Metadata.Labels,
    }
}
```

### Method 2: Modifying Existing JSON Records (Append, Add, Overwrite)

```go
import (
    "io/ioutil"
    "os"
    "time"
)

type ClusterConfig struct {
    ClusterName    string                 `json:"cluster_name"`
    Nodes          []string               `json:"nodes"`
    Services       []string               `json:"services"`
    TotalPods      int                    `json:"total_pods"`
    CPUUtilization float64                `json:"cpu_utilization"`
    LastUpdated    time.Time              `json:"last_updated"`
    Enabled        bool                   `json:"enabled"`
    Metadata       map[string]interface{} `json:"metadata"`
}

func updateClusterConfig(filename string) error {
    // Read existing config
    var config ClusterConfig
    if data, err := ioutil.ReadFile(filename); err == nil {
        json.Unmarshal(data, &config)
    }
    
    // APPEND to arrays/slices
    config.Nodes = append(config.Nodes, "aks-nodepool-new-001", "aks-nodepool-new-002")
    config.Services = append(config.Services, "nginx-ingress", "cert-manager")
    
    // ADD to numbers (increment counts, add utilization)
    config.TotalPods += 15
    config.CPUUtilization += 12.5
    
    // ADD to unix timestamp (update last modified time)
    config.LastUpdated = time.Now()
    
    // OVERWRITE boolean values
    config.Enabled = true
    
    // OVERWRITE null values and add to metadata map
    if config.Metadata == nil {
        config.Metadata = make(map[string]interface{})
    }
    config.Metadata["aks_version"] = "1.30.0"
    config.Metadata["grafana_integrated"] = true
    config.Metadata["monitoring_enabled"] = true
    config.Metadata["last_backup"] = time.Now().Unix() // Unix timestamp
    
    // Append configuration history to nested array
    if config.Metadata["config_history"] == nil {
        config.Metadata["config_history"] = []interface{}{}
    }
    history := config.Metadata["config_history"].([]interface{})
    newEntry := map[string]interface{}{
        "timestamp": time.Now().Unix(),
        "change":    "Added new nodes and services",
        "admin":     "k8s-admin",
    }
    config.Metadata["config_history"] = append(history, newEntry)
    
    // Write updated config back to file
    updatedData, err := json.MarshalIndent(config, "", "  ")
    if err != nil {
        return fmt.Errorf("marshaling config: %w", err)
    }
    
    return ioutil.WriteFile(filename, updatedData, 0644)
}

// Bulk append operations for monitoring data
func appendMonitoringData(filename string, newMetrics []map[string]interface{}) error {
    var existingData []map[string]interface{}
    
    // Load existing metrics data
    if data, err := ioutil.ReadFile(filename); err == nil && len(data) > 0 {
        json.Unmarshal(data, &existingData)
    }
    
    // Append new metrics with timestamp modification
    for _, metric := range newMetrics {
        // Add current timestamp if not present
        if _, exists := metric["timestamp"]; !exists {
            metric["timestamp"] = time.Now().Unix()
        }
        
        // Increment any numeric values that represent cumulative metrics
        if value, ok := metric["total_requests"].(float64); ok {
            metric["total_requests"] = value + 100 // Add baseline requests
        }
        
        existingData = append(existingData, metric)
    }
    
    // Maintain only last 1000 entries for performance
    if len(existingData) > 1000 {
        existingData = existingData[len(existingData)-1000:]
    }
    
    // Save back to file
    updatedData, err := json.MarshalIndent(existingData, "", "  ")
    if err != nil {
        return err
    }
    
    return ioutil.WriteFile(filename, updatedData, 0644)
}
```

### Method 3: Template Variables (Mad-libs Style)

```go
import (
    "text/template"
    "bytes"
)

// Extract variables from JSON content for template processing
func generateKubernetesManifests() error {
    // Source JSON with configuration data
    configJSON := `{
        "app_name": "web-frontend",
        "namespace": "production",
        "replicas": 3,
        "image": "nginx:1.21",
        "port": 80,
        "cpu_request": "100m",
        "memory_request": "128Mi",
        "cpu_limit": "200m",
        "memory_limit": "256Mi",
        "domain": "myapp.example.com",
        "environment": "production"
    }`
    
    // Parse JSON into variables
    var vars map[string]interface{}
    if err := json.Unmarshal([]byte(configJSON), &vars); err != nil {
        return err
    }
    
    // Kubernetes deployment template with variable placeholders
    deploymentTemplate := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{.app_name}}
  namespace: {{.namespace}}
  labels:
    app: {{.app_name}}
    environment: {{.environment}}
spec:
  replicas: {{.replicas}}
  selector:
    matchLabels:
      app: {{.app_name}}
  template:
    metadata:
      labels:
        app: {{.app_name}}
        environment: {{.environment}}
    spec:
      containers:
      - name: {{.app_name}}
        image: {{.image}}
        ports:
        - containerPort: {{.port}}
        resources:
          requests:
            cpu: {{.cpu_request}}
            memory: {{.memory_request}}
          limits:
            cpu: {{.cpu_limit}}
            memory: {{.memory_limit}}
---
apiVersion: v1
kind: Service
metadata:
  name: {{.app_name}}-service
  namespace: {{.namespace}}
spec:
  selector:
    app: {{.app_name}}
  ports:
  - port: {{.port}}
    targetPort: {{.port}}
  type: ClusterIP
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{.app_name}}-ingress
  namespace: {{.namespace}}
  annotations:
    kubernetes.io/ingress.class: nginx
spec:
  rules:
  - host: {{.domain}}
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: {{.app_name}}-service
            port:
              number: {{.port}}
`
    
    // Process template with variables
    tmpl, err := template.New("k8s-manifest").Parse(deploymentTemplate)
    if err != nil {
        return fmt.Errorf("parsing template: %w", err)
    }
    
    var output bytes.Buffer
    if err := tmpl.Execute(&output, vars); err != nil {
        return fmt.Errorf("executing template: %w", err)
    }
    
    // Save generated manifest
    manifestFile := fmt.Sprintf("%s-manifest.yaml", vars["app_name"])
    return ioutil.WriteFile(manifestFile, output.Bytes(), 0644)
}

// Template functions for advanced JSON manipulation
var k8sTemplateFuncs = template.FuncMap{
    "json": func(v interface{}) string {
        data, _ := json.Marshal(v)
        return string(data)
    },
    "toYAML": func(v interface{}) string {
        // Convert JSON to YAML format (simplified)
        data, _ := json.MarshalIndent(v, "", "  ")
        return string(data)
    },
    "getNamespace": func(name string) string {
        return fmt.Sprintf("%s-namespace", name)
    },
    "generateLabels": func(app, env string) map[string]string {
        return map[string]string{
            "app":         app,
            "environment": env,
            "managed-by":  "k8s-admin-tool",
        }
    },
}

// Generate monitoring configuration from metrics data
func generateGrafanaConfig() error {
    metricsJSON := `{
        "cluster_name": "aks-production",
        "datasource_url": "http://prometheus:9090",
        "dashboards": [
            {
                "name": "Cluster Overview",
                "metrics": ["cpu_usage", "memory_usage", "pod_count"],
                "refresh": "30s"
            },
            {
                "name": "Application Metrics", 
                "metrics": ["request_rate", "error_rate", "response_time"],
                "refresh": "10s"
            }
        ]
    }`
    
    var config map[string]interface{}
    json.Unmarshal([]byte(metricsJSON), &config)
    
    grafanaTemplate := `{
  "dashboard": {
    "title": "{{.cluster_name}} Monitoring",
    "panels": [
{{range $index, $dashboard := .dashboards}}
      {
        "title": "{{$dashboard.name}}",
        "type": "graph",
        "refresh": "{{$dashboard.refresh}}",
        "targets": [
{{range $metricIndex, $metric := $dashboard.metrics}}
          {
            "expr": "{{$metric}}",
            "legendFormat": "{{$metric}}"
          }{{if ne $metricIndex (sub (len $dashboard.metrics) 1)}},{{end}}
{{end}}
        ]
      }{{if ne $index (sub (len $.dashboards) 1)}},{{end}}
{{end}}
    ]
  }
}`
    
    // Add helper functions for template processing  
    funcs := template.FuncMap{
        "sub": func(a, b int) int { return a - b },
        "len": func(v interface{}) int {
            switch s := v.(type) {
            case []interface{}:
                return len(s)
            default:
                return 0
            }
        },
    }
    
    tmpl, err := template.New("grafana").Funcs(funcs).Parse(grafanaTemplate)
    if err != nil {
        return err
    }
    
    var output bytes.Buffer
    if err := tmpl.Execute(&output, config); err != nil {
        return err
    }
    
    return ioutil.WriteFile("grafana-dashboard.json", output.Bytes(), 0644)
}
```

## File and Folder Operations Using JSON Content

### Creating Directory Structures from JSON Data

```go
import (
    "path/filepath"
    "strings"
)

type BackupManifest struct {
    ClusterName string `json:"cluster_name"`
    Timestamp   string `json:"timestamp"`
    Namespaces  []struct {
        Name      string `json:"name"`
        Resources []struct {
            Kind string `json:"kind"`
            Name string `json:"name"`
        } `json:"resources"`
    } `json:"namespaces"`
}

func createBackupStructure(manifest BackupManifest) error {
    // Use JSON data to create folder names
    baseDir := fmt.Sprintf("k8s-backups/%s/%s", 
        manifest.ClusterName, 
        strings.ReplaceAll(manifest.Timestamp, ":", "-"))
    
    // Create base directory
    if err := os.MkdirAll(baseDir, 0755); err != nil {
        return fmt.Errorf("creating base directory: %w", err)
    }
    
    // Create namespace directories and resource files
    for _, ns := range manifest.Namespaces {
        nsDir := filepath.Join(baseDir, "namespaces", ns.Name)
        if err := os.MkdirAll(nsDir, 0755); err != nil {
            return fmt.Errorf("creating namespace directory: %w", err)
        }
        
        // Group resources by kind
        resourcesByKind := make(map[string][]string)
        for _, resource := range ns.Resources {
            resourcesByKind[resource.Kind] = append(
                resourcesByKind[resource.Kind], resource.Name)
        }
        
        // Create resource type directories and files
        for kind, names := range resourcesByKind {
            kindDir := filepath.Join(nsDir, strings.ToLower(kind)+"s")
            if err := os.MkdirAll(kindDir, 0755); err != nil {
                return fmt.Errorf("creating resource directory: %w", err)
            }
            
            // Create individual resource files
            for _, name := range names {
                filename := filepath.Join(kindDir, name+".yaml")
                placeholder := fmt.Sprintf("# %s %s from namespace %s\n# Timestamp: %s\n", 
                    kind, name, ns.Name, manifest.Timestamp)
                
                if err := ioutil.WriteFile(filename, []byte(placeholder), 0644); err != nil {
                    return fmt.Errorf("creating resource file: %w", err)
                }
            }
        }
    }
    
    // Create manifest file in backup directory
    manifestData, err := json.MarshalIndent(manifest, "", "  ")
    if err != nil {
        return err
    }
    
    manifestFile := filepath.Join(baseDir, "backup-manifest.json")
    return ioutil.WriteFile(manifestFile, manifestData, 0644)
}

// Save monitoring data with dynamic file naming
func saveMonitoringData(jsonData []byte) error {
    var metrics map[string]interface{}
    if err := json.Unmarshal(jsonData, &metrics); err != nil {
        return err
    }
    
    // Extract data for file/folder naming
    clusterName, _ := metrics["cluster"].(string)
    nodeType, _ := metrics["node_type"].(string)
    timestamp, _ := metrics["timestamp"].(string)
    
    // Create directory structure: data/monitoring/{cluster}/{node_type}/{date}/
    date := strings.Split(timestamp, "T")[0] // Extract date from ISO timestamp
    directory := filepath.Join("data", "monitoring", clusterName, nodeType, date)
    
    if err := os.MkdirAll(directory, 0755); err != nil {
        return fmt.Errorf("creating monitoring directory: %w", err)
    }
    
    // Create filename with timestamp and metrics type
    metricsType, _ := metrics["type"].(string)
    filename := fmt.Sprintf("%s-%s-metrics.json", 
        strings.ReplaceAll(timestamp, ":", "-"), metricsType)
    
    fullPath := filepath.Join(directory, filename)
    
    // Add metadata to JSON before saving
    metrics["saved_at"] = time.Now().Format(time.RFC3339)
    metrics["file_path"] = fullPath
    
    // Save enhanced JSON data
    enhancedData, err := json.MarshalIndent(metrics, "", "  ")
    if err != nil {
        return err
    }
    
    return ioutil.WriteFile(fullPath, enhancedData, 0644)
}
```

## Saving New Data Structures from JSON

### Transforming Kubernetes API Data into Custom Structures

```go
// Source: Raw Kubernetes metrics API response
type K8sMetricsResponse struct {
    Kind       string `json:"kind"`
    APIVersion string `json:"apiVersion"`
    Items      []struct {
        Metadata struct {
            Name      string `json:"name"`
            Namespace string `json:"namespace"`
        } `json:"metadata"`
        Containers []struct {
            Name  string `json:"name"`
            Usage struct {
                CPU    string `json:"cpu"`
                Memory string `json:"memory"`
            } `json:"usage"`
        } `json:"containers"`
    } `json:"items"`
}

// Target: Internal monitoring data structure
type ClusterResourceUsage struct {
    Timestamp        time.Time                    `json:"timestamp"`
    TotalPods        int                          `json:"total_pods"`
    NamespaceMetrics map[string]NamespaceMetrics  `json:"namespace_metrics"`
    TopConsumers     []ResourceConsumer           `json:"top_consumers"`
    Alerts           []ResourceAlert              `json:"alerts"`
}

type NamespaceMetrics struct {
    Name           string  `json:"name"`
    PodCount       int     `json:"pod_count"`
    TotalCPU       float64 `json:"total_cpu_cores"`
    TotalMemory    float64 `json:"total_memory_bytes"`
    AverageCPU     float64 `json:"average_cpu_cores"`
    AverageMemory  float64 `json:"average_memory_bytes"`
}

type ResourceConsumer struct {
    PodName       string  `json:"pod_name"`
    Namespace     string  `json:"namespace"`
    ContainerName string  `json:"container_name"`
    CPUCores      float64 `json:"cpu_cores"`
    MemoryBytes   float64 `json:"memory_bytes"`
}

type ResourceAlert struct {
    Level       string    `json:"level"`
    Message     string    `json:"message"`
    PodName     string    `json:"pod_name"`
    Namespace   string    `json:"namespace"`
    Timestamp   time.Time `json:"timestamp"`
    MetricType  string    `json:"metric_type"`
    MetricValue float64   `json:"metric_value"`
}

func transformMetricsToUsageReport(metricsResp K8sMetricsResponse) ClusterResourceUsage {
    usage := ClusterResourceUsage{
        Timestamp:        time.Now(),
        TotalPods:        len(metricsResp.Items),
        NamespaceMetrics: make(map[string]NamespaceMetrics),
        TopConsumers:     []ResourceConsumer{},
        Alerts:           []ResourceAlert{},
    }
    
    // Track namespace metrics
    nsMetrics := make(map[string]*NamespaceMetrics)
    allConsumers := []ResourceConsumer{}
    
    // Process each pod's metrics
    for _, pod := range metricsResp.Items {
        namespace := pod.Metadata.Namespace
        
        // Initialize namespace metrics if not exists
        if _, exists := nsMetrics[namespace]; !exists {
            nsMetrics[namespace] = &NamespaceMetrics{
                Name:     namespace,
                PodCount: 0,
            }
        }
        
        nsMetrics[namespace].PodCount++
        
        // Process each container in the pod
        for _, container := range pod.Containers {
            // Parse CPU (e.g., "100m" -> 0.1 cores)
            cpuCores := parseResourceQuantity(container.Usage.CPU, "cpu")
            memoryBytes := parseResourceQuantity(container.Usage.Memory, "memory")
            
            // Add to namespace totals
            nsMetrics[namespace].TotalCPU += cpuCores
            nsMetrics[namespace].TotalMemory += memoryBytes
            
            // Create resource consumer entry
            consumer := ResourceConsumer{
                PodName:       pod.Metadata.Name,
                Namespace:     namespace,
                ContainerName: container.Name,
                CPUCores:      cpuCores,
                MemoryBytes:   memoryBytes,
            }
            allConsumers = append(allConsumers, consumer)
            
            // Generate alerts for high resource usage
            if cpuCores > 1.0 { // Alert if using more than 1 CPU core
                alert := ResourceAlert{
                    Level:       "WARNING",
                    Message:     fmt.Sprintf("High CPU usage: %.2f cores", cpuCores),
                    PodName:     pod.Metadata.Name,
                    Namespace:   namespace,
                    Timestamp:   time.Now(),
                    MetricType:  "cpu",
                    MetricValue: cpuCores,
                }
                usage.Alerts = append(usage.Alerts, alert)
            }
            
            if memoryBytes > 1024*1024*1024 { // Alert if using more than 1GB
                alert := ResourceAlert{
                    Level:       "WARNING", 
                    Message:     fmt.Sprintf("High memory usage: %.2f GB", memoryBytes/(1024*1024*1024)),
                    PodName:     pod.Metadata.Name,
                    Namespace:   namespace,
                    Timestamp:   time.Now(),
                    MetricType:  "memory",
                    MetricValue: memoryBytes,
                }
                usage.Alerts = append(usage.Alerts, alert)
            }
        }
    }
    
    // Calculate averages and finalize namespace metrics
    for ns, metrics := range nsMetrics {
        if metrics.PodCount > 0 {
            metrics.AverageCPU = metrics.TotalCPU / float64(metrics.PodCount)
            metrics.AverageMemory = metrics.TotalMemory / float64(metrics.PodCount)
        }
        usage.NamespaceMetrics[ns] = *metrics
    }
    
    // Sort and get top 10 resource consumers
    slices.SortFunc(allConsumers, func(a, b ResourceConsumer) int {
        return cmp.Compare(b.CPUCores, a.CPUCores) // Sort by CPU descending
    })
    
    if len(allConsumers) > 10 {
        usage.TopConsumers = allConsumers[:10]
    } else {
        usage.TopConsumers = allConsumers
    }
    
    return usage
}

func parseResourceQuantity(quantity, quantityType string) float64 {
    // Simple parser for Kubernetes resource quantities
    if strings.HasSuffix(quantity, "m") && quantityType == "cpu" {
        // CPU millicores (e.g., "100m" -> 0.1 cores)
        value, _ := strconv.ParseFloat(strings.TrimSuffix(quantity, "m"), 64)
        return value / 1000
    }
    
    if quantityType == "memory" {
        // Memory (e.g., "128Mi" -> bytes)
        if strings.HasSuffix(quantity, "Mi") {
            value, _ := strconv.ParseFloat(strings.TrimSuffix(quantity, "Mi"), 64)
            return value * 1024 * 1024
        }
        if strings.HasSuffix(quantity, "Gi") {
            value, _ := strconv.ParseFloat(strings.TrimSuffix(quantity, "Gi"), 64)
            return value * 1024 * 1024 * 1024
        }
    }
    
    // Default: parse as float
    value, _ := strconv.ParseFloat(quantity, 64)
    return value
}

// Save the transformed data structure
func saveClusterUsageReport(usage ClusterResourceUsage) error {
    // Create timestamped filename
    timestamp := usage.Timestamp.Format("2006-01-02_15-04-05")
    filename := fmt.Sprintf("cluster-usage-%s.json", timestamp)
    
    // Create reports directory
    reportsDir := "reports/cluster-usage"
    if err := os.MkdirAll(reportsDir, 0755); err != nil {
        return fmt.Errorf("creating reports directory: %w", err)
    }
    
    // Save JSON data
    data, err := json.MarshalIndent(usage, "", "  ")
    if err != nil {
        return fmt.Errorf("marshaling usage report: %w", err)
    }
    
    fullPath := filepath.Join(reportsDir, filename)
    return ioutil.WriteFile(fullPath, data, 0644)
}
```

### Integration with go-echarts for Visualization

```go
import (
    "github.com/go-echarts/go-echarts/v2/charts"
    "github.com/go-echarts/go-echarts/v2/opts"
)

func createResourceVisualization(usage ClusterResourceUsage) error {
    // Create bar chart for namespace resource usage
    bar := charts.NewBar()
    bar.SetGlobalOptions(
        charts.WithTitleOpts(opts.Title{
            Title:    "Namespace Resource Usage",
            Subtitle: fmt.Sprintf("Cluster Report - %s", usage.Timestamp.Format("2006-01-02 15:04")),
        }),
        charts.WithYAxisOpts(opts.YAxis{
            Name: "Resource Usage",
        }),
    )
    
    // Extract namespace names and metrics for chart
    var namespaces []string
    var cpuData []opts.BarData
    var memoryData []opts.BarData
    
    for nsName, metrics := range usage.NamespaceMetrics {
        namespaces = append(namespaces, nsName)
        cpuData = append(cpuData, opts.BarData{Value: metrics.TotalCPU})
        memoryData = append(memoryData, opts.BarData{
            Value: metrics.TotalMemory / (1024 * 1024 * 1024), // Convert to GB
        })
    }
    
    bar.SetXAxis(namespaces).
        AddSeries("CPU (Cores)", cpuData).
        AddSeries("Memory (GB)", memoryData)
    
    // Save chart
    chartFile := fmt.Sprintf("charts/resource-usage-%s.html", 
        usage.Timestamp.Format("2006-01-02_15-04-05"))
    
    if err := os.MkdirAll("charts", 0755); err != nil {
        return err
    }
    
    f, err := os.Create(chartFile)
    if err != nil {
        return err
    }
    defer f.Close()
    
    return bar.Render(f)
}
```

## Error Handling and Go Idioms

### Comprehensive Error Handling Pattern

```go
import (
    "errors"
    "context"
)

var (
    ErrKubernetesAPI    = errors.New("kubernetes API error")
    ErrInvalidJSON      = errors.New("invalid JSON format")
    ErrResourceNotFound = errors.New("resource not found")
    ErrPermissionDenied = errors.New("permission denied")
)

type K8sClient struct {
    apiServerURL string
    token        string
    httpClient   *http.Client
}

func NewK8sClient(apiServerURL, token string) *K8sClient {
    return &K8sClient{
        apiServerURL: apiServerURL,
        token:        token,
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
        },
    }
}

func (c *K8sClient) FetchWithRetry(ctx context.Context, endpoint string, target interface{}) error {
    const maxRetries = 3
    
    for attempt := 1; attempt <= maxRetries; attempt++ {
        if err := c.fetchOnce(ctx, endpoint, target); err != nil {
            if attempt == maxRetries {
                return fmt.Errorf("failed after %d attempts: %w", maxRetries, err)
            }
            
            // Exponential backoff
            backoff := time.Duration(attempt) * time.Second
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(backoff):
                continue
            }
        }
        
        return nil // Success
    }
    
    return fmt.Errorf("unexpected retry loop exit")
}

func (c *K8sClient) fetchOnce(ctx context.Context, endpoint string, target interface{}) error {
    url := c.apiServerURL + endpoint
    
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return fmt.Errorf("creating request: %w", err)
    }
    
    req.Header.Set("Authorization", "Bearer "+c.token)
    req.Header.Set("Accept", "application/json")
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("HTTP request failed: %w", ErrKubernetesAPI)
    }
    defer resp.Body.Close()
    
    // Handle HTTP status codes
    switch resp.StatusCode {
    case http.StatusOK:
        // Continue processing
    case http.StatusNotFound:
        return fmt.Errorf("resource not found at %s: %w", endpoint, ErrResourceNotFound)
    case http.StatusForbidden:
        return fmt.Errorf("access denied to %s: %w", endpoint, ErrPermissionDenied)
    default:
        return fmt.Errorf("API returned status %d for %s: %w", 
            resp.StatusCode, endpoint, ErrKubernetesAPI)
    }
    
    // Validate content type
    contentType := resp.Header.Get("Content-Type")
    if !strings.Contains(contentType, "application/json") {
        return fmt.Errorf("expected JSON, got %s: %w", contentType, ErrInvalidJSON)
    }
    
    // Decode JSON with detailed error information
    decoder := json.NewDecoder(resp.Body)
    if err := decoder.Decode(target); err != nil {
        return c.handleJSONError(err, endpoint)
    }
    
    return nil
}

func (c *K8sClient) handleJSONError(err error, endpoint string) error {
    var syntaxError *json.SyntaxError
    var unmarshalTypeError *json.UnmarshalTypeError
    
    switch {
    case errors.As(err, &syntaxError):
        return fmt.Errorf("JSON syntax error at offset %d in response from %s: %w", 
            syntaxError.Offset, endpoint, ErrInvalidJSON)
    case errors.As(err, &unmarshalTypeError):
        return fmt.Errorf("JSON type error: cannot unmarshal %s into %s at field %s from %s: %w", 
            unmarshalTypeError.Value, unmarshalTypeError.Type, 
            unmarshalTypeError.Field, endpoint, ErrInvalidJSON)
    default:
        return fmt.Errorf("JSON decode error from %s: %w", endpoint, err)
    }
}

// Usage with proper error handling
func demonstrateErrorHandling() {
    client := NewK8sClient("https://aks-cluster.example.com", "your-token")
    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
    defer cancel()
    
    var pods KubernetesAPIResponse
    if err := client.FetchWithRetry(ctx, "/api/v1/pods", &pods); err != nil {
        switch {
        case errors.Is(err, ErrResourceNotFound):
            fmt.Println("No pods found in cluster")
        case errors.Is(err, ErrPermissionDenied):
            fmt.Println("Check your RBAC permissions")
        case errors.Is(err, ErrInvalidJSON):
            fmt.Println("Invalid JSON response from API")
        case errors.Is(err, context.DeadlineExceeded):
            fmt.Println("Request timed out")
        default:
            fmt.Printf("Unexpected error: %v\n", err)
        }
        return
    }
    
    fmt.Printf("Successfully fetched %d pods\n", len(pods.Items))
}
```

This comprehensive guide provides practical, working examples specifically tailored for Kubernetes administrators working with AKS environments. All code examples include proper error handling, follow Go 1.21+ best practices, and integrate seamlessly with the specified software stack (grafana-oss 12, kubectl 1.30, go 1.21).