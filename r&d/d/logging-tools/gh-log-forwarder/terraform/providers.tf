# GitHub: authenticate as the issueops-autoadmin-app installation.
# Secrets come from the environment, never from tfvars:
#   GITHUB_APP_ID, GITHUB_APP_INSTALLATION_ID, GITHUB_APP_PEM_FILE (PEM content)
provider "github" {
  owner     = var.github_owner
  auth_mode = "app" # credentials only from the GITHUB_APP_* environment variables
}

# Dynatrace: classic resources (API tokens) use an API token with
# apiTokens.read + apiTokens.write (+ settings.read/write for OpenPipeline):
#   DYNATRACE_API_TOKEN
# Platform resources (Grail buckets) need an OAuth client with
# storage:bucket-definitions:read/write/delete:
#   DT_CLIENT_ID, DT_CLIENT_SECRET, DT_ACCOUNT_ID
# Set DYNATRACE_HTTP_OAUTH_PREFERENCE=true so settings objects created
# through OAuth get an owner instead of becoming public.
provider "dynatrace" {
  dt_env_url = var.dt_env_url
}
