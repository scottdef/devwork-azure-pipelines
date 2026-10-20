output "tmplwatch_full_name" {
  description = "owner/name of the code repository."
  value       = github_repository.tmplwatch.full_name
}

output "tmplwatch_kv_full_name" {
  description = "owner/name of the key/value repository (source.repo in tmplwatch.json)."
  value       = github_repository.tmplwatch_kv.full_name
}

output "tmplwatch_clone_urls" {
  description = "SSH and HTTPS clone URLs of the code repository."
  value = {
    ssh  = github_repository.tmplwatch.ssh_clone_url
    http = github_repository.tmplwatch.http_clone_url
  }
}

output "tmplwatch_kv_clone_urls" {
  description = "SSH and HTTPS clone URLs of the key/value repository."
  value = {
    ssh  = github_repository.tmplwatch_kv.ssh_clone_url
    http = github_repository.tmplwatch_kv.http_clone_url
  }
}
