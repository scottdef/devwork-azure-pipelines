package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// repo is the repository root, so configs can name templates/ and testdata/.
const repo = "../.."

func writeConfig(t *testing.T, dir, kv, listen string, extra string) string {
	t.Helper()
	abs := func(p string) string { a, _ := filepath.Abs(filepath.Join(repo, p)); return a }
	cfg := fmt.Sprintf(`{
  "source": {"type": "dir", "path": %q, "interval": "1s"},
  "templates": [
    {"source": %q, "destination": %q, "error_on_missing_key": true},
    {"source": %q, "destination": %q, "perms": "0600"%s}
  ],
  "listen": %q
}`, kv, abs("templates/hosts.tmpl"), filepath.Join(dir, "out/hosts"),
		abs("templates/wg0.conf.tmpl"), filepath.Join(dir, "out/wg0.conf"), extra, listen)
	p := filepath.Join(dir, "tmplwatch.json")
	if err := os.WriteFile(p, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOnceAndDry(t *testing.T) {
	dir := t.TempDir()
	kv, _ := filepath.Abs(filepath.Join(repo, "testdata/kv"))
	cfg := writeConfig(t, dir, kv, "off", "")
	var out, errb bytes.Buffer

	if code := run([]string{"-c", cfg, "-once", "-dry"}, &out, &errb); code != exitOK {
		t.Fatalf("-once -dry exit %d\n%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "+10.0.1.10\tweb-1.example.internal web-1") {
		t.Errorf("-dry diff missing:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "out/hosts")); !os.IsNotExist(err) {
		t.Fatal("-dry wrote a file")
	}

	out.Reset()
	if code := run([]string{"-c", cfg, "-once"}, &out, &errb); code != exitOK {
		t.Fatalf("-once exit %d\n%s", code, errb.String())
	}
	want, _ := os.ReadFile(filepath.Join(repo, "testdata/golden/hosts"))
	got, _ := os.ReadFile(filepath.Join(dir, "out/hosts"))
	if !bytes.Equal(got, want) {
		t.Errorf("hosts differs from golden")
	}
	if fi, _ := os.Stat(filepath.Join(dir, "out/wg0.conf")); fi == nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("wg0.conf mode %v", fi)
	}

	// Second -dry against written files: nothing to show.
	out.Reset()
	run([]string{"-c", cfg, "-once", "-dry"}, &out, &errb)
	if out.Len() != 0 {
		t.Errorf("-dry after write shows changes:\n%s", out.String())
	}
}

func TestOnceExitCodes(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "emptykv")
	os.MkdirAll(empty, 0o755)
	cfg := writeConfig(t, dir, empty, "off", "")
	var out, errb bytes.Buffer
	// hosts uses error_on_missing_key... but key() on an absent key marks
	// the render incomplete, which is exit 3.
	if code := run([]string{"-c", cfg, "-once"}, &out, &errb); code != exitIncomplete {
		t.Errorf("empty kv: exit %d, want %d\n%s", code, exitIncomplete, errb.String())
	}
	if code := run([]string{"-c", filepath.Join(dir, "nope.json")}, &out, &errb); code != exitUsage {
		t.Errorf("missing config: exit %d", code)
	}
	if code := run([]string{"-c", cfg, "-once", "-a", "0.0.0.0:0", "-pprof"}, &out, &errb); code != exitUsage {
		t.Errorf("-pprof on non-loopback: exit %d", code)
	}
	// A failing command under -once is exit 1.
	cfg = writeConfig(t, dir, func() string { a, _ := filepath.Abs(filepath.Join(repo, "testdata/kv")); return a }(), "off", `, "command": ["false"]`)
	if _, err := os.Stat("/bin/false"); err == nil {
		if code := run([]string{"-c", cfg, "-once"}, &out, &errb); code != exitRender {
			t.Errorf("failing command: exit %d\n%s", code, errb.String())
		}
	}
	out.Reset()
	if code := run([]string{"-version"}, &out, &errb); code != 0 || !strings.HasPrefix(out.String(), "tmplwatch ") {
		t.Errorf("-version: %d %q", code, out.String())
	}
}

func TestStaticToken(t *testing.T) {
	f := filepath.Join(t.TempDir(), "tok")
	os.WriteFile(f, []byte("ghs_file\n"), 0o600)
	t.Setenv("TW_TEST_TOKEN", "ghs_env")
	if tok, _ := staticToken("TW_TEST_TOKEN")(context.Background()); tok != "ghs_env" {
		t.Errorf("env token %q", tok)
	}
	t.Setenv("TW_TEST_TOKEN_FILE", f)
	if tok, _ := staticToken("TW_TEST_TOKEN")(context.Background()); tok != "ghs_file" {
		t.Errorf("file token %q", tok)
	}
}

func freeAddr(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

// TestDaemon runs the real loop: start, serve, see a KV edit, shut down.
func TestDaemon(t *testing.T) {
	dir := t.TempDir()
	kv := filepath.Join(dir, "kv")
	src, _ := filepath.Abs(filepath.Join(repo, "testdata/kv"))
	if err := os.CopyFS(kv, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	addr := freeAddr(t)
	cfg := writeConfig(t, dir, kv, addr, "")
	done := make(chan int)
	go func() { done <- run([]string{"-c", cfg}, io.Discard, io.Discard) }()
	get := func(path string) (int, string) {
		resp, err := http.Get("http://" + addr + path)
		if err != nil {
			return 0, err.Error()
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	wait := func(what string, ok func() bool) {
		t.Helper()
		for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
			if ok() {
				return
			}
		}
		t.Fatalf("timeout: %s", what)
	}
	wait("healthy", func() bool { c, _ := get("/healthz"); return c == 200 })
	if code := healthcheck(addr, io.Discard); code != exitOK {
		t.Errorf("-healthcheck exit %d", code)
	}
	_, body := get("/render?template=hosts")
	if !strings.Contains(body, "web-2.example.internal") {
		t.Fatalf("render: %q", body)
	}
	os.WriteFile(filepath.Join(kv, "hosts/web-3"), []byte("10.0.1.12\n"), 0o644)
	wait("web-3 rendered", func() bool {
		b, _ := os.ReadFile(filepath.Join(dir, "out/hosts"))
		return strings.Contains(string(b), "10.0.1.12\tweb-3")
	})
	_, m := get("/metrics")
	if !strings.Contains(m, `tmplwatch_source_polls_total{result="changed"} 2`) {
		t.Errorf("metrics after change:\n%s", m)
	}
	// Remove a key the hosts template needs: incomplete, 503, old file kept.
	os.Remove(filepath.Join(kv, "site/domain"))
	wait("unhealthy", func() bool { c, _ := get("/healthz"); return c == 503 })
	if b, _ := os.ReadFile(filepath.Join(dir, "out/hosts")); !strings.Contains(string(b), "web-3") {
		t.Error("incomplete render clobbered the file")
	}
	if code := healthcheck(addr, io.Discard); code != exitUnhealthy {
		t.Errorf("-healthcheck while incomplete: %d", code)
	}
	stopSelf(t)
	select {
	case code := <-done:
		if code != exitOK {
			t.Errorf("daemon exit %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("daemon did not stop")
	}
}
