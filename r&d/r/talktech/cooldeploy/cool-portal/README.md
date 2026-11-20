# cool-portal

The front page of CoolDeploy: which apps exist, what runs where, and the
links that start a request. A static site on GitHub Pages, written in HTML,
CSS, HTMX and a little plain JavaScript. No framework and no build step of
its own.

```
site/index.html        the app list
site/app.html          one app
site/css/portal.css
site/js/htmx.min.js    HTMX 2.0.11 (0BSD), vendored
site/js/portal.js      the app page
site/js/auth.js        Entra ID sign-in with PKCE, for the live view
site/js/config.js      where the request forms and cool-api are
site/fragments/        generated: the app list and one fragment per app
site/data/apps.json    generated: the same, as JSON
```

## How it knows what is deployed

It reads no API to draw the page. The Pages workflow checks out
`cool-gitops` and runs

```sh
coolctl render portal -gitops ../cool-gitops -out site
```

which writes `site/fragments/` and `site/data/apps.json`. The browser loads
those like any other file. So the portal shows what Git says should be
running, works for anyone who can open the page, and keeps working when the
control plane is down.

The build runs when this repository changes, when `cool-gitops` announces a
change, and hourly.

## The live view

The app page can also show what the clusters say: sync and health from
Argo CD, through `cool-api` behind API Management. That needs a sign-in, so
it is behind a button:

1. Register a single-page application in Entra ID with the portal's URL as
   a redirect URI, and grant it the scope of the environment's `cool-<env>`
   application.
2. Fill `tenant`, `clientId`, `scope` and `api` in `site/js/config.js`. None
   of them is a secret.

Sign-in is authorization code with PKCE, written out in `site/js/auth.js`:
about a hundred lines, no library, no client secret, and the token stays in
`sessionStorage`.

## The portal changes nothing

Every button is a link to an issue form in `CoolEngOrg/issueops`, with the
app filled in. The portal holds no credential that could deploy anything,
and there is nothing in it to attack for one.

## Try it

```sh
../demo/run-local.sh --keep
```

runs the demo, renders the portal from the demo's GitOps repository, and
serves it at `http://127.0.0.1:8000` with the live view pointed at the local
`cool-api`.

Or by hand:

```sh
coolctl render portal -gitops ../cool-gitops -out site
coolctl serve -dir site
```

## GitHub Pages and private organizations

On GitHub Enterprise Cloud, Pages sites can be private: visible to people
with read access to the repository. Publish this one that way. The page
names every app and release in the organization.
