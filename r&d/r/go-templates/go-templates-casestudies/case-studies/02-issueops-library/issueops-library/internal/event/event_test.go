package event

import (
	"os"
	"strings"
	"testing"
)

func load(t *testing.T, name string) (*Event, error) {
	t.Helper()
	f, err := os.Open("../../testdata/events/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return Decode(f)
}

func TestFixtures(t *testing.T) {
	e, err := load(t, "publish")
	if err != nil {
		t.Fatal(err)
	}
	if e.ItemID != "payments-api/caddyfile/42" || e.Phase != 1 || e.Requester != "alice" || e.Payload["port"].(interface{ String() string }).String() != "8443" {
		t.Errorf("publish = %+v", e)
	}
	e, err = load(t, "borrow")
	if err != nil || e.Days != 14 || e.ItemID != "payments-api/caddyfile/42" {
		t.Errorf("borrow = %+v, %v", e, err)
	}
	if _, err := load(t, "return"); err != nil {
		t.Error(err)
	}
	if _, err := load(t, "bad-sha"); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Errorf("bad-sha: %v", err)
	}
	if _, err := load(t, "bad-path"); err == nil {
		t.Error("bad-path: want error")
	}
}

func TestDecodeRejects(t *testing.T) {
	base := `"issue":1,"repo":"o/r","requester":"bob"`
	for name, doc := range map[string]string{
		"nojson":    `{`,
		"nopayload": `{"action":"library-return"}`,
		"action":    `{"action":"library-burn","client_payload":{` + base + `}}`,
		"issue0":    `{"action":"library-return","client_payload":{"issue":0,"repo":"o/r","requester":"b","item_id":"a1/readme/1"}}`,
		"repo":      `{"action":"library-return","client_payload":{"issue":1,"repo":"o/r/x","requester":"b","item_id":"a1/readme/1"}}`,
		"login":     `{"action":"library-return","client_payload":{"issue":1,"repo":"o/r","requester":"b c","item_id":"a1/readme/1"}}`,
		"days":      `{"action":"library-borrow","client_payload":{` + base + `,"item_id":"a1/readme/1","days":"30"}}`,
		"template":  `{"action":"library-return","client_payload":{` + base + `,"item_id":"a1/shell/1"}}`,
		"issuezero": `{"action":"library-return","client_payload":{` + base + `,"item_id":"a1/readme/01"}}`,
		"dots":      `{"action":"library-return","client_payload":{` + base + `,"item_id":"../readme/1"}}`,
		"phase":     `{"action":"library-publish","client_payload":{` + base + `,"source_id":"ab","template_id":"readme","phase":"4","payload":"{}","sha256":"44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"}}`,
		"array":     `{"action":"library-publish","client_payload":{` + base + `,"source_id":"ab","template_id":"readme","phase":"1","payload":"[]","sha256":"4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945"}}`,
		"source":    `{"action":"library-publish","client_payload":{` + base + `,"source_id":"A_B","template_id":"readme","phase":"1","payload":"{}","sha256":"44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"}}`,
	} {
		if _, err := Decode(strings.NewReader(doc)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestCleanItemID(t *testing.T) {
	if id, err := CleanItemID(" payments-api/caddyfile/42 "); err != nil || id != "payments-api/caddyfile/42" {
		t.Errorf("got %q %v", id, err)
	}
	for _, bad := range []string{"", "a/b", "ab/readme/1/x", "ab/readme/-1", "ab/readme/1e3", "ab/../1", "/readme/1"} {
		if _, err := CleanItemID(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func FuzzDecode(f *testing.F) {
	for _, n := range []string{"publish", "borrow", "return", "bad-path"} {
		b, _ := os.ReadFile("../../testdata/events/" + n + ".json")
		f.Add(string(b))
	}
	f.Fuzz(func(t *testing.T, doc string) {
		e, err := Decode(strings.NewReader(doc))
		if err != nil {
			return
		}
		if e.ItemID != "" {
			if _, err := CleanItemID(e.ItemID); err != nil {
				t.Fatalf("accepted bad item id %q", e.ItemID)
			}
		}
	})
}
