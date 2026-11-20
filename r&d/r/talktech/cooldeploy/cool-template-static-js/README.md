# cool-template-static-js

The template behind the `static` preset: HTML, CSS and plain JavaScript,
served as they are. No Go, no Node, no build step.

```sh
coolctl serve -dir .       # http://127.0.0.1:8000
coolctl validate
```

The platform puts these files in an image with its own small file server,
`cool-static`, which answers `/healthz` and `/metrics` for you, sends
sensible cache headers, and does not serve dotfiles, `cool.json` or a
`Dockerfile`.

| File | Holds |
| --- | --- |
| `index.html` | the page |
| `js/app.js` | loads `data/items.json` and filters it as you type |
| `style.css` | the styles |
| `data/items.json` | sample data |

A static app gets the same things as any other: a preview per pull request,
promotion, rolling releases, rollback, custom domains. It has no secrets and
needs none: anything in these files is public to whoever can reach the site.

If the site needs an API, that is a second app, with the `go-server` preset.
