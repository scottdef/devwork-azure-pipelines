# Managed by tmplwatch. Do not edit by hand.
job "api" {
  datacenters = ["dc1", "dc2"]
  type        = "service"

  update {
    max_parallel     = 1
    min_healthy_time = "10s"
    auto_revert      = true
  }

  group "api" {
    count = 3

    network {
      port "http" { to = 8080 }
    }

    service {
      name     = "api"
      port     = "http"
      provider = "nomad"
      check {
        type     = "http"
        path     = "/healthz"
        interval = "10s"
        timeout  = "2s"
      }
    }

    task "app" {
      driver = "docker"
      config {
        image = "ghcr.io/coolgitorg/api:1.4.2"
        ports = ["http"]
      }
      env {
        DATABASE_URL = "postgres://api@db.service.consul:5432/api"
        GREETING     = "hello $${USER} %%{if}"
        LOG_LEVEL    = "info"
      }

      # Nomad renders this block at runtime; tmplwatch leaves it alone.
      template {
        destination = "local/instance.env"
        env         = true
        data        = <<EOT
NOMAD_ALLOC={{ env "NOMAD_ALLOC_ID" }}
EOT
      }

      resources {
        cpu    = 200
        memory = 256
      }
    }
  }
}
