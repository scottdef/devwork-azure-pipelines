# Design

tmplwatch is one loop: poll, diff, render what changed, write what differs,
then react. This document explains each step and the choices behind it.

## Lifecycle

1. **Start.** Load and validate the JSON config, which rejects unknown fields
   and bad modes or delimiters. Parse every template; a template that does
   not parse stops startup, because running half-configured is worse than not
   running. Remove temporary files that an earlier crash left beside each
   destination (`.NAME.tmplwatch-*`). Start the HTTP listener.
2. **Poll.** `source.Watch` calls `Fetch` at once and then every `interval`.
   A source reports `changed=false` when it knows nothing moved: the GitHub
   source on a 304, the dir source when no name, size or mtime changed. An
   error is logged and counted. A rate-limit error stretches the next wait
   to the reset time.
3. **Diff.** `brain.Update` installs the snapshot and returns the sorted
   keys whose presence or value changed. On the first snapshot every key has
   changed.
4. **Select.** A template is *dirty* if it has never rendered, or if one of
   its recorded dependencies is touched (below).
5. **Render.** The parsed template is cloned and given a FuncMap bound to a
   *recording store*, then executed into a `bytes.Buffer`. Rendering never
   touches the file system except through `file`.
6. **Decide.**
   * Execution error (bad JSON, `fail`, `missingkey=error`): log, count,
     write nothing, and `/healthz` returns 503.
   * Missing key (`key` on an absent path): the render is **incomplete**.
     Write nothing and keep the old file. `/healthz` returns 503 and
     `/status` lists the missing keys. The missing key is still a dependency,
     so its arrival triggers the next render. consul-template blocks in the
     same situation.
   * Complete: go on.
7. **Write.** `WriteAtomic`; see below. If the bytes equal the file on disk
   (SHA-256) nothing happens: no write, no command, no signal.
8. **React.** For each template written, schedule its `command` after
   `wait` + random(`splay`). Once all templates have a complete render, start
   the `exec` child. Later writes send it `change_signal`, or restart it when
   no signal is configured.
9. **Reload** (SIGHUP; the `hangup` note on Plan 9). Re-read every template
   file and render all of them. A template that no longer parses keeps its
   previous version and the error is logged.
10. **Stop** (SIGINT/SIGTERM; the `interrupt` note on Plan 9). Cancel the
    context, stop pending command timers, send the child `kill_signal`, wait
    `kill_timeout`, then kill it. Finally shut down the HTTP server.

`-once` runs steps 2 through 8 a single time, with commands run
synchronously and no wait or splay, and maps the outcome to an exit code.
`-dry` replaces step 7 with a unified diff on stdout and skips step 8.

## Dependency tracking

consul-template learns a template's dependencies by executing it: each
function call that reads Consul registers a watch. tmplwatch does the same
without the watches. The recording store notes:

| Call | Recorded as | Re-render when |
|---|---|---|
| `key "a/b"`, `keyOrDefault "a/b" _`, `keyExists "a/b"` | key `a/b` | `a/b` is added, removed or changed |
| `ls "a"`, `tree "a"` | prefix `a/` | any key under `a/` changes |

`keyOrDefault` and `keyExists` record the key even when it is absent, which
is the point: the key's arrival must cause a re-render.

The recorded set is what the **last** render read. A template such as
`{{ if keyExists "x" }}{{ key "y" }}{{ end }}` depends on `y` only while
`x` exists, and that is correct: when `x` disappears the template is
re-rendered (it depends on `x`), and from then on it no longer depends on
`y`.

`ls` and `tree` share one prefix dependency. `ls` could in theory ignore
changes deeper than one level. Being conservative costs one extra render of
a cheap template, and being wrong would cost a stale file, so tmplwatch is
conservative.

The cost model: rendering is microseconds (see `/metrics`
`tmplwatch_render_duration_seconds`). Tracking exists to avoid needless
writes and **commands**, not to save CPU. Even without it, the SHA-256
comparison before writing would stop needless reloads. With it, a change to
`hosts/web-3` does not even render the WireGuard config.

### Why clone per render

`text/template` binds functions at parse time. To give each render its own
recording store, the parsed template is `Clone`d and `Funcs` rebinds the
names. Clone is cheap (it copies the template map, not the parse trees).
One gotcha cost a test failure: **`Clone` does not copy `Option` settings**
(Go 1.24), so `missingkey=error` must be set again on the clone.

## Atomic write

```
same := sha256(old) == sha256(new)       → if same: fix mode drift, return
f := CreateTemp(dir(dest), ".NAME.tmplwatch-*")   same file system as dest
write; f.Sync(); f.Chmod(perm); f.Close()
if backup: same dance for dest.bak with the old bytes
rename(f, dest)                          atomic on POSIX (Plan 9: see below)
sync(dir(dest))                          the rename survives power loss
```

Readers see the old file or the new file, never a truncated one. A process
holding the old file open keeps reading the old inode; `TestWriteAtomic`
checks exactly that. Any failure before the rename removes the temp file
and leaves `dest` untouched (`TestWriteAtomicCrash` injects an fsync
failure). A crash between create and rename leaves a dot-file that the next
start removes. `CleanStale` matches only our own prefix.

The mode is set on the temp file before the rename, so the destination is
never briefly world-readable. That matters for `wg0.conf` at `0600`.

**Plan 9 caveat.** Plan 9 renames with `wstat`, which can change a name
within a directory but cannot replace an existing file. Go's `os.Rename`
therefore removes the destination and then renames the temp file. A reader
can briefly find **no** file, but never a partial one. Go also requires
the two names to share a literal directory prefix, so `WriteAtomic`
renames to the temp file's own directory prefix plus the base name, the spelling `CreateTemp`
used (`TestWriteAtomicRelative`).

## Why JSON

The standard library parses JSON and nothing else that is suitable for
configuration. HCL would mean `github.com/hashicorp/hcl/v2` and its
dependency tree, which breaks the build convention (stdlib only, no module
network). JSON is verbose but unambiguous. `DisallowUnknownFields` turns the
usual JSON weakness, typos silently ignored, into an error. Durations are
strings (`"15s"`) parsed with `time.ParseDuration`.

## Why polling, not webhooks

A webhook needs an inbound HTTPS endpoint reachable from GitHub on every
host that runs tmplwatch. That means a public listener, TLS certificates,
signature verification and a firewall hole, on machines whose whole job is
to hold configuration. Polling needs one outbound HTTPS connection.

Webhooks also lose events (delivery failures, downtime) and need a polling
fallback anyway. Polling with ETags is self-healing: after any outage, the
next poll converges.

Latency is bounded by `interval` (15s by default). Configuration changes go
through pull-request review, which takes minutes, so 15 seconds is noise.
Where a faster signal matters, the kv repository's `notify.yml` already
fires a `repository_dispatch`, and a future version could expose a
loopback `/poke` to cut the wait.

## API budget

One poll is one conditional `GET /repos/{o}/{r}/git/trees/{ref}?recursive=1`.

* Every 15s is 4 per minute, **240 per hour** per host.
* GitHub does not count a `304 Not Modified` answer to a conditional request
  against the primary rate limit, as long as the request is authenticated.
  So on an idle repository the steady-state cost is **zero**.
* A changed tree costs 1 request, plus 1 per **new** blob. Blobs are content
  addressed, so values that did not change are served from the cache.
* Budgets: an installation token gets at least 5,000 requests per hour
  (more on Enterprise Cloud), and a personal token 5,000. Even if every
  poll were a 200, 240 per hour per host is under 5%. Twenty hosts sharing
  one App installation would use 4,800 per hour **only** if the repository
  changed every 15 seconds, all day.
* Anonymous access (60 per hour) cannot sustain a 15s interval. tmplwatch
  warns at startup when no token is configured.

The git trees API was chosen over the contents API because one request
returns the whole tree with blob SHAs. The contents API needs one request
per directory, and its ETags change with every commit even when the prefix
did not.

A truncated tree (over 100,000 entries or 7 MB) is an error, not a partial
snapshot: rendering against half the keys would be worse than not
rendering.

## GitHub App tokens without a JWT library

`internal/ghapp` is about 150 lines. A GitHub App JWT is
`base64url(header).base64url(claims).base64url(RSASSA-PKCS1-v1_5-SHA256(…))`.
`crypto/rsa.SignPKCS1v15` does the signing. The claims are `iat` = now−60s
(clock skew), `exp` = now+9m (GitHub's ceiling is 10), and `iss` = the App's
client ID. The minter finds the installation for the repository once, mints
tokens scoped to that single repository with `contents: read`, and refreshes
five minutes before expiry. The test verifies the signature with the public
key and runs the full exchange against an `httptest` server.

## Plan 9

Plan 9 has notes, not signals. `os.Process.Signal` on Plan 9 writes
`sig.String()` to `/proc/PID/note`, so `syscall.Note("hangup")` *is* the
mechanism, with nothing further to implement. `signal_plan9.go` maps the
Unix names onto notes. `signal_unix.go` keeps `syscall.SIGHUP` and friends
behind `//go:build !plan9`, which is the only reason the binary
cross-compiles. The GitHub source works on Plan 9 if the system has
`/net` and TLS roots. The dir source is the native way: bind a
`git/fs` or 9P-served tree and poll it.

## What is deliberately missing

* Consul, Vault and Nomad clients: they are the dependency tree this
  project exists to avoid.
* Template-to-template dependencies (`{{ template "x" }}` across files):
  each file is self-contained.
* De-duplication across hosts (consul-template's leader-elected render):
  the GitHub ETag already makes polls nearly free.
