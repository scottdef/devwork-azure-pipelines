output "dev_repo_clone_url" {
  value = github_repository.dev.ssh_clone_url
}

output "test_repo_clone_url" {
  value = github_repository.test.ssh_clone_url
}

output "prod_repo_clone_url" {
  value = github_repository.prod.ssh_clone_url
}

output "dev_repo_html_url" {
  value = github_repository.dev.html_url
}

output "test_repo_html_url" {
  value = github_repository.test.html_url
}

output "prod_repo_html_url" {
  value = github_repository.prod.html_url
}
