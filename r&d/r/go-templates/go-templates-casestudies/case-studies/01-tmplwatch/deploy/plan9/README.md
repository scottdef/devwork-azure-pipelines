# tmplwatch on Plan 9

tmplwatch cross-compiles for `plan9/amd64` (`make cross`, or `mk cross` on
Plan 9 with a Go toolchain). On Plan 9 it behaves the same, with three
differences.

## 1. Use the dir source

The dir source has no dependencies beyond the file system, which is the
Plan 9 way to reach anything. Point it at any tree: a local directory, a
`git/fs` checkout of the kv repository, or a 9P server bound into the
namespace.

```
% cat /usr/glenda/lib/tmplwatch.json
{
  "source": {"type": "dir", "path": "/usr/glenda/kv", "interval": "10s"},
  "templates": [
    {"source": "/usr/glenda/lib/tmplwatch/motd.txt.tmpl", "destination": "/lib/motd"}
  ],
  "listen": "127.0.0.1:9180"
}
% tmplwatch -c /usr/glenda/lib/tmplwatch.json -once -dry
% tmplwatch -c /usr/glenda/lib/tmplwatch.json &
```

To follow the kv repository, refresh a checkout from cron and let
tmplwatch poll it:

```
% git/clone https://github.com/CoolGitOrg/tmplwatch-kv /usr/glenda/kv-repo
% bind /usr/glenda/kv-repo/kv /usr/glenda/kv
# cron: cd /usr/glenda/kv-repo && git/pull
```

The `github` source also works where `/net` reaches `api.github.com` and
`/sys/lib/tls/ca.pem` holds the CA roots. Set `GITHUB_TOKEN` in the
environment (`GITHUB_TOKEN=... tmplwatch ...` in rc).

## 2. Notes, not signals

Plan 9 has no signals. Processes receive *notes*, which are strings
written to `/proc/PID/note`. Go's `os.Process.Signal` on Plan 9 does
exactly that with `syscall.Note`, and `internal/runner/signal_plan9.go`
maps the Unix names:

| Config / Unix | Note posted | Effect on tmplwatch itself |
|---|---|---|
| `SIGHUP` | `hangup` | reload templates |
| `SIGINT`, `SIGTERM`, `SIGQUIT` | `interrupt` | clean shutdown |
| `SIGKILL` | `kill` | (cannot be caught) |
| anything else | the text itself | |

From rc:

```
% echo hangup >/proc/`{ps | grep tmplwatch | awk '{print $2}'}/note    # reload
% echo interrupt >/proc/$pid/note                                      # stop
```

With `exec.change_signal: "SIGHUP"` the supervised child receives the
`hangup` note. Go programs see it through `os/signal` as
`syscall.Note("hangup")`. Plain Plan 9 programs see it in their
`notify(2)` handler.

## 3. Rename is remove-then-wstat

`WriteAtomic` creates the temporary file in the destination's directory,
writes and syncs it, sets its mode, and then renames it. On Plan 9 a rename
is a `wstat` that changes the name, and `wstat` cannot replace an existing
file, so Go's `os.Rename` **removes the destination first**. A program that
opens the destination at that instant gets "file does not exist". It never
sees a partially written file. Programs that re-read their config on a note
(the usual pattern) are unaffected, because tmplwatch posts the note after
the rename.

Go's Plan 9 rename also refuses a new name that does not share the old
name's literal directory prefix; tmplwatch always renames within one
directory, spelled the same way. The directory fsync, which Unix needs for
durability, fails harmlessly and is ignored. Modes from `perms` map to
Plan 9 permission bits as usual.

## Building with mk

```
% cd tmplwatch
% mk            # vet, test, build into bin/
% mk once       # render testdata with -once -dry
% mk cross      # every target into dist/
```

`mkfile` sets `GOFLAGS=-mod=mod` and `GOPROXY=off`; tmplwatch has no
dependencies to fetch.

## Not available on Plan 9

* `-race` (the race detector needs cgo).
* The systemd unit and Dockerfile, obviously. Start tmplwatch from
  `/cfg/$sysname/cpustart` or a user's `lib/profile`.
