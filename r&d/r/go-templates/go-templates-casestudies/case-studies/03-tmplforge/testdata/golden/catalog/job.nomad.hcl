job "tmplforge" {
  datacenters = ["dc1"]
  type        = "service"

  group "app" {
    count = 2

    network {
      port "http" { to = 8088 }
    }

    task "server" {
      driver = "docker"

      config {
        image = "ghcr.io/coolgitorg/tmplforge:v0.1.0"
        ports = ["http"]
      }

      template {
        destination = "secrets/env"
        env         = true
        data        = <<EOT
{{ with secret "kv/data/tmplforge" }}TOKEN={{ .Data.data.token }}{{ end }}
EOT
      }

      resources {
        cpu    = 200
        memory = 128
      }
    }
  }
}
