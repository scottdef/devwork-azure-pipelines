# cool-template-go-htmx

The template behind the `go-htmx` preset: a server-rendered page in Go 1.24
with HTMX, standard library only. No build step, no client-side framework:
the server renders HTML, and the browser swaps in the fragments it asks for.

```sh
go run .                   # http://localhost:8080
go test ./...
coolctl validate
```

| File | Holds |
| --- | --- |
| `main.go` | the server: routes, the page, the fragment, the platform's four endpoints |
| `web/templates/page.html` | the `page` template and the `notes` fragment it includes |
| `web/static/` | the stylesheet and `htmx.min.js` 2.0.11 (0BSD), vendored so the page loads nothing from elsewhere |

The pattern to copy: a handler for the whole page, and a handler per
fragment that renders one named template. A form says where its answer goes
with `hx-post` and `hx-target`.

The page sends `Content-Security-Policy: default-src 'self'`, so inline
scripts and styles do not run. Keep it; put behaviour in attributes and
styles in the stylesheet.

Keep `/healthz`, `/metrics`, `$PORT` and the `SIGTERM` handling. Replace the
notes: they live in memory, one list per replica, which is fine for a
template and wrong for an app.
