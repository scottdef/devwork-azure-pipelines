# checkout

The payments team's sample app on CoolDeploy: a small shop with a
server-rendered HTMX front end and a JSON API. Go 1.24, standard library
only, one binary, everything embedded.

In a real organization this directory is the repository
`CoolEngOrg/payments-checkout`.

## Run it

```sh
make run                       # http://localhost:8080
make test
```

```sh
curl -s localhost:8080/api/products?q=desk | jq -M '.products[].name'
curl -s -X POST localhost:8080/api/orders \
  -H 'Content-Type: application/json' \
  -d '{"email":"ada@example.com","items":[{"sku":"CB-1001","qty":2}]}' | jq -M .
CRON_SECRET=s3cret make run &
curl -s -X POST -H 'Authorization: Bearer s3cret' localhost:8080/internal/cron/expire-orders
```

## What the platform asks of an app, and where this one does it

| The platform | The app | Where |
| --- | --- | --- |
| sets `PORT` | listens on it | `main.go` |
| probes `GET /healthz` | answers 200 | `main.go` |
| scrapes `GET /metrics` | serves request counts and durations by route | `main.go`, `internal/metrics` |
| sends `SIGTERM` before stopping a pod | finishes in-flight requests | `main.go` |
| mounts feature flags at `COOL_FLAGS_FILE` | shows the express checkout button | `main.go`, `web.go` |
| calls cron paths with `CRON_SECRET` | checks the bearer token | `api.go` |
| sets `COOL_RELEASE` | names the release in the footer and in `X-Cool-Release` | `main.go` |

That last one is what makes a rolling release visible: reload the page during
one and the footer changes between two releases.

## cool.json

`cool.json` is all the deployment configuration there is. It asks for a
public app with two to six replicas, a longer timeout for `/api/`, cached
static files, one redirect, one cron and one secret, and it gives `dev` and
`prd` their own scale. `coolctl validate` checks it.

There is no Dockerfile: the `go-htmx` preset supplies one.
`coolctl render dockerfile` prints it.

## Sample data

`data/products.json` holds twelve products, one of them out of stock.
`data/orders.json` holds five orders: two paid, one expired, two pending and
old enough that the first cron run expires them. The tests check that every
seeded order adds up against the catalogue.

Orders live in memory. Each replica has its own and a restart forgets them,
which is fine for a demo and the first thing to replace in a real shop: put a
database behind the methods of `shop` in `shop.go`. Nothing else changes.
The cart needs no server state at all; it is a cookie.

## Deploying

`.github/workflows/cool.yml` calls the platform's pipeline and passes no
secrets. A push to `main` deploys to `dev`. A pull request gets a preview at
`https://checkout-pr-<n>.preview.dev.coolapps.example`. Everything after
`dev` is a request in `CoolEngOrg/issueops`: promote, rolling release,
rollback, environment variables, domains, publishing the API.
