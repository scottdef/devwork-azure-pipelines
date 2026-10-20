package item

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoolGitOrg/issueops-library/internal/event"
	"github.com/CoolGitOrg/issueops-library/internal/golden"
)

var fixed = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func renderer() *Renderer {
	return &Renderer{TemplateDir: "../../templates/items", MetaDir: "../../templates/meta", Now: func() time.Time { return fixed }}
}

func decode(t *testing.T, name string) *event.Event {
	t.Helper()
	f, err := os.Open("../../testdata/events/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	e, err := event.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// Every item template has a golden artifact, next.tmpl and config.json.
func TestGoldenArtifacts(t *testing.T) {
	for _, name := range []string{"publish", "publish-workflow", "publish-nomad", "publish-terraform", "publish-issueform", "publish-readme"} {
		t.Run(name, func(t *testing.T) {
			e := decode(t, name)
			it, files, err := renderer().Render(e, "")
			if err != nil {
				t.Fatal(err)
			}
			dir := "../../testdata/golden/items/" + e.TemplateID + "/"
			golden.Check(t, dir+it.Artifact, files[it.Artifact])
			golden.Check(t, dir+"next.tmpl", files["next.tmpl"])
			golden.Check(t, dir+"config.json", files["config.json"])
			golden.Check(t, dir+"SHA256SUMS", files["SHA256SUMS"])
		})
	}
}

func TestAllTemplatesCovered(t *testing.T) {
	for _, id := range event.Templates {
		if _, err := os.Stat("../../templates/items/" + id + ".tmpl"); err != nil {
			t.Errorf("template %s: %v", id, err)
		}
		if _, err := os.Stat("../../testdata/golden/items/" + id); err != nil {
			t.Errorf("no golden for %s", id)
		}
	}
}

func TestBundleDeterministic(t *testing.T) {
	e := decode(t, "publish")
	_, a, err := renderer().Render(e, "")
	if err != nil {
		t.Fatal(err)
	}
	_, b, _ := renderer().Render(e, "")
	if !bytes.Equal(a["bundle.tar.gz"], b["bundle.tar.gz"]) {
		t.Fatal("bundle differs between identical renders")
	}
	zr, err := gzip.NewReader(bytes.NewReader(a["bundle.tar.gz"]))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(tr)
		base := strings.TrimPrefix(h.Name, "payments-api_caddyfile_42/")
		if !bytes.Equal(body, a[base]) {
			t.Errorf("%s in bundle differs from item file", h.Name)
		}
		if !h.ModTime.Equal(fixed) {
			t.Errorf("%s mtime %v", h.Name, h.ModTime)
		}
		names = append(names, base)
	}
	if strings.Join(names, " ") != "artifact.caddyfile config.json next.tmpl" {
		t.Errorf("bundle holds %v", names)
	}
}

func TestSums(t *testing.T) {
	got := Sums(map[string][]byte{"b": []byte("abc"), "a": nil, "SHA256SUMS": []byte("x")})
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  a\n" +
		"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad  b\n"
	if string(got) != want {
		t.Errorf("Sums = %q", got)
	}
}

func TestMissingKeyRefused(t *testing.T) {
	e := decode(t, "publish-nomad")
	delete(e.Payload, "image") // nomad-job has no default for image
	if _, _, err := renderer().Render(e, ""); err == nil {
		t.Error("render without image: want error")
	}
	tm, _, err := ParseTemplate("x", "#!tmpl missingkey=default\nimage = {{.image}}\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Execute(tm, map[string]any{}); err == nil || !strings.Contains(err.Error(), "line 1 prints <no value>") {
		t.Errorf("err = %v", err)
	}
}

func TestNextTemplateFinalPhase(t *testing.T) {
	e := decode(t, "publish")
	e.Phase = 3
	_, files, err := renderer().Render(e, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files["next.tmpl"]), "is final") {
		t.Errorf("phase 3 next.tmpl does not say final:\n%s", files["next.tmpl"])
	}
}

func TestWriteLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	r := renderer()
	for _, n := range []string{"publish", "publish-readme"} {
		it, files, err := r.Render(decode(t, n), "")
		if err != nil {
			t.Fatal(err)
		}
		if err := Write(root, it, files); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(root, "junk", "x", "y"), 0o755) // skipped, not fatal
	items, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "docs-portal/readme/47" || len(items[0].Files) != 5 {
		t.Fatalf("loaded %d items: %+v", len(items), items)
	}
	if !Exists(root, "payments-api/caddyfile/42") || Exists(root, "payments-api/caddyfile/43") || Exists(root, "../x/1") {
		t.Error("Exists is wrong")
	}
	if ph, err := PhaseOf(root, "payments-api/caddyfile/42"); err != nil || ph != 1 {
		t.Errorf("PhaseOf = %d, %v", ph, err)
	}
}

func TestHeader(t *testing.T) {
	h, body, err := ParseHeader("#!tmpl ext=nomad.hcl delims=[[,]] missingkey=error\nX")
	if err != nil || h.Ext != "nomad.hcl" || h.Left != "[[" || h.MissingKey != "error" || body != "X" {
		t.Errorf("%+v %q %v", h, body, err)
	}
	for _, bad := range []string{"#!tmpl ext=../x\n", "#!tmpl ext=.x\n", "#!tmpl delims=[[\n", "#!tmpl missingkey=boom\n", "#!tmpl colour=red\n", "#!tmpl ext\n"} {
		if _, _, err := ParseHeader(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
	if h, body, _ := ParseHeader("plain {{.}}"); h.Ext != "txt" || body != "plain {{.}}" {
		t.Error("no-header default wrong")
	}
}
