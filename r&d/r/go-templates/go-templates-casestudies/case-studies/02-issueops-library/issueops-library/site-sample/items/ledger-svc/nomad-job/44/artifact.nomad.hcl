# ledger-svc: Nomad job, phase 1 of ledger-svc/nomad-job/44
job "ledger-svc" {
  datacenters = ["dc1"]
  type        = "service"

  group "ledger-svc" {
    count = 3

    network {
      port "http" {
        to = 8080
      }
    }

    task "ledger-svc" {
      driver = "docker"

      config {
        image = "ghcr.io/coolgitorg/ledger:1.4.2"
        ports = ["http"]
      }

      resources {
        cpu    = 200
        memory = 256
      }

      env {
        LOG_LEVEL = "info"
      }
    }
  }
}
