// Creates or updates one model deployment on an EXISTING Microsoft Foundry
// (AIServices / Azure OpenAI) account. Parameters are produced by
// `issueops foundry plan` (encoding/json, never text templating) from an
// approved foundry-model-deployment request, and deployed with:
//
//   az deployment group create -g <rg> -n issueops-<issue>-<run> \
//     --template-file deploy/bicep/model-deployment.bicep --parameters @parameters.json
//
// The account itself (network rules, CMK, private endpoints, RBAC) is managed
// by the platform team's landing-zone code, not by self-service requests.

targetScope = 'resourceGroup'

@description('Name of the existing Foundry (Cognitive Services) account.')
param accountName string

@description('Deployment name that callers use (MODEL_DEPLOYMENT_NAME).')
@minLength(2)
@maxLength(64)
param deploymentName string

@description('Model name, e.g. gpt-4.1-mini.')
param modelName string

@description('Model version, e.g. 2025-04-14.')
param modelVersion string

@description('Model format (publisher), e.g. OpenAI or Microsoft.')
param modelFormat string = 'OpenAI'

@description('Deployment type (SKU).')
@allowed([
  'GlobalStandard'
  'DataZoneStandard'
  'Standard'
  'GlobalProvisionedManaged'
  'DataZoneProvisionedManaged'
  'ProvisionedManaged'
  'GlobalBatch'
  'DataZoneBatch'
])
param skuName string

@description('Capacity: thousands of tokens per minute for Standard SKUs, PTUs for provisioned SKUs.')
@minValue(1)
param capacity int

@description('How the deployment follows model version updates.')
@allowed([
  'OnceNewDefaultVersionAvailable'
  'OnceCurrentVersionExpired'
  'NoAutoUpgrade'
])
param versionUpgradeOption string = 'OnceNewDefaultVersionAvailable'

@description('Content filter (RAI policy) applied to the deployment.')
param raiPolicyName string = 'Microsoft.DefaultV2'

resource account 'Microsoft.CognitiveServices/accounts@2024-10-01' existing = {
  name: accountName
}

resource deployment 'Microsoft.CognitiveServices/accounts/deployments@2024-10-01' = {
  parent: account
  name: deploymentName
  sku: {
    name: skuName
    capacity: capacity
  }
  properties: {
    model: {
      format: modelFormat
      name: modelName
      version: modelVersion
    }
    versionUpgradeOption: versionUpgradeOption
    raiPolicyName: raiPolicyName
  }
}

output deploymentId string = deployment.id
output provisioningState string = deployment.properties.provisioningState
output endpoint string = account.properties.endpoint
