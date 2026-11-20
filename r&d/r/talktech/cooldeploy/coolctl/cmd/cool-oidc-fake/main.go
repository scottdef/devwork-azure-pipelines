// Cool-oidc-fake stands in for GitHub's OIDC issuer in the local demo.
//
// It signs any claims it is asked to sign. That is the point of a demo and
// the reason it must never run anywhere a real cool-api can reach it: it
// refuses to listen on anything but the loopback interface.
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/CoolEngOrg/coolctl/internal/oidctest"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9911", "loopback address to listen on")
	flag.Parse()

	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		log.Fatal(err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		log.Fatalf("refusing to listen on %s: this issuer signs anything, so it stays on loopback", *addr)
	}
	iss, err := oidctest.New()
	if err != nil {
		log.Fatal(err)
	}
	iss.URL = "http://" + *addr
	log.Printf("DEMO ONLY: fake OIDC issuer at %s", iss.URL)
	srv := &http.Server{Addr: *addr, Handler: iss.Handler(), ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
