# Template functions and the `#!tmpl` header

One FuncMap (`issueops-library/internal/funcs`) serves every library template: item
artifacts, `next.tmpl`, HTML pages, the Atom feed, comment templates and `tpl`. A
template that works with `tpl` works in `pagegen`.

| Function | Signature | Example | Result |
|---|---|---|---|
| `dict` | `dict k1 v1 k2 v2 ...` | `{{template "row" dict "item" . "odd" true}}` | `map[string]any`; error on odd count or non-string key |
| `list` | `list a b c` | `{{.branches \| default (list "main")}}` | `[]any` |
| `default` | `default def value` | `{{.port \| default 8080}}` | `value` unless empty (nil, "", 0, false, empty list/map) |
| `toJSON` | `toJSON v` | `{{toJSON .routes}}` | compact JSON, no HTML escaping |
| `indent` | `indent n s` | `{{indent 4 .block}}` | every non-empty line prefixed with n spaces |
| `nindent` | `nindent n s` | `key:{{nindent 2 .yaml}}` | newline, then `indent` |
| `sha256sum` | `sha256sum s` | `{{sha256sum .payload}}` | hex sha256 |
| `humanSize` | `humanSize n` | `{{humanSize .Size}}` | `512 B`, `1.5 KiB`, `3.0 MiB` |
| `dateFmt` | `dateFmt layout t` | `{{dateFmt "date" .Due}}` | `date` (2006-01-02), `rfc3339`, `human`, or any Go layout; accepts `time.Time` or RFC 3339 string |
| `plural` | `plural n word [many]` | `{{plural 3 "entry" "entries"}}` | `1 item` / `3 items` / `3 entries` |
| `slugify` | `slugify s` | `{{slugify "Payments API"}}` | `payments-api` |
| `add` | `add a b ...` | `{{add .Phase 1}}` | sum; integers stay integers |
| `join` | `join sep list` | `{{join ", " .owners}}` | joined string forms |
| `quote` | `quote s` | `name = {{quote .name}}` | Go-quoted string (valid HCL/JSON string) |
| `lower`, `upper`, `trim` | | | `strings` equivalents |
| `hasKey` | `hasKey map key` | `{{if hasKey . "port"}}` | bool |

Page templates add `overdue loan now` and `odd i`; the feed adds `xml s`
(`xml.EscapeText`); intake comment templates use `code` (fenced block that cannot be
broken out of) and `inline` (backticked, `|`-escaped, single line).

## The `#!tmpl` header

The first line of an item template may configure how it is parsed:

```
#!tmpl ext=nomad.hcl delims=[[,]] missingkey=error
```

| Key | Default | Meaning |
|---|---|---|
| `ext` | `txt` | artifact file extension (`artifact.<ext>`); `[a-z0-9]{1,12}` with at most one dot |
| `delims` | `{{,}}` | left,right action delimiters; use `[[,]]` when the output itself contains `{{` (GitHub Actions, Helm, Go templates) |
| `missingkey` | `zero` | `zero`, `error`, `default` or `invalid` (text/template option) |

The line is removed before parsing. Whatever `missingkey` says, output containing
`<no value>` is refused (`item.Execute`): it always means a payload key is missing and
the template forgot a `default`.

## Data available to item templates

The payload's keys at the top level (`.name`, `.port`, ...) plus provenance under `._`:

| Key | Example |
|---|---|
| `._.item_id` | `payments-api/caddyfile/42` |
| `._.source_id` | `payments-api` |
| `._.template_id` | `caddyfile` |
| `._.phase` | `1` |
| `._.issue` | `42` |

Numbers in the payload are `json.Number`, so `8443` prints as `8443`, never `8443.0`.

## Meta-templates

`templates/meta/next.tmpl.tmpl` runs at publish time with `[[ ]]` delimiters and
`missingkey=error`. Its `{{ }}` are literal text, so its *output* is a Go template, the
`next.tmpl` a borrower downloads:

```
[[- range .Keys]]
  [[quote .]]: {{if hasKey . [[quote .]]}}{{toJSON (index . [[quote .]])}}{{else}}{{[[index $.Item.Payload . | toJSON | quote]]}}{{end}},
[[- end]]
```

becomes, for a payload with `port: 8443`,

```
  "port": {{if hasKey . "port"}}{{toJSON (index . "port")}}{{else}}{{"8443"}}{{end}},
```

so `tpl -d mine.json -t next.tmpl` keeps every phase-1 value the borrower did not override.
