# terraform

Both repositories are provisioned together (repos, Pages, labels, Actions variables,
`github-pages` environment, ruleset) by the case study's shared Terraform:
[`../../terraform`](../../terraform/README.md). Keeping one root module avoids two states
that each half-own the labels and the App bypass.
