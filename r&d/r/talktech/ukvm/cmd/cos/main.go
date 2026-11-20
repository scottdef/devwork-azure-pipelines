// Command cos is the chief of staff for a voice-driven universal KVM.
//
//	cos serve          run the daemon (chief of staff, or a satellite)
//	cos order TEXT     give a written order to a running daemon
//	cos listen         read microphone PCM on stdin and submit utterances
//	cos tools          print the tool schema for the Needle model
//	cos pdf FILE       convert a PDF to text on standard output
//	cos ledger-verify FILE   check an audit ledger's hash chain
//
// One static binary, standard library only.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"ukvm/internal/broker"
	"ukvm/internal/intake"
	"ukvm/internal/ledger"
	"ukvm/internal/order"
	"ukvm/internal/staff"
)

var version = "0.1.0-prototype"

type tether struct {
	Hands string `json:"hands"` // "ch9329:/dev/ukvm/hands-server1", "hidg:/dev/hidg0,/dev/hidg1" or "sim:/tmp/wire.bin"
	Eyes  string `json:"eyes"`  // "v4l2:/dev/ukvm/eyes-server1" or "file:/path/to/frame.png"
}

type config struct {
	Self         string            `json:"self"`
	DocRoot      string            `json:"doc_root"`
	SettleMS     int               `json:"settle_ms"`
	VoiceConfirm bool              `json:"voice_confirm"`
	Policy       order.Policy      `json:"policy"`
	Nodes        []broker.Node     `json:"nodes"`
	Tethers      map[string]tether `json:"tethers"`
	Cactus       *struct {
		Bin     string `json:"bin"`
		Needle  string `json:"needle"`
		Whistle string `json:"whistle"`
		Tools   string `json:"tools"`
	} `json:"cactus"`
}

func loadConfig(path string) (config, error) {
	var c config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields() // a misspelt policy key must not silently vanish
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if c.Self == "" {
		return c, fmt.Errorf("%s: \"self\" is required", path)
	}
	return c, nil
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("cos: ")
	if len(os.Args) < 2 {
		usage()
	}
	args := os.Args[2:]
	switch os.Args[1] {
	case "serve":
		serve(args)
	case "order":
		sendOrder(args)
	case "listen":
		listen(args)
	case "tools":
		tools(args)
	case "pdf":
		pdf(args)
	case "ledger-verify":
		verify(args)
	case "version":
		fmt.Println(version)
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: cos serve|order|listen|tools|pdf|ledger-verify|version [flags]")
	os.Exit(2)
}

func token() string {
	t := os.Getenv("COS_TOKEN")
	if len(t) < 24 {
		log.Fatal("COS_TOKEN must be set to at least 24 characters")
	}
	return t
}

func serve(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cfgPath := fs.String("config", "/etc/cos/cos.json", "configuration file")
	listenAddr := fs.String("listen", "127.0.0.1:8088", "API address (token required)")
	panelAddr := fs.String("panel", "", "front-panel address, e.g. 127.0.0.1:8089 (no token: bind it privately)")
	state := fs.String("state", "/var/lib/cos", "state directory")
	armedFile := fs.String("armed-file", "/run/cos/armed", "actions are performed only while this file exists")
	cert := fs.String("tls-cert", "", "serve the API over TLS with this certificate")
	key := fs.String("tls-key", "", "private key for -tls-cert")
	clientCA := fs.String("client-ca", "", "require API clients to present a certificate signed by this CA")
	fs.Parse(args)

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	tok := token()
	if err := os.MkdirAll(*state, 0o750); err != nil {
		log.Fatal(err)
	}
	led, err := ledger.Open(*state + "/audit.jsonl")
	if err != nil {
		log.Fatal(err)
	}
	reg, err := broker.New(cfg.Nodes)
	if err != nil {
		log.Fatal(err)
	}

	deps := staff.Deps{
		Policy: cfg.Policy, Reg: reg, Ledger: led,
		Tools: intake.Tools{Run: intake.Exec},
		Hands: map[string]staff.Hands{}, Eyes: map[string]staff.Eyes{},
		Armed: func() bool { _, err := os.Stat(*armedFile); return err == nil },
	}
	for target, t := range cfg.Tethers {
		kind, arg, _ := strings.Cut(t.Hands, ":")
		switch kind {
		case "ch9329":
			deps.Hands[target] = staff.SerialHands{Dev: arg}
		case "hidg":
			kbd, ptr, _ := strings.Cut(arg, ",")
			deps.Hands[target] = staff.GadgetHands{Kbd: kbd, Ptr: ptr}
		case "sim":
			// Simulator: append the exact CH9329 byte stream to a file
			// instead of a serial port, so it can be inspected with od or xxd.
			deps.Hands[target] = staff.SerialHands{Dev: arg, Gap: time.Microsecond, Open: func(dev string) (io.WriteCloser, error) {
				return os.OpenFile(dev, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o640)
			}}
		case "":
		default:
			log.Fatalf("tether %s: unknown hands kind %q", target, kind)
		}
		kind, arg, _ = strings.Cut(t.Eyes, ":")
		switch kind {
		case "v4l2":
			deps.Eyes[target] = staff.CaptureEyes{Dev: arg}
		case "file":
			deps.Eyes[target] = staff.FileEyes{Path: arg}
		case "":
		default:
			log.Fatalf("tether %s: unknown eyes kind %q", target, kind)
		}
	}
	if c := cfg.Cactus; c != nil {
		engine := intake.Cactus{Bin: c.Bin, Needle: c.Needle, Whistle: c.Whistle, Tools: c.Tools, Run: intake.Exec}
		deps.Hear = engine.Hear
	}
	chief, err := staff.New(staff.Config{
		Self: cfg.Self, StateDir: *state, DocRoot: cfg.DocRoot, Token: tok,
		Settle: time.Duration(cfg.SettleMS) * time.Millisecond, VoiceConfirm: cfg.VoiceConfirm,
	}, deps)
	if err != nil {
		log.Fatal(err)
	}
	led.Append("start", map[string]any{"version": version, "self": cfg.Self})

	go probe(reg, cfg.Self)

	if *panelAddr != "" {
		go func() { log.Fatal(http.ListenAndServe(*panelAddr, chief.Panel())) }()
		log.Printf("front panel on http://%s", *panelAddr)
	}
	srv := &http.Server{Addr: *listenAddr, Handler: chief.API(tok), ReadHeaderTimeout: 10 * time.Second}
	if *cert == "" {
		log.Printf("%s serving API on http://%s (state %s)", cfg.Self, *listenAddr, *state)
		log.Fatal(srv.ListenAndServe())
	}
	srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS13}
	if *clientCA != "" {
		pem, err := os.ReadFile(*clientCA)
		if err != nil {
			log.Fatal(err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			log.Fatalf("%s holds no certificates", *clientCA)
		}
		srv.TLSConfig.ClientCAs, srv.TLSConfig.ClientAuth = pool, tls.RequireAndVerifyClientCert
	}
	log.Printf("%s serving API on https://%s (state %s)", cfg.Self, *listenAddr, *state)
	log.Fatal(srv.ListenAndServeTLS(*cert, *key))
}

// probe is the monitoring loop: every node that answers on its API port, or
// on SSH for an agentless lender, is marked as seen.
func probe(reg *broker.Registry, self string) {
	for {
		for _, n := range reg.Snapshot() {
			if n.Name == self {
				continue
			}
			addr := n.Addr
			if addr == "" && n.SSH != "" {
				host := n.SSH[strings.LastIndex(n.SSH, "@")+1:]
				addr = net.JoinHostPort(host, "22")
			}
			if addr == "" {
				continue
			}
			addr = strings.TrimPrefix(strings.TrimPrefix(addr, "https://"), "http://")
			if conn, err := net.DialTimeout("tcp", addr, 2*time.Second); err == nil {
				conn.Close()
				reg.Touch(n.Name, time.Now())
			}
		}
		time.Sleep(10 * time.Second)
	}
}

func post(addr, path, contentType string, body []byte) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(addr, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token())
	req.Header.Set("Content-Type", contentType)
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return b, resp.StatusCode, err
}

func sendOrder(args []string) {
	fs := flag.NewFlagSet("order", flag.ExitOnError)
	addr := fs.String("addr", "http://127.0.0.1:8088", "daemon API address")
	source := fs.String("source", "text", "where the order came from (for the ledger)")
	fs.Parse(args)
	text := strings.Join(fs.Args(), " ")
	if text == "" {
		log.Fatal(`usage: cos order "target server3; run uptime"`)
	}
	body, code, err := post(*addr, "/v1/order?source="+*source, "text/plain", []byte(text))
	if err != nil {
		log.Fatal(err)
	}
	var job staff.Job
	json.Unmarshal(body, &job)
	if code != 200 || job.ID == "" {
		log.Fatalf("HTTP %d: %s", code, strings.TrimSpace(string(body)))
	}
	fmt.Printf("%s %s", job.ID, job.State)
	if job.Reason != "" {
		fmt.Printf(" (%s)", job.Reason)
	}
	fmt.Println()
	for _, s := range job.Steps {
		fmt.Printf("  %s on %s\n", s.Call, s.Node)
		for _, line := range strings.Split(strings.TrimRight(s.Output, "\n"), "\n") {
			if line != "" {
				fmt.Printf("    %s\n", line)
			}
		}
		if s.Err != "" {
			fmt.Printf("    ERROR: %s\n", s.Err)
		}
	}
	if job.State == staff.StateFailed || job.State == staff.StateRefused {
		os.Exit(1)
	}
}

// listen turns a microphone pipe into orders:
//
//	arecord -q -t raw -f S16_LE -r 16000 -c 1 | cos listen
func listen(args []string) {
	fs := flag.NewFlagSet("listen", flag.ExitOnError)
	addr := fs.String("addr", "http://127.0.0.1:8088", "daemon API address")
	fs.Parse(args)
	var seg intake.Segmenter
	frame := make([]int16, 320)
	for {
		if err := binary.Read(os.Stdin, binary.LittleEndian, frame); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return
			}
			log.Fatal(err)
		}
		for _, utt := range seg.Write(frame) {
			var pcm bytes.Buffer
			binary.Write(&pcm, binary.LittleEndian, utt)
			body, code, err := post(*addr, "/v1/utterance", "audio/L16;rate=16000;channels=1", pcm.Bytes())
			if err != nil {
				log.Print(err)
				continue
			}
			fmt.Printf("%d %s\n", code, strings.TrimSpace(string(body)))
		}
	}
}

func tools(args []string) {
	fs := flag.NewFlagSet("tools", flag.ExitOnError)
	cfgPath := fs.String("config", "/etc/cos/cos.json", "configuration file")
	fs.Parse(args)
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	b, err := order.ToolsJSON(cfg.Policy)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))
}

func pdf(args []string) {
	if len(args) != 1 {
		log.Fatal("usage: cos pdf FILE")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	txt, err := intake.Tools{Run: intake.Exec}.PDFText(ctx, args[0])
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(txt)
}

func verify(args []string) {
	if len(args) != 1 {
		log.Fatal("usage: cos ledger-verify FILE")
	}
	f, err := os.Open(args[0])
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	n, head, err := ledger.Verify(f)
	if err != nil {
		log.Fatalf("ledger is NOT intact after record %d: %v", n, err)
	}
	fmt.Printf("ok: %d records, head %s\n", n, head)
}
