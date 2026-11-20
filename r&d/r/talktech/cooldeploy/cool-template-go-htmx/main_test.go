package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func read(t *testing.T, resp *http.Response, err error) (int, string) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	return read(t, resp, err)
}

func post(t *testing.T, url, text string) (int, string) {
	t.Helper()
	resp, err := http.PostForm(url, map[string][]string{"text": {text}})
	return read(t, resp, err)
}

func TestPageAndFragment(t *testing.T) {
	s, err := newServer()
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.routes())
	defer srv.Close()

	if code, body := get(t, srv.URL+"/healthz"); code != 200 || body != "ok\n" {
		t.Errorf("/healthz = %d %q", code, body)
	}
	code, page := get(t, srv.URL+"/")
	if code != 200 || !strings.Contains(page, "<!doctype html>") || !strings.Contains(page, `hx-post="/notes"`) {
		t.Errorf("page = %d:\n%s", code, page)
	}

	// A post answers with the list fragment only, with the new note in it.
	code, frag := post(t, srv.URL+"/notes", "<b>hello</b>")
	if code != 200 || strings.Contains(frag, "<!doctype") || !strings.Contains(frag, "&lt;b&gt;hello&lt;/b&gt;") {
		t.Errorf("fragment = %d:\n%s", code, frag)
	}
	if _, frag := post(t, srv.URL+"/notes", "  "); !strings.Contains(frag, "Write something first") {
		t.Errorf("an empty note should be refused:\n%s", frag)
	}
	if code, _ := get(t, srv.URL+"/static/htmx.min.js"); code != 200 {
		t.Errorf("htmx is not served: %d", code)
	}
	if _, m := get(t, srv.URL+"/metrics"); !strings.Contains(m, `route="POST /notes"`) {
		t.Errorf("/metrics does not count the posts:\n%s", m)
	}
}
