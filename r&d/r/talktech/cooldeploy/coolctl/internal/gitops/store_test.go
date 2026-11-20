package gitops

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestDirStore(t *testing.T) {
	ctx := context.Background()
	s := NewDirStore(t.TempDir())

	if _, _, err := s.Read(ctx, "apps/a/b/app.json"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read of a missing file: %v", err)
	}
	app := App{Team: "payments", App: "checkout", Repo: "CoolEngOrg/checkout", Chart: "cool-app-1.0.0"}
	if err := WriteJSON(ctx, s, AppPath("payments", "checkout"), app, "", "register"); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(ctx, s, AppPath("payments", "checkout"), app, "", "again"); !errors.Is(err, ErrConflict) {
		t.Fatalf("create over an existing file: %v, want ErrConflict", err)
	}
	var got App
	rev, err := ReadJSON(ctx, s, AppPath("payments", "checkout"), &got)
	if err != nil || !reflect.DeepEqual(got, app) {
		t.Fatalf("read back %+v, %v", got, err)
	}
	app.Chart = "cool-app-1.1.0"
	if err := WriteJSON(ctx, s, AppPath("payments", "checkout"), app, "stale", "update"); !errors.Is(err, ErrConflict) {
		t.Fatalf("write with a stale rev: %v, want ErrConflict", err)
	}
	if err := WriteJSON(ctx, s, AppPath("payments", "checkout"), app, rev, "update"); err != nil {
		t.Fatal(err)
	}

	apps, err := Apps(ctx, s)
	if err != nil || len(apps) != 1 || apps[0].Chart != "cool-app-1.1.0" {
		t.Fatalf("Apps = %+v, %v", apps, err)
	}
	names, _ := s.List(ctx, "apps/payments")
	if strings.Join(names, " ") != "checkout/" {
		t.Errorf("List = %v", names)
	}

	_, rev, _ = s.Read(ctx, AppPath("payments", "checkout"))
	if err := s.Delete(ctx, AppPath("payments", "checkout"), rev, "retire"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Root + "/apps"); !os.IsNotExist(err) {
		t.Error("delete left empty directories behind")
	}
}

func TestDirStoreRejectsEscapes(t *testing.T) {
	s := NewDirStore(t.TempDir())
	for _, p := range []string{"../x", "/etc/passwd", "a//b", ""} {
		if err := s.Write(context.Background(), p, []byte("x"), "", "m"); err == nil {
			t.Errorf("Write(%q) was accepted", p)
		}
	}
}

func TestDirStoreCommits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init: %v: %s", err, out)
	}
	s := NewDirStore(dir)
	if !s.Git {
		t.Fatal("a git work tree was not detected")
	}
	ctx := context.Background()
	if err := s.Write(ctx, "a/b.json", []byte("{}\n"), "", "deploy: first"); err != nil {
		t.Fatal(err)
	}
	_, rev, _ := s.Read(ctx, "a/b.json")
	if err := s.Delete(ctx, "a/b.json", rev, "retire: first"); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", dir, "log", "--format=%s").Output()
	if err != nil || strings.TrimSpace(string(out)) != "retire: first\ndeploy: first" {
		t.Errorf("log = %q, %v", out, err)
	}
}
