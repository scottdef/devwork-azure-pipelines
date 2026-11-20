package staff

import (
	"crypto/subtle"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"ukvm/internal/intake"
	"ukvm/internal/order"
)

var sourceRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,15}$`)

// API is the token-protected interface used by the CLI, by microphones and
// other intake devices, by satellites, and by timers.
func (c *Chief) API(token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/order", c.hOrder)
	mux.HandleFunc("POST /v1/utterance", c.hUtterance)
	mux.HandleFunc("GET /v1/status", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"status": c.Report(), "text": c.StatusText()})
	})
	mux.HandleFunc("GET /v1/jobs", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, c.Jobs(50)) })
	mux.HandleFunc("GET /v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if j, ok := c.Job(r.PathValue("id")); ok {
			reply(w, 200, j)
		} else {
			fail(w, 404, "no such job")
		}
	})
	mux.HandleFunc("POST /v1/jobs/{id}/confirm", func(w http.ResponseWriter, r *http.Request) { c.hResolve(w, r, true) })
	mux.HandleFunc("POST /v1/jobs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) { c.hResolve(w, r, false) })
	mux.HandleFunc("POST /v1/nodes/{name}/beat", c.hBeat)
	mux.HandleFunc("POST /v1/act", c.hAct)
	mux.HandleFunc("PUT /v1/runbooks/{name}", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 64<<10))
		if err := c.SaveRunbook(r.PathValue("name"), string(body)); err != nil {
			fail(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]string{"saved": r.PathValue("name"), "next": "approve it before it can run unattended"})
	})
	mux.HandleFunc("POST /v1/runbooks/{name}/approve", func(w http.ResponseWriter, r *http.Request) {
		sum, err := c.ApproveRunbook(r.PathValue("name"))
		if err != nil {
			fail(w, 404, err.Error())
			return
		}
		reply(w, 200, map[string]string{"approved": r.PathValue("name"), "sha256": sum})
	})
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			fail(w, 401, "unauthorized")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (c *Chief) hOrder(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 8<<10))
	text := strings.TrimSpace(string(body))
	source := r.URL.Query().Get("source")
	if !sourceRE.MatchString(source) {
		source = "text"
	}
	calls, err := order.Parse(text)
	if err != nil {
		c.note("rejected", map[string]any{"source": source, "text": text, "error": err.Error()})
		fail(w, 400, err.Error())
		return
	}
	reply(w, 200, c.Submit(r.Context(), source, text, calls, 1.0))
}

// hUtterance accepts one utterance of 16 kHz mono s16le PCM, transcribes it
// and decodes an order from it in a single speech-engine call.
func (c *Chief) hUtterance(w http.ResponseWriter, r *http.Request) {
	if c.Hear == nil {
		fail(w, 501, "no speech engine is configured on this node")
		return
	}
	pcm, _ := io.ReadAll(io.LimitReader(r.Body, intake.Rate*2*30))
	if len(pcm) < intake.Rate*2*3/10 {
		fail(w, 400, "less than 0.3 s of audio")
		return
	}
	f, err := os.CreateTemp("", "cos-utt-*.wav")
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer os.Remove(f.Name())
	f.Write(intake.WAV(pcm))
	f.Close()
	heard, err := c.Hear(r.Context(), f.Name())
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	calls, conf, ok, why := heard.Usable()
	if !ok {
		c.note("heard-dropped", map[string]any{"text": heard.Text, "reason": why})
		reply(w, 200, map[string]string{"state": "dropped", "reason": why, "heard": heard.Text})
		return
	}
	reply(w, 200, c.Submit(r.Context(), "voice", heard.Text, calls, conf))
}

func (c *Chief) hResolve(w http.ResponseWriter, r *http.Request, approve bool) {
	j, err := c.Resolve(r.Context(), r.PathValue("id"), approve, "api")
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	reply(w, 200, j)
}

func (c *Chief) hBeat(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Load      float64 `json:"load"`
		MemFreeMB int     `json:"mem_free_mb"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&b); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err := c.Reg.Beat(r.PathValue("name"), b.Load, b.MemFreeMB, c.Now()); err != nil {
		fail(w, 404, err.Error())
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}

func (c *Chief) hAct(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Target string     `json:"target"`
		Call   order.Call `json:"call"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	out, err := c.Act(r.Context(), req.Target, req.Call)
	res := map[string]string{"output": out}
	if err != nil {
		res["error"] = err.Error()
	}
	reply(w, 200, res)
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	reply(w, code, map[string]string{"error": msg})
}

// ---- the front panel ----

type panelJob struct {
	ID, Source, Text, Plan, State, Reason, Output string
}

type panelData struct {
	Focus     string
	Armed     bool
	Up, Total int
	Pending   []panelJob
	Recent    []panelJob
}

func (c *Chief) panelData() panelData {
	s := c.Report()
	d := panelData{Focus: s.Focus, Armed: s.Armed, Total: len(s.Nodes)}
	for _, n := range s.Nodes {
		if n.Up {
			d.Up++
		}
	}
	for _, j := range c.Jobs(40) {
		p := panelJob{ID: j.ID, Source: j.Source, Text: j.Text, Plan: j.Plan(), State: j.State, Reason: j.Reason}
		if n := len(j.Steps); n > 0 {
			p.Output = clip(j.Steps[n-1].Output, 1500)
		}
		if j.State == StateAwait {
			d.Pending = append([]panelJob{p}, d.Pending...) // oldest first
		} else if len(d.Recent) < 6 {
			d.Recent = append(d.Recent, p)
		}
	}
	return d
}

// Panel is the direct user interface: status, orders awaiting confirmation
// with large CONFIRM and CANCEL targets, recent results, and an order box.
// It is laid out for an 800x480 touch display and carries no token, so bind
// it only where the principal's own screen can reach it. Everything it shows
// that came from a target (screen text, command output) is escaped and
// displayed as text; none of it is ever interpreted as an order.
func (c *Chief) Panel() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		panelTmpl.ExecuteTemplate(w, "page", c.panelData())
	})
	mux.HandleFunc("GET /fragment", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		panelTmpl.ExecuteTemplate(w, "live", c.panelData())
	})
	post := func(pattern string, f func(r *http.Request)) {
		mux.HandleFunc("POST "+pattern, func(w http.ResponseWriter, r *http.Request) {
			// Refuse cross-site form posts: the browser states where the form came from.
			if o := r.Header.Get("Origin"); o != "" {
				if u, err := url.Parse(o); err != nil || u.Host != r.Host {
					http.Error(w, "cross-origin request refused", http.StatusForbidden)
					return
				}
			}
			f(r)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})
	}
	post("/order", func(r *http.Request) {
		text := strings.TrimSpace(r.FormValue("text"))
		calls, err := order.Parse(text)
		if err != nil {
			c.note("rejected", map[string]any{"source": "panel", "text": text, "error": err.Error()})
			return
		}
		c.Submit(r.Context(), "panel", text, calls, 1.0)
	})
	post("/confirm", func(r *http.Request) { c.Resolve(r.Context(), r.URL.Query().Get("id"), true, "panel") })
	post("/cancel", func(r *http.Request) { c.Resolve(r.Context(), r.URL.Query().Get("id"), false, "panel") })
	return mux
}

var panelTmpl = template.Must(template.New("panel").Parse(`{{define "page"}}<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Chief of Staff</title>
<style>
:root{color-scheme:dark}
*{box-sizing:border-box}
body{margin:0;background:#101418;color:#e6e9ec;font:18px/1.35 system-ui,sans-serif}
header{display:flex;flex-wrap:wrap;gap:.6rem;align-items:center;padding:.5rem .9rem;background:#1a2129}
header b{font-size:1.15rem;margin-right:auto}
.pill{padding:.1rem .65rem;border-radius:1rem;background:#2b3743;white-space:nowrap}
.armed{background:#8a1c1c}.dry{background:#2f5d3a}
main{padding:.5rem .9rem}
.pending{border:2px solid #d9a300;border-radius:.5rem;padding:.6rem .7rem;margin-bottom:.6rem}
.pending form{display:inline}
.plan{font:15px/1.3 ui-monospace,monospace;color:#ffd866;overflow-wrap:anywhere}
button{font:inherit;font-weight:600;padding:.7rem 1.6rem;border:0;border-radius:.4rem;margin:.5rem .6rem 0 0;min-height:3.2rem;color:#fff}
.ok{background:#2f8f4e}.no{background:#555f69}
.job{border-top:1px solid #2b3743;padding:.35rem 0}
small{color:#9aa6b2}
pre{margin:.2rem 0 0;white-space:pre-wrap;overflow-wrap:anywhere;font:14px/1.3 ui-monospace,monospace;color:#b9c4cf;max-height:7.5em;overflow:auto}
.failed,.refused{color:#ff8f8f}.done{color:#8fd6a4}.cancelled{color:#9aa6b2}
form.order{display:flex;gap:.5rem;margin:.6rem .9rem}
form.order button{margin:0}
input[type=text]{flex:1;min-width:0;font:inherit;padding:.6rem;border-radius:.4rem;border:1px solid #3a4754;background:#0c1014;color:inherit}
</style></head><body>
<div id="live">{{template "live" .}}</div>
<form class="order" method="post" action="/order"><input type="text" name="text" placeholder="target server3; run uptime" autocomplete="off" aria-label="Order"><button class="ok">Send</button></form>
<script>
setInterval(async()=>{try{const r=await fetch('/fragment');if(r.ok)document.getElementById('live').innerHTML=await r.text()}catch(e){}},2000)
</script></body></html>{{end}}
{{define "live"}}<header><b>Chief of Staff</b>
<span class="pill">target: {{if .Focus}}{{.Focus}}{{else}}none{{end}}</span>
<span class="pill {{if .Armed}}armed{{else}}dry{{end}}">{{if .Armed}}ARMED{{else}}dry run{{end}}</span>
<span class="pill">{{.Up}}/{{.Total}} devices up</span></header>
<main>{{range .Pending}}<div class="pending"><div><b>Confirm?</b> {{.Text}}</div>
<div class="plan">{{.Plan}}</div><small>{{.Reason}} · {{.ID}} · from {{.Source}}</small><br>
<form method="post" action="/confirm?id={{.ID}}"><button class="ok">CONFIRM</button></form>
<form method="post" action="/cancel?id={{.ID}}"><button class="no">CANCEL</button></form></div>
{{end}}{{range .Recent}}<div class="job"><span class="{{.State}}">{{.State}}</span> {{.Text}} <small>{{.ID}} · {{.Source}}</small>{{if .Reason}}<br><small>{{.Reason}}</small>{{end}}{{if .Output}}<pre>{{.Output}}</pre>{{end}}</div>
{{end}}{{if not .Recent}}{{if not .Pending}}<p><small>No orders yet. Type one below, or speak.</small></p>{{end}}{{end}}</main>{{end}}`))
