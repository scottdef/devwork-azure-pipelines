output "intake_pages_url" {
  description = "Public URL of the intake form site."
  value       = local.repos.intake.homepage
}

output "library_pages_url" {
  description = "Public URL of the library catalog."
  value       = local.repos.library.homepage
}

output "repositories" {
  description = "Full names of the managed repositories."
  value       = { for k, r in github_repository.this : k => r.full_name }
}

output "secret_commands" {
  description = "Run these after apply to store the App private key (kept out of Terraform state)."
  value = [
    for k, r in github_repository.this :
    "gh secret set APP_PRIVATE_KEY --repo ${r.full_name} < issueops-autoadmin-app.private-key.pem"
  ]
}
