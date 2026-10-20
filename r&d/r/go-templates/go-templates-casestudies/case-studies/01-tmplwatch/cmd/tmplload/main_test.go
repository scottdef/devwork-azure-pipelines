package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoad(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			http.Error(w, "no", 500)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	var out, errb bytes.Buffer
	if code := run([]string{"-url", srv.URL, "-rate", "200", "-duration", "250ms", "-c", "4", "-json"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s %s", code, out.String(), errb.String())
	}
	var rep Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Requests != 50 || rep.Errors != 0 || rep.Status["200"] != 50 || rep.P99 < rep.P50 {
		t.Errorf("report %+v", rep)
	}
	out.Reset()
	if code := run([]string{"-url", srv.URL + "/bad", "-rate", "20", "-duration", "100ms"}, &out, &errb); code != 1 {
		t.Errorf("errors not reflected in exit status: %d\n%s", code, out.String())
	}
	if code := run([]string{"-rate", "1"}, &out, &errb); code != 2 {
		t.Error("missing -url accepted")
	}
}
