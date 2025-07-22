package main

// Configuration Template
// Copy this file to update azure-devops-processor.go with your settings

// Update the getConfig() function in azure-devops-processor.go:
func getConfig() Config {
	return Config{
		Organization: "myorga",          // 👈 CHANGE THIS: Your Azure DevOps organization name
		Project:      "project001",     // 👈 CHANGE THIS: Your project name  
		PAT:          os.Getenv("AZURE_DEVOPS_PAT"), // Keep this as-is
		APIVersion:   "7.2-preview",    // Keep this as-is unless you need a different version
	}
}

// Update the envIds list in the main() function:
// envIds := []int{8, 10, 15, 20} // 👈 CHANGE THIS: Your environment IDs

// Example configurations for different scenarios:

// Single Environment
// envIds := []int{8}

// Multiple Production Environments  
// envIds := []int{1, 2, 3, 4, 5}

// Dev, Test, Staging, Prod
// envIds := []int{10, 20, 30, 40}

// All environments in your project
// envIds := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

/* 
How to find your values:

1. Organization Name:
   - Look at your Azure DevOps URL: https://dev.azure.com/{ORGANIZATION}/
   - Example: https://dev.azure.com/contoso/ → Organization = "contoso"

2. Project Name:
   - Look at your project URL: https://dev.azure.com/org/{PROJECT}/
   - Example: https://dev.azure.com/contoso/MyProject/ → Project = "MyProject"

3. Environment IDs:
   - Go to Pipelines → Environments in Azure DevOps
   - Click on an environment
   - Look at the URL: .../_environments/{ID}/...
   - The number is your environment ID

4. Personal Access Token:
   - User Settings → Personal Access Tokens → New Token
   - Scopes needed: Environment (Read), Build (Read)
   - Set as environment variable: export AZURE_DEVOPS_PAT="your-token"
*/