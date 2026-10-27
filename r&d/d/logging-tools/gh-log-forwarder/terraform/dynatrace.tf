# ---------------------------------------------------------- ingest token
#
# Least privilege: the forwarder's token can do one thing, logs.ingest.
# The provider defaults `enabled` to false, so it is set explicitly.
# Rotate with: terraform apply -replace='dynatrace_api_token.logs_ingest[0]'

resource "dynatrace_api_token" "logs_ingest" {
  count = var.manage_dt_token ? 1 : 0

  name            = "ghlog2dt-${var.github_owner}-${var.forwarder_repo}"
  enabled         = true
  scopes          = ["logs.ingest"]
  expiration_date = var.dt_token_expiration
}

# ---------------------------------------------------------- Grail bucket
#
# A dedicated bucket gives build logs their own retention and lets IAM
# policies (storage:bucket-name) decide who may read them.

resource "dynatrace_platform_bucket" "ci_logs" {
  count = var.manage_dt_bucket ? 1 : 0

  name         = var.dt_bucket_name
  display_name = "GitHub Actions build logs"
  table        = "logs"
  retention    = var.dt_bucket_retention_days
}

# ------------------------------------------------- OpenPipeline (opt-in)
#
# The DQL/DPL in the processors below is not validated by Terraform:
# paste each into the pipeline editor's sample-data preview before
# enabling this in production.

resource "dynatrace_openpipeline_v2_logs_pipelines" "github_actions" {
  count = var.manage_openpipeline_pipeline ? 1 : 0

  display_name = "GitHub Actions build logs (ghlog2dt)"
  custom_id    = "pipeline_ghlog2dt_github_actions"

  processing {
    processors {
      processor {
        type        = "dql"
        id          = "processor_ghlog2dt_exit_code"
        description = "Extract the step exit code from the runner's failure line"
        matcher     = "matchesPhrase(content, \"Process completed with exit code\")"
        sample_data = jsonencode({ content = "Process completed with exit code 2." })
        dql {
          script = "parse content, \"LD 'exit code ' INT:gh.exit_code\""
        }
        enabled = true
      }
      processor {
        type        = "dql"
        id          = "processor_ghlog2dt_mask_tokens"
        description = "Second line of defence: mask GitHub tokens the client-side redaction missed"
        matcher     = "matchesPhrase(content, \"ghp_\") or matchesPhrase(content, \"ghs_\") or matchesPhrase(content, \"github_pat_\")"
        sample_data = jsonencode({ content = "token ghp_abcdefghijklmnopqrstuvwxyz0123456789" })
        dql {
          script = "fieldsAdd content = replacePattern(content, \"('ghp_' | 'gho_' | 'ghu_' | 'ghs_' | 'ghr_') ALNUM{36,255}\", \"[REDACTED]\")"
        }
        enabled = true
      }
    }
  }

  storage {
    processors {
      processor {
        type        = "bucketAssignment"
        id          = "processor_ghlog2dt_bucket"
        description = "Store GitHub Actions logs in their own bucket"
        matcher     = "true"
        bucket_assignment {
          bucket_name = var.dt_bucket_name
        }
        enabled = true
      }
    }
  }

  depends_on = [dynatrace_platform_bucket.ci_logs]
}

# DANGER: this resource owns the ENTIRE logs routing table of the
# environment. Leave manage_openpipeline_routing = false unless this
# module is the single owner of logs routing; otherwise add the first
# entry below to your central routing configuration.
resource "dynatrace_openpipeline_v2_logs_routing" "routing" {
  count = var.manage_openpipeline_routing ? 1 : 0

  routing_entries {
    routing_entry {
      enabled       = true
      pipeline_type = "custom"
      pipeline_id   = dynatrace_openpipeline_v2_logs_pipelines.github_actions[0].id
      matcher       = "matchesValue(log.source, \"github-actions\") or matchesValue(log.source, \"ghlog2dt.self\")"
      description   = "ghlog2dt: GitHub Actions build logs"
    }
    routing_entry {
      enabled             = true
      pipeline_type       = "builtin"
      builtin_pipeline_id = "default"
      matcher             = "true"
      description         = "Everything else: default pipeline"
    }
  }

  lifecycle {
    precondition {
      condition     = var.manage_openpipeline_pipeline
      error_message = "manage_openpipeline_routing needs manage_openpipeline_pipeline = true."
    }
  }
}
