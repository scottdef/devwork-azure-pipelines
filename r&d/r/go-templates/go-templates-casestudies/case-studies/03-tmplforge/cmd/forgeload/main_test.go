package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestShootAndSummarize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("HX-Request") != "true" || r.FormValue("template") == "" {
			w.WriteHeader(400)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	var rs []result
	for i := 0; i < 10; i++ {
		rs = append(rs, shoot(srv.Client(), srv.URL, "template=x", ""))
	}
	rep := summarize(rs, srv.URL, 10, time.Second, 1000)
	if rep.Requests != 10 || rep.Codes["200"] != 10 || rep.Success != 100 || !rep.SLOMet || rep.P99MS <= 0 {
		t.Errorf("%+v", rep)
	}
	rep = summarize(append(rs, result{err: true}), srv.URL, 10, time.Second, 1000)
	if rep.SLOMet || rep.Errors != 1 {
		t.Errorf("errors must fail the SLO: %+v", rep)
	}
}
