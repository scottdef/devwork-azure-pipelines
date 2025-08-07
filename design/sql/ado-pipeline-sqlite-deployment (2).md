# Azure DevOps Pipeline and PowerShell Deployment for SQLite Container

Complete CI/CD solution for building SQLite containers with ADO data and deploying to AKS cluster.

## Azure DevOps Pipeline YAML

### Main Pipeline (azure-pipelines.yml)
```yaml
# Azure DevOps Pipeline for SQLite Container with ADO Data
# Builds on Ubuntu 22.04, gathers JSON data, builds container, pushes to ACR

trigger:
  branches:
    include:
    - main
    - develop
  paths:
    include:
    - scripts/*
    - sql/*
    - Dockerfile
    - manifests/*

schedules:
- cron: "0 12 1-7 * 1"  # First Monday of every month at 7 AM EST (12 PM UTC)
  displayName: Monthly SQLite Data Refresh
  branches:
    include:
    - main
  always: true

variables:
  # Container Registry Variables
  containerRegistry: 'your-acr-connection'  # Azure Container Registry service connection
  imageRepository: 'sqlite-grafana'
  containerRegistryName: 'youracr.azurecr.io'
  dockerfilePath: '$(Build.SourcesDirectory)/Dockerfile'
  
  # Build Variables
  buildConfiguration: 'Release'
  vmImageName: 'ubuntu-22.04'
  
  # Versioning
  majorVersion: '1'
  minorVersion: '0'
  patchVersion: $[counter(variables['Build.SourceBranchName'], 0)]
  imageTag: '$(majorVersion).$(minorVersion).$(patchVersion)'
  latestTag: 'latest'

pool:
  vmImage: $(vmImageName)

stages:
- stage: Build
  displayName: Build and Push SQLite Container
  jobs:
  - job: Build
    displayName: Build SQLite Container with ADO Data
    steps:
    
    - checkout: self
      displayName: 'Checkout Source Code'
      clean: true
    
    - task: AzureCLI@2
      displayName: 'Verify Azure CLI and Permissions'
      inputs:
        azureSubscription: '$(containerRegistry)'
        scriptType: 'bash'
        scriptLocation: 'inlineScript'
        inlineScript: |
          echo "Current subscription:"
          az account show --output table
          echo "Verifying ACR access:"
          az acr list --output table
    
    - script: |
        echo "##[section]Installing Required Dependencies"
        sudo apt-get update
        sudo apt-get install -y jq sqlite3 curl
        
        echo "##[section]Verifying Installation"
        echo "jq version: $(jq --version)"
        echo "sqlite3 version: $(sqlite3 --version)"
        echo "curl version: $(curl --version | head -1)"
        
        echo "##[section]Setting up directory structure"
        mkdir -p data
        chmod +x scripts/*.sh
      displayName: 'Setup Build Environment'
    
    - script: |
        echo "##[section]Fetching ADO Data"
        export ADO_ORG="$(ADO_ORGANIZATION)"
        export ADO_PROJECT="$(ADO_PROJECT_NAME)"
        export ADO_PAT="$(ADO_PERSONAL_ACCESS_TOKEN)"
        
        if [[ -z "$ADO_ORG" || -z "$ADO_PROJECT" || -z "$ADO_PAT" ]]; then
          echo "##vso[task.logissue type=error]Missing required ADO variables: ADO_ORGANIZATION, ADO_PROJECT_NAME, ADO_PERSONAL_ACCESS_TOKEN"
          exit 1
        fi
        
        echo "Fetching data for organization: $ADO_ORG, project: $ADO_PROJECT"
        ./scripts/fetch-ado-data.sh
        
        echo "##[section]Validating Data Files"
        if [[ -f "./data/work_items_detailed.json" ]]; then
          WORK_ITEMS_COUNT=$(jq '.value | length' ./data/work_items_detailed.json)
          echo "Work items fetched: $WORK_ITEMS_COUNT"
        fi
        
        if [[ -f "./data/builds.json" ]]; then
          BUILDS_COUNT=$(jq '.value | length' ./data/builds.json)
          echo "Builds fetched: $BUILDS_COUNT"
        fi
        
        if [[ -f "./data/releases.json" ]]; then
          RELEASES_COUNT=$(jq '.value | length' ./data/releases.json)
          echo "Releases fetched: $RELEASES_COUNT"
        fi
        
        echo "##[section]Data Summary"
        ls -la ./data/
      displayName: 'Fetch ADO JSON Data'
      env:
        ADO_ORGANIZATION: $(ADO_ORGANIZATION)
        ADO_PROJECT_NAME: $(ADO_PROJECT_NAME)
        ADO_PERSONAL_ACCESS_TOKEN: $(ADO_PERSONAL_ACCESS_TOKEN)
    
    - script: |
        echo "##[section]Building Docker Image"
        echo "Image: $(containerRegistryName)/$(imageRepository):$(imageTag)"
        echo "Build context: $(Build.SourcesDirectory)"
        
        # Build image with build args
        docker build \
          --build-arg BUILD_DATE="$(date -Iseconds)" \
          --build-arg BUILD_VERSION="$(imageTag)" \
          --build-arg BUILD_NUMBER="$(Build.BuildNumber)" \
          --build-arg GIT_COMMIT="$(Build.SourceVersion)" \
          -t $(containerRegistryName)/$(imageRepository):$(imageTag) \
          -t $(containerRegistryName)/$(imageRepository):$(latestTag) \
          -f $(dockerfilePath) \
          $(Build.SourcesDirectory)
        
        echo "##[section]Image Information"
        docker images $(containerRegistryName)/$(imageRepository)
        
        echo "##[section]Testing Built Image"
        docker run --rm $(containerRegistryName)/$(imageRepository):$(imageTag) sqlite3 --version
      displayName: 'Build Docker Image'
    
    - task: AzureCLI@2
      displayName: 'Login to Azure Container Registry'
      inputs:
        azureSubscription: '$(containerRegistry)'
        scriptType: 'bash'
        scriptLocation: 'inlineScript'
        inlineScript: |
          az acr login --name $(echo "$(containerRegistryName)" | cut -d'.' -f1)
    
    - script: |
        echo "##[section]Pushing Images to ACR"
        docker push $(containerRegistryName)/$(imageRepository):$(imageTag)
        docker push $(containerRegistryName)/$(imageRepository):$(latestTag)
        
        echo "##[section]Push Summary"
        echo "Pushed images:"
        echo "  - $(containerRegistryName)/$(imageRepository):$(imageTag)"
        echo "  - $(containerRegistryName)/$(imageRepository):$(latestTag)"
      displayName: 'Push Images to ACR'
    
    - script: |
        echo "##[section]Updating Kubernetes Manifests"
        
        # Update ephemeral deployment
        sed -i "s|image: .*|image: $(containerRegistryName)/$(imageRepository):$(imageTag)|g" manifests/sqlite-ephemeral.yaml
        
        # Update persistent deployment  
        sed -i "s|image: .*|image: $(containerRegistryName)/$(imageRepository):$(imageTag)|g" manifests/sqlite-persistent.yaml
        
        echo "##[section]Updated Manifests"
        echo "Ephemeral deployment image:"
        grep "image:" manifests/sqlite-ephemeral.yaml
        echo "Persistent deployment image:"
        grep "image:" manifests/sqlite-persistent.yaml
        
        echo "##[section]Setting Pipeline Variables"
        echo "##vso[task.setvariable variable=finalImageTag;isOutput=true]$(imageTag)"
        echo "##vso[task.setvariable variable=finalImageName;isOutput=true]$(containerRegistryName)/$(imageRepository):$(imageTag)"
      displayName: 'Update Kubernetes Manifests'
      name: 'updateManifests'
    
    - task: PublishBuildArtifacts@1
      displayName: 'Publish Kubernetes Manifests'
      inputs:
        pathToPublish: 'manifests'
        artifactName: 'kubernetes-manifests'
        publishLocation: 'Container'
    
    - task: PublishBuildArtifacts@1
      displayName: 'Publish PowerShell Deployment Script'
      inputs:
        pathToPublish: 'scripts/Deploy-SQLiteToAKS.ps1'
        artifactName: 'deployment-scripts'
        publishLocation: 'Container'
    
    - script: |
        echo "##[section]Build Summary"
        echo "Build completed successfully!"
        echo "Image built: $(containerRegistryName)/$(imageRepository):$(imageTag)"
        echo "Build number: $(Build.BuildNumber)"
        echo "Source version: $(Build.SourceVersion)"
        echo "Trigger reason: $(Build.Reason)"
        
        if [[ "$(Build.Reason)" == "Schedule" ]]; then
          echo "This was a scheduled monthly build"
        fi
      displayName: 'Build Summary'

- stage: Deploy
  displayName: Deploy to AKS (Conditional)
  dependsOn: Build
  condition: and(succeeded(), or(eq(variables['Build.Reason'], 'Schedule'), eq(variables['Build.SourceBranch'], 'refs/heads/main')))
  variables:
    imageTag: $[ stageDependencies.Build.Build.outputs['updateManifests.finalImageTag'] ]
    imageName: $[ stageDependencies.Build.Build.outputs['updateManifests.finalImageName'] ]
  jobs:
  - deployment: DeployToAKS
    displayName: Deploy SQLite Container to AKS
    environment: 'uat-aks-deployment'
    strategy:
      runOnce:
        deploy:
          steps:
          
          - download: current
            artifact: kubernetes-manifests
            displayName: 'Download Kubernetes Manifests'
          
          - download: current
            artifact: deployment-scripts
            displayName: 'Download Deployment Scripts'
          
          - task: AzureCLI@2
            displayName: 'Deploy via PowerShell Script'
            inputs:
              azureSubscription: '$(containerRegistry)'
              scriptType: 'bash'
              scriptLocation: 'inlineScript'
              inlineScript: |
                echo "##[section]Preparing PowerShell Deployment"
                
                # Install PowerShell on Ubuntu
                wget -q https://github.com/PowerShell/PowerShell/releases/download/v7.4.0/powershell_7.4.0-1.deb_amd64.deb
                sudo dpkg -i powershell_7.4.0-1.deb_amd64.deb
                sudo apt-get install -f
                
                # Install kubectl
                curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
                sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
                
                # Configure AKS access
                az aks get-credentials --resource-group $(AKS_RESOURCE_GROUP) --name myass-cluster-uat-dc01 --overwrite-existing
                
                echo "##[section]Running PowerShell Deployment Script"
                pwsh -File "$(Pipeline.Workspace)/deployment-scripts/Deploy-SQLiteToAKS.ps1" \
                  -ClusterName "myass-cluster-uat-dc01" \
                  -ManifestsPath "$(Pipeline.Workspace)/kubernetes-manifests" \
                  -ImageName "$(imageName)" \
                  -DeploymentType "persistent" \
                  -ExecutionMode "Pipeline"
```

## Updated Kubernetes Manifests

### Updated Ephemeral Deployment (manifests/sqlite-ephemeral.yaml)
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sqlite-grafana
  namespace: monitoring
  labels:
    app: sqlite-grafana
    storage-type: ephemeral
    deployed-by: azure-devops
spec:
  replicas: 1
  selector:
    matchLabels:
      app: sqlite-grafana
  template:
    metadata:
      labels:
        app: sqlite-grafana
        storage-type: ephemeral
      annotations:
        deployment.timestamp: "PLACEHOLDER_TIMESTAMP"
        image.version: "PLACEHOLDER_VERSION"
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: sqlite
        image: youracr.azurecr.io/sqlite-grafana:latest  # Updated by pipeline
        imagePullPolicy: Always
        ports:
        - containerPort: 8080
          name: http
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
          readOnlyRootFilesystem: false
        env:
        - name: SQLITE_DB_PATH
          value: "/data/grafana.db"
        - name: DEPLOYMENT_TYPE
          value: "ephemeral"
        - name: BUILD_VERSION
          value: "PLACEHOLDER_VERSION"
        resources:
          requests:
            cpu: 100m
            memory: 256Mi
          limits:
            cpu: 500m
            memory: 1Gi
        livenessProbe:
          exec:
            command: ["sqlite3", "/data/grafana.db", "SELECT 1;"]
          initialDelaySeconds: 30
          periodSeconds: 30
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          exec:
            command: ["sqlite3", "/data/grafana.db", "SELECT COUNT(*) FROM ado_work_items;"]
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        startupProbe:
          exec:
            command: ["sqlite3", "/data/grafana.db", ".tables"]
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 30
        volumeMounts:
        - name: sqlite-data
          mountPath: /data
        - name: grafana-shared
          mountPath: /var/lib/grafana/sqlite
        - name: tmp
          mountPath: /tmp
      volumes:
      - name: sqlite-data
        emptyDir:
          sizeLimit: 5Gi
      - name: grafana-shared
        emptyDir:
          sizeLimit: 1Gi
      - name: tmp
        emptyDir:
          sizeLimit: 100Mi
      restartPolicy: Always
      terminationGracePeriodSeconds: 30
---
apiVersion: v1
kind: Service
metadata:
  name: sqlite-grafana-service
  namespace: monitoring
  labels:
    app: sqlite-grafana
spec:
  selector:
    app: sqlite-grafana
  ports:
  - port: 8080
    targetPort: 8080
    name: http
    protocol: TCP
  type: ClusterIP
  sessionAffinity: None
```

### Updated Persistent Deployment (manifests/sqlite-persistent.yaml)
```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: sqlite-grafana
  namespace: monitoring
  labels:
    app: sqlite-grafana
    storage-type: persistent
    deployed-by: azure-devops
spec:
  serviceName: sqlite-grafana-service
  replicas: 1
  selector:
    matchLabels:
      app: sqlite-grafana
  template:
    metadata:
      labels:
        app: sqlite-grafana
        storage-type: persistent  
      annotations:
        deployment.timestamp: "PLACEHOLDER_TIMESTAMP"
        image.version: "PLACEHOLDER_VERSION"
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: sqlite
        image: youracr.azurecr.io/sqlite-grafana:latest  # Updated by pipeline
        imagePullPolicy: Always
        ports:
        - containerPort: 8080
          name: http
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
          readOnlyRootFilesystem: false
        env:
        - name: SQLITE_DB_PATH
          value: "/data/grafana.db"
        - name: DEPLOYMENT_TYPE
          value: "persistent"
        - name: BUILD_VERSION
          value: "PLACEHOLDER_VERSION"
        resources:
          requests:
            cpu: 200m
            memory: 512Mi
          limits:
            cpu: 1000m
            memory: 2Gi
        livenessProbe:
          exec:
            command: ["sqlite3", "/data/grafana.db", "SELECT 1;"]
          initialDelaySeconds: 30
          periodSeconds: 30
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          exec:
            command: ["sqlite3", "/data/grafana.db", "SELECT COUNT(*) FROM ado_work_items;"]
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        startupProbe:
          exec:
            command: ["sqlite3", "/data/grafana.db", ".tables"]
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 30
        volumeMounts:
        - name: sqlite-persistent-storage
          mountPath: /data
        - name: grafana-shared-persistent
          mountPath: /var/lib/grafana/sqlite
        - name: tmp
          mountPath: /tmp
      volumes:
      - name: grafana-shared-persistent
        persistentVolumeClaim:
          claimName: grafana-shared-pvc
      - name: tmp
        emptyDir:
          sizeLimit: 100Mi
      restartPolicy: Always
      terminationGracePeriodSeconds: 30
  volumeClaimTemplates:
  - metadata:
      name: sqlite-persistent-storage
      labels:
        app: sqlite-grafana
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: "managed-premium"
      resources:
        requests:
          storage: 20Gi
  updateStrategy:
    type: RollingUpdate
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: grafana-shared-pvc
  namespace: monitoring
  labels:
    app: sqlite-grafana
spec:
  accessModes:
    - ReadWriteMany
  storageClassName: azurefile-csi
  resources:
    requests:
      storage: 10Gi
---
apiVersion: v1
kind: Service
metadata:
  name: sqlite-grafana-service
  namespace: monitoring
  labels:
    app: sqlite-grafana
spec:
  selector:
    app: sqlite-grafana
  ports:
  - port: 8080
    targetPort: 8080
    name: http
    protocol: TCP
  type: ClusterIP
  clusterIP: None
```

## PowerShell Deployment Script

### Deploy-SQLiteToAKS.ps1
```powershell
<#
.SYNOPSIS
    Deploys SQLite Grafana container to myass-cluster-uat-dc01 AKS cluster
    
.DESCRIPTION
    PowerShell script to deploy SQLite container with ADO data to AKS cluster.
    Supports manual execution, scheduled tasks, and Azure DevOps pipeline execution.
    
.PARAMETER ClusterName
    Name of the AKS cluster (default: myass-cluster-uat-dc01)
    
.PARAMETER ResourceGroup
    Azure resource group containing the AKS cluster
    
.PARAMETER ManifestsPath
    Path to Kubernetes manifests directory
    
.PARAMETER ImageName
    Full container image name with tag
    
.PARAMETER DeploymentType
    Type of deployment: ephemeral or persistent (default: persistent)
    
.PARAMETER ExecutionMode
    Execution mode: Manual, ScheduledTask, or Pipeline (default: Manual)
    
.PARAMETER LogPath
    Path for log files (default: C:\Logs\SQLiteDeployment)

.EXAMPLE
    .\Deploy-SQLiteToAKS.ps1 -DeploymentType persistent
    
.EXAMPLE
    .\Deploy-SQLiteToAKS.ps1 -ClusterName myass-cluster-uat-dc01 -DeploymentType ephemeral -ExecutionMode ScheduledTask
#>

[CmdletBinding()]
param(
    [Parameter(Mandatory = $false)]
    [string]$ClusterName = "myass-cluster-uat-dc01",
    
    [Parameter(Mandatory = $false)]
    [string]$ResourceGroup = "rg-myass-uat",
    
    [Parameter(Mandatory = $false)]
    [string]$ManifestsPath = ".\manifests",
    
    [Parameter(Mandatory = $false)]
    [string]$ImageName = "",
    
    [Parameter(Mandatory = $false)]
    [ValidateSet("ephemeral", "persistent")]
    [string]$DeploymentType = "persistent",
    
    [Parameter(Mandatory = $false)]
    [ValidateSet("Manual", "ScheduledTask", "Pipeline")]
    [string]$ExecutionMode = "Manual",
    
    [Parameter(Mandatory = $false)]
    [string]$LogPath = "C:\Logs\SQLiteDeployment"
)

# Script Configuration
$ErrorActionPreference = "Stop"
$ProgressPreference = "Continue"
$InformationPreference = "Continue"

# Initialize logging
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$logFile = Join-Path $LogPath "sqlite-deployment-$timestamp.log"

# Ensure log directory exists
if (!(Test-Path $LogPath)) {
    New-Item -ItemType Directory -Path $LogPath -Force | Out-Null
}

# Logging function
function Write-Log {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Message,
        
        [Parameter(Mandatory = $false)]
        [ValidateSet("INFO", "WARN", "ERROR", "SUCCESS")]
        [string]$Level = "INFO"
    )
    
    $logEntry = "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') [$Level] $Message"
    
    # Write to console with colors
    switch ($Level) {
        "INFO" { Write-Host $logEntry -ForegroundColor White }
        "WARN" { Write-Host $logEntry -ForegroundColor Yellow }
        "ERROR" { Write-Host $logEntry -ForegroundColor Red }
        "SUCCESS" { Write-Host $logEntry -ForegroundColor Green }
    }
    
    # Write to log file
    Add-Content -Path $logFile -Value $logEntry
}

# Error handling function
function Handle-Error {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ErrorMessage,
        
        [Parameter(Mandatory = $false)]
        [int]$ExitCode = 1
    )
    
    Write-Log "FATAL ERROR: $ErrorMessage" -Level "ERROR"
    Write-Log "Script execution failed. Check log file: $logFile" -Level "ERROR"
    
    # Send notification based on execution mode
    switch ($ExecutionMode) {
        "ScheduledTask" {
            # Could integrate with email/Teams notifications here
            Write-Log "Scheduled task failed. Manual intervention required." -Level "ERROR"
        }
        "Pipeline" {
            # Azure DevOps will handle the failure
            Write-Host "##vso[task.logissue type=error]$ErrorMessage"
            Write-Host "##vso[task.complete result=Failed;]$ErrorMessage"
        }
    }
    
    exit $ExitCode
}

# Main deployment function
function Deploy-SQLiteContainer {
    try {
        Write-Log "🚀 Starting SQLite container deployment to AKS" -Level "INFO"
        Write-Log "Execution Mode: $ExecutionMode" -Level "INFO"
        Write-Log "Cluster: $ClusterName" -Level "INFO"
        Write-Log "Deployment Type: $DeploymentType" -Level "INFO"
        Write-Log "Timestamp: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')" -Level "INFO"
        
        # Step 1: Verify Prerequisites
        Write-Log "🔍 Verifying prerequisites..." -Level "INFO"
        
        # Check if kubectl is available
        try {
            $kubectlVersion = kubectl version --client --output=json | ConvertFrom-Json
            Write-Log "kubectl version: $($kubectlVersion.clientVersion.gitVersion)" -Level "INFO"
        }
        catch {
            Handle-Error "kubectl not found or not working. Please ensure kubectl is installed and in PATH."
        }
        
        # Check if az CLI is available and logged in
        try {
            $azAccount = az account show | ConvertFrom-Json
            Write-Log "Azure CLI logged in as: $($azAccount.user.name)" -Level "INFO"
            Write-Log "Active subscription: $($azAccount.name)" -Level "INFO"
        }
        catch {
            Handle-Error "Azure CLI not available or not logged in. Please run 'az login' first."
        }
        
        # Step 2: Configure AKS Access
        Write-Log "🔐 Configuring AKS cluster access..." -Level "INFO"
        
        try {
            # Get AKS credentials with admin access
            az aks get-credentials --resource-group $ResourceGroup --name $ClusterName --admin --overwrite-existing
            Write-Log "AKS credentials configured successfully" -Level "SUCCESS"
            
            # Test cluster connectivity
            $clusterInfo = kubectl cluster-info | Out-String
            Write-Log "Cluster connectivity verified" -Level "SUCCESS"
            
            # Get current context
            $currentContext = kubectl config current-context
            Write-Log "Current kubectl context: $currentContext" -Level "INFO"
        }
        catch {
            Handle-Error "Failed to configure AKS access. Error: $($_.Exception.Message)"
        }
        
        # Step 3: Verify Manifests
        Write-Log "📋 Verifying Kubernetes manifests..." -Level "INFO"
        
        $manifestFile = Join-Path $ManifestsPath "sqlite-$DeploymentType.yaml"
        $storageClassFile = Join-Path $ManifestsPath "storage-class.yaml"
        
        if (!(Test-Path $manifestFile)) {
            Handle-Error "Manifest file not found: $manifestFile"
        }
        
        Write-Log "Using manifest: $manifestFile" -Level "INFO"
        
        # Update image name if provided
        if ($ImageName) {
            Write-Log "🔄 Updating manifest with image: $ImageName" -Level "INFO"
            
            $manifestContent = Get-Content $manifestFile -Raw
            $manifestContent = $manifestContent -replace 'image: .*', "image: $ImageName"
            $manifestContent = $manifestContent -replace 'PLACEHOLDER_TIMESTAMP', (Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ')
            $manifestContent = $manifestContent -replace 'PLACEHOLDER_VERSION', ($ImageName -split ':')[-1]
            
            # Create temporary updated manifest
            $tempManifest = Join-Path $env:TEMP "sqlite-$DeploymentType-updated.yaml"
            $manifestContent | Set-Content -Path $tempManifest
            $manifestFile = $tempManifest
        }
        
        # Step 4: Create Namespace
        Write-Log "📦 Ensuring monitoring namespace exists..." -Level "INFO"
        
        try {
            kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -
            Write-Log "Monitoring namespace ready" -Level "SUCCESS"
        }
        catch {
            Write-Log "Warning: Could not create/verify namespace: $($_.Exception.Message)" -Level "WARN"
        }
        
        # Step 5: Apply Storage Class (for persistent deployment)
        if ($DeploymentType -eq "persistent" -and (Test-Path $storageClassFile)) {
            Write-Log "💾 Applying storage class configuration..." -Level "INFO"
            
            try {
                kubectl apply -f $storageClassFile
                Write-Log "Storage class applied successfully" -Level "SUCCESS"
            }
            catch {
                Write-Log "Warning: Storage class application failed: $($_.Exception.Message)" -Level "WARN"
            }
        }
        
        # Step 6: Deploy SQLite Container
        Write-Log "🚀 Deploying SQLite container..." -Level "INFO"
        
        try {
            # Apply the manifest
            kubectl apply -f $manifestFile
            
            # Wait for deployment to be ready
            if ($DeploymentType -eq "persistent") {
                Write-Log "Waiting for StatefulSet to be ready..." -Level "INFO"
                kubectl rollout status statefulset/sqlite-grafana -n monitoring --timeout=600s
            }
            else {
                Write-Log "Waiting for Deployment to be ready..." -Level "INFO"
                kubectl rollout status deployment/sqlite-grafana -n monitoring --timeout=600s
            }
            
            Write-Log "SQLite container deployed successfully!" -Level "SUCCESS"
        }
        catch {
            Handle-Error "Deployment failed: $($_.Exception.Message)"
        }
        
        # Step 7: Verify Deployment
        Write-Log "✅ Verifying deployment..." -Level "INFO"
        
        try {
            # Get pod status
            $pods = kubectl get pods -n monitoring -l app=sqlite-grafana -o json | ConvertFrom-Json
            
            if ($pods.items.Count -eq 0) {
                Handle-Error "No pods found for SQLite deployment"
            }
            
            $pod = $pods.items[0]
            $podName = $pod.metadata.name
            $podStatus = $pod.status.phase
            
            Write-Log "Pod: $podName" -Level "INFO"
            Write-Log "Status: $podStatus" -Level "INFO"
            
            if ($podStatus -ne "Running") {
                Write-Log "Warning: Pod is not in Running state" -Level "WARN"
                
                # Get pod events for troubleshooting
                $events = kubectl get events -n monitoring --field-selector involvedObject.name=$podName --sort-by='.metadata.creationTimestamp'
                Write-Log "Recent pod events:" -Level "INFO"
                Write-Log $events -Level "INFO"
            }
            
            # Test database connectivity
            Write-Log "🔍 Testing database connectivity..." -Level "INFO"
            
            $workItemsCount = kubectl exec -n monitoring $podName -- sqlite3 /data/grafana.db "SELECT COUNT(*) FROM ado_work_items;" 2>$null
            $buildsCount = kubectl exec -n monitoring $podName -- sqlite3 /data/grafana.db "SELECT COUNT(*) FROM ado_builds;" 2>$null
            
            Write-Log "Database verification:" -Level "INFO"
            Write-Log "  Work Items: $workItemsCount" -Level "INFO"
            Write-Log "  Builds: $buildsCount" -Level "INFO"
            
            if ([int]$workItemsCount -gt 0 -or [int]$buildsCount -gt 0) {
                Write-Log "Database connectivity verified!" -Level "SUCCESS"
            }
            else {
                Write-Log "Warning: Database appears empty" -Level "WARN"
            }
        }
        catch {
            Write-Log "Warning: Verification partially failed: $($_.Exception.Message)" -Level "WARN"
        }
        
        # Step 8: Display Connection Information
        Write-Log "📊 Deployment Summary" -Level "INFO"
        Write-Log "===========================================" -Level "INFO"
        Write-Log "Cluster: $ClusterName" -Level "INFO"
        Write-Log "Namespace: monitoring" -Level "INFO"
        Write-Log "Service: sqlite-grafana-service" -Level "INFO"
        Write-Log "Database Path: /data/grafana.db" -Level "INFO"
        Write-Log "Deployment Type: $DeploymentType" -Level "INFO"
        Write-Log "Execution Mode: $ExecutionMode" -Level "INFO"
        Write-Log "Log File: $logFile" -Level "INFO"
        Write-Log "===========================================" -Level "INFO"
        
        # Step 9: Cleanup temporary files
        if ($ImageName -and (Test-Path $tempManifest)) {
            Remove-Item $tempManifest -Force
        }
        
        Write-Log "🎉 SQLite deployment completed successfully!" -Level "SUCCESS"
        
        # Return success status for pipeline
        if ($ExecutionMode -eq "Pipeline") {
            Write-Host "##vso[task.complete result=Succeeded;]SQLite deployment completed successfully"
        }
        
        return $true
    }
    catch {
        Handle-Error "Unexpected error during deployment: $($_.Exception.Message)"
        return $false
    }
}

# Script Execution
try {
    Write-Log "🔧 SQLite AKS Deployment Script Started" -Level "INFO"
    Write-Log "Parameters:" -Level "INFO"
    Write-Log "  ClusterName: $ClusterName" -Level "INFO"
    Write-Log "  ResourceGroup: $ResourceGroup" -Level "INFO"
    Write-Log "  DeploymentType: $DeploymentType" -Level "INFO"
    Write-Log "  ExecutionMode: $ExecutionMode" -Level "INFO"
    Write-Log "  ManifestsPath: $ManifestsPath" -Level "INFO"
    Write-Log "  ImageName: $ImageName" -Level "INFO"
    
    # Execute deployment
    $deploymentResult = Deploy-SQLiteContainer
    
    if ($deploymentResult) {
        Write-Log "Script completed successfully" -Level "SUCCESS"
        exit 0
    }
    else {
        Handle-Error "Deployment failed"
    }
}
catch {
    Handle-Error "Script execution failed: $($_.Exception.Message)"
}
```

## Execution Methods

### 1. Manual Execution from Windows Terminal

```powershell
# Navigate to script directory
cd C:\Scripts\SQLiteDeployment

# Run with default parameters
.\Deploy-SQLiteToAKS.ps1

# Run with specific parameters
.\Deploy-SQLiteToAKS.ps1 -DeploymentType persistent -ExecutionMode Manual

# Run with custom image (from pipeline build)
.\Deploy-SQLiteToAKS.ps1 -ImageName "youracr.azurecr.io/sqlite-grafana:1.0.123" -DeploymentType persistent
```

### 2. Scheduled Task Configuration

```powershell
# Create scheduled task for first Monday of every month at 7 AM EST
$taskName = "SQLite-AKS-Monthly-Deployment"
$scriptPath = "C:\Scripts\SQLiteDeployment\Deploy-SQLiteToAKS.ps1"
$logPath = "C:\Logs\SQLiteDeployment"

# Ensure directories exist
if (!(Test-Path "C:\Scripts\SQLiteDeployment")) { New-Item -ItemType Directory -Path "C:\Scripts\SQLiteDeployment" -Force }
if (!(Test-Path $logPath)) { New-Item -ItemType Directory -Path $logPath -Force }

# Create scheduled task action
$action = New-ScheduledTaskAction -Execute "PowerShell.exe" -Argument "-ExecutionPolicy Bypass -File `"$scriptPath`" -DeploymentType persistent -ExecutionMode ScheduledTask"

# Create trigger for first Monday of every month at 7 AM EST
$trigger = New-ScheduledTaskTrigger -Weekly -DaysOfWeek Monday -At "07:00AM" -WeeksInterval 4

# Create task settings
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -RunOnlyIfNetworkAvailable

# Create and register the scheduled task
$principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest

Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Description "Monthly SQLite container deployment to AKS cluster"

Write-Host "Scheduled task '$taskName' created successfully"
Write-Host "Next run time: $((Get-ScheduledTask -TaskName $taskName).Triggers[0].StartBoundary)"
```

### 3. Azure DevOps Pipeline Integration

```yaml
# Additional pipeline stage for PowerShell deployment
- stage: PowerShellDeploy
  displayName: Deploy via PowerShell (First Monday)
  dependsOn: Build
  condition: and(succeeded(), eq(variables['Build.Reason'], 'Schedule'))
  jobs:
  - job: ScheduledDeployment
    displayName: Monthly Scheduled Deployment
    pool:
      name: 'Azure Virtual Desktop Pool'  # Your AVD agent pool
    steps:
    
    - download: current
      artifact: kubernetes-manifests
      displayName: 'Download Kubernetes Manifests'
    
    - download: current
      artifact: deployment-scripts
      displayName: 'Download PowerShell Scripts'
    
    - task: PowerShell@2
      displayName: 'Deploy SQLite to AKS via PowerShell'
      inputs:
        targetType: 'filePath'
        filePath: '$(Pipeline.Workspace)/deployment-scripts/Deploy-SQLiteToAKS.ps1'
        arguments: >
          -ClusterName "myass-cluster-uat-dc01"
          -ResourceGroup "$(AKS_RESOURCE_GROUP)"
          -ManifestsPath "$(Pipeline.Workspace)/kubernetes-manifests"
          -ImageName "$(containerRegistryName)/$(imageRepository):$(Build.BuildNumber)"
          -DeploymentType "persistent"
          -ExecutionMode "Pipeline"
        workingDirectory: '$(Pipeline.Workspace)'
        pwsh: true
      env:
        AZURE_CLIENT_ID: $(AZURE_CLIENT_ID)
        AZURE_CLIENT_SECRET: $(AZURE_CLIENT_SECRET)
        AZURE_TENANT_ID: $(AZURE_TENANT_ID)
```

## Pipeline Variables Configuration

### Required Azure DevOps Variables

```yaml
# Variable Groups in Azure DevOps Library
variables:
- group: 'SQLite-AKS-Config'  # Contains:
  # ADO_ORGANIZATION: your-organization
  # ADO_PROJECT_NAME: your-project
  # AKS_RESOURCE_GROUP: rg-myass-uat
  # containerRegistryName: youracr.azurecr.io

- group: 'SQLite-Secrets'     # Contains:
  # ADO_PERSONAL_ACCESS_TOKEN: your-pat-token
  # AZURE_CLIENT_ID: service-principal-id
  # AZURE_CLIENT_SECRET: service-principal-secret  
  # AZURE_TENANT_ID: your-tenant-id
```

## Setup Instructions

### 1. Azure DevOps Setup
```bash
# Create service connections
# - Azure Resource Manager connection for ACR and AKS
# - Container Registry connection for image operations

# Configure variable groups with required secrets
# - ADO credentials for data fetching
# - Azure service principal for AKS access
```

### 2. AKS Cluster Preparation
```bash
# Ensure proper RBAC permissions
kubectl create clusterrolebinding azure-devops-admin --clusterrole=cluster-admin --serviceaccount=kube-system:azure-devops

# Create monitoring namespace
kubectl create namespace monitoring
```

### 3. Azure Virtual Desktop Setup
```powershell
# Install required tools on AVD
Install-Module -Name Az -Force
Install-Module -Name AzureAD -Force

# Download kubectl
Invoke-WebRequest -Uri "https://dl.k8s.io/release/v1.30.0/bin/windows/amd64/kubectl.exe" -OutFile "C:\Windows\System32\kubectl.exe"

# Configure Azure CLI
az login --service-principal -u $env:AZURE_CLIENT_ID -p $env:AZURE_CLIENT_SECRET --tenant $env:AZURE_TENANT_ID
```

This complete solution provides automated SQLite container builds with ADO data integration and flexible deployment options for your AKS environment.