# cool-template-go-api

The template behind the `go-server` preset: a JSON API in Go 1.24, standard
library only. A `cool-app-create` request copies it, writes your team and
app into `cool.json`, renames the module, and the first push deploys to dev.

```sh
go run .                 # http://localhost:8080/api/items
go test ./...
coolctl validate         # check cool.json
coolctl render dockerfile  # see the Dockerfile the preset builds with
```

## What to keep

| | Why |
| --- | --- |
| listen on `$PORT` | the platform chooses the port |
| `GET /healthz` answers 200 | the probes ask it; a release that does not answer never takes traffic |
| `GET /metrics` | dashboards and the rolling-release gate read it |
| stop cleanly on `SIGTERM` | a deploy or a scale-down must not drop requests |

Everything else is yours: replace `Item`, `data/items.json` and the two
`/api/items` routes.

## What to do next

- Settings go under `env` in `cool.json`; per-environment values under
  `environments`.
- Secrets go in the team Key Vault as `<app>-<NAME>`, and their names under
  `secrets` in `cool.json`. They arrive as environment variables.
- To publish the API through API Management, describe it in an OpenAPI
  file, name it under `api` in `cool.json`, and open a **Publish an API**
  request.
- There is no Dockerfile on purpose. If you need your own, commit one and
  set `"preset": "dockerfile"`.
