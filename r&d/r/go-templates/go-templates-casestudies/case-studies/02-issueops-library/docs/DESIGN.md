# Design

The system is a pipeline of small programs that pass text: an issue body becomes JSON,
JSON becomes a dispatch event, an event becomes files in git, and files become a static
site. Each stage can be run by hand from a shell, which is also how every stage is tested.

## Two repositories

| | issueops-intake | issueops-library |
|---|---|---|
| Holds | issue forms, rules, the form page | items, the ledger, the catalog site |
| Who writes | requesters (issues) | only automation (the App) |
| Pages | the request form | the catalog |
| Token in its workflows | `GITHUB_TOKEN` (issues) + App token to dispatch | App token (push, comment) + `GITHUB_TOKEN` (Pages) |

Splitting them keeps the blast radius of each part small. A requester can file issues
without any access to the library. The library repository is effectively a database:
its main branch is written only by workflows that act as the App, and its ruleset makes
every human change a reviewed pull request. Rebuilding the library from scratch means
replaying its git history; nothing lives anywhere else.

## Why `repository_dispatch`

The intake must start work in another repository, and the alternatives are worse:

- `workflow_dispatch` has a small, fixed set of typed inputs and names one workflow file;
  `repository_dispatch` names an *event*, so the library can reorganise its workflows
  without the intake noticing.
- A shared PAT or deploy key would put a long-lived credential in the intake. The App
  token is minted per run, limited to two repositories, and expires within an hour.
- `client_payload` carries up to 10 top-level keys. We use 8 for a publish (`source_id,
  template_id, phase, issue, repo, requester, payload, sha256`), 6 for a borrow and 4 for a
  return; `dispatch.Build` refuses anything over 10, and a test asserts it on the wire.

The payload travels as a *string*, not as a nested object, for two reasons: its sha256
can be checked byte for byte on arrival (a nested object would be re-serialised by
GitHub), and the receiver decodes it once with `UseNumber`, so `8443` stays `8443`.

## Parsing issue bodies without a YAML library

`issueparse` needs the form's field ids and labels. `internal/yamlite` is a line scanner
for the YAML subset issue forms use: block maps, block sequences, quoted and plain
scalars, flow sequences of scalars, and `|` and `>` block scalars. It refuses
anchors, aliases, tags and flow maps with an error. That is about 410 lines
plus tests, instead of a dependency we are not allowed to take.

The body parser works on headings (`### Label`), which is how GitHub renders forms. It
tolerates the edits people actually make: CRLF line endings, changed heading case, extra
text above the first heading, unknown headings (their sections are dropped), a field
repeated later in the body (the last one wins), and code fences that contain `###`
lines (a heading inside a fence never splits a field). `_No response_` means absent.
`render: json` textareas lose exactly one surrounding fence. The fuzz target asserts it
never panics, never emits a key the form does not define, and never leaks a `\r`.

## Form and page cannot drift

`pagegen -intake` reads `.github/ISSUE_TEMPLATE/*.yml` and `schema/*.json` and generates
the form page: labels, help text, options, required marks, HTML `pattern` and
`maxlength`. `pages.yml` redeploys the page whenever a form changes. A test
(`TestNoDrift`) checks that every field and option in the forms appears on the page.

## Rendering items

Item templates are `text/template` files with an optional first line:

```
#!tmpl ext=yml delims=[[,]] missingkey=zero
```

The header travels with the template, so adding a template needs no code change, only
`templates/items/<id>.tmpl` and an entry in the issue form's dropdown and
`event.Templates`. The workflow template uses `[[ ]]` so GitHub's `${{ }}` passes through
untouched. Templates see the payload at the top level and provenance under `._`
(`._.item_id`, `._.phase`), so a payload key can never shadow provenance.

Output containing `<no value>` is refused: it means a template used a key the payload
lacked and had no `default`. Shipping `image = <no value>` to Nomad would be worse
than refusing the request with a comment.

`bundle.tar.gz` is deterministic: sorted entries, the publish time as mtime, fixed
owner, PAX format, no gzip timestamp, OS byte 255. Re-rendering the same event
yields the same bytes and the same sha256, so `SHA256SUMS` is stable across replays.

## Full rebuilds

Every event rebuilds the whole site from `items/` and the ledger. At the scale of a
library (hundreds to low thousands of items) a rebuild takes milliseconds, and it removes a
whole class of bugs: no page can go stale, a template change reaches every page at once,
and the output is a pure function of (items, ledger, base URL, time). `Site.Write`
renders into `site.tmp` and renames it over `site`, so a failed render never leaves half a site.

Pages are `html/template` with `embed.FS`: a `layout` defining `block "content"`, shared
partials, and one `Clone()` per page (including one per item) so each page's
`content` definition is private to it. Fragments (`ledger/open.html`,
`ledger/history.html`, `items/…/status.html`) are the same partials rendered on their
own; htmx fetches them with `hx-get`, a plain GET that static hosting serves. Their links
use the site's absolute path prefix (`/issueops-library/`) because they are swapped into
pages at different depths; the copies inlined at build time use relative links so the
site also works from a local checkout (`site-sample/`).

The Atom feed and sitemap are `text/template` with an explicit `xml` escaper, because
`html/template` escaping rules are wrong for XML.

## Concurrency without losing events

GitHub keeps at most one *pending* run per concurrency group and cancels the others.
A `library-pages` group around the whole publish workflow would therefore drop events
when three arrive during one deploy. So:

- **apply** has no concurrency group. `scripts/apply-event.sh` does fetch, apply, commit,
  push; when the push loses a race it resets to the new `main` and applies again.
  Every step is a function of (main, event), so re-applying is correct.
- **deploy** sits in `library-pages` with `cancel-in-progress: false`. It deploys whatever
  `main` holds when it starts, so when a pending deploy is replaced by a newer one, the
  newer one carries both changes.
- **notify** polls the live `library.json` until its `generated` timestamp is at or after
  the event's `applied_at`. Whichever run's deploy made it live, our change is in it.

Intake runs use a per-issue group with `cancel-in-progress: true`: if someone edits an
issue twice quickly, only the last edit matters.

## Loans

The ledger is a JSON array in git. Records are appended and later closed by setting
`returned_at`; nothing is deleted, so `git log -p ledger/ledger.json` is the audit trail.
`Ledger.Save` writes a temp file and renames it. Rules live in one place
(`internal/ledger`) and every refusal is a typed error (`ErrLimit`, `ErrBorrowed`,
`ErrNoItem`, ...) that the commands turn into exit status 3 and a JSON reason the
comment templates print.

Phase continuation is enforced at publish time: a phase N request for
`source/template` checks for an open loan on any phase N-1 item of the same pair. If one
exists, only its borrower may publish; publishing closes the loan and records
`supersedes` in the new item's `config.json`.

## What is deliberately not here

- No database, no server, no queue. Git is the database; GitHub's event delivery is the queue.
- No JavaScript framework. The form page has about 100 lines of plain JS; the library has
  a 30-line table filter and vendored htmx (0BSD, checksummed) for fragment refresh.
- No YAML or Markdown libraries. The subsets we need are small and their parsers are tested.
- No reflection beyond `text/template`'s own.
