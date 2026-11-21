package ninep

import (
	"bytes"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
)

// Byte sequences worked out by hand from the protocol manual, so the codec is
// not only tested against itself.
func TestWireFormat(t *testing.T) {
	ver, _ := (&Fcall{Type: Tversion, Tag: NoTag, Msize: 8192, Version: "9P2000"}).Bytes()
	wantVer := []byte{19, 0, 0, 0, 100, 0xff, 0xff, 0x00, 0x20, 0, 0, 6, 0, '9', 'P', '2', '0', '0', '0'}
	if !bytes.Equal(ver, wantVer) {
		t.Errorf("Tversion\n got  % x\n want % x", ver, wantVer)
	}
	walk, _ := (&Fcall{Type: Twalk, Tag: 1, Fid: 0, Newfid: 1, Wname: []string{"sessions", "clone"}}).Bytes()
	wantWalk := append([]byte{34, 0, 0, 0, 110, 1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 2, 0, 8, 0}, "sessions"...)
	wantWalk = append(append(wantWalk, 5, 0), "clone"...)
	if !bytes.Equal(walk, wantWalk) {
		t.Errorf("Twalk\n got  % x\n want % x", walk, wantWalk)
	}
	st := Dir{Qid: Qid{Type: QTDIR, Path: 7}, Mode: DMDIR | 0o555, Name: "m", UID: "u", GID: "g", MUID: "u"}.Bytes()
	// size[2]=49+4 type[2] dev[4] qid[13] mode[4] atime[4] mtime[4] length[8] then four strings.
	if len(st) != 2+2+4+13+4+4+4+8+(2+1)*4 || int(st[0]) != len(st)-2 || st[8] != QTDIR || st[13] != 7 {
		t.Errorf("stat record: % x", st)
	}
	if st[21] != 0x6d || st[22] != 0x01 || st[24] != 0x80 { // mode 0x8000016d little-endian
		t.Errorf("stat mode bytes: % x", st[21:25])
	}
	for _, m := range [][]byte{ver, walk} {
		f, err := parse(m[4:])
		if err != nil {
			t.Fatal(err)
		}
		if again, _ := f.Bytes(); !bytes.Equal(again, m) {
			t.Errorf("decode then encode changed the message")
		}
	}
}

// engine is a stand-in for lm9 with the same file tree shape as the plan:
// /probe, /sessions/clone, /sessions/N/{prompt,data}. Its "model" upper-cases.
type engine struct {
	mu       sync.Mutex
	sessions []*session
}

type session struct {
	id     int
	prompt string
}

// cloneHandle is the Plan 9 clone idiom: opening the file allocates a new
// session, and reading it returns that session's number.
type cloneHandle struct{ id int }

func (c *cloneHandle) Read(off uint64, _ uint32) ([]byte, error) {
	if off > 0 {
		return nil, nil
	}
	return []byte(fmt.Sprintf("%d\n", c.id)), nil
}
func (c *cloneHandle) Write(uint64, []byte) (uint32, error) { return 0, ErrPerm }
func (c *cloneHandle) Close()                               {}

func (e *engine) tree() *Node {
	clone := &Node{Name: "clone", Open: func() (Handle, error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		s := &session{id: len(e.sessions)}
		e.sessions = append(e.sessions, s)
		return &cloneHandle{id: s.id}, nil
	}}
	sessions := Directory("sessions", func() []*Node {
		e.mu.Lock()
		defer e.mu.Unlock()
		nodes := []*Node{clone}
		for _, s := range e.sessions {
			nodes = append(nodes, Directory(fmt.Sprint(s.id), func() []*Node {
				return []*Node{
					WriteOnly("prompt", func(b []byte) error {
						e.mu.Lock()
						defer e.mu.Unlock()
						s.prompt = string(b)
						return nil
					}),
					ReadOnly("data", func() []byte {
						e.mu.Lock()
						defer e.mu.Unlock()
						return []byte(strings.ToUpper(s.prompt))
					}),
				}
			}))
		}
		return nodes
	})
	probe := ReadOnly("probe", func() []byte { return []byte(`{"cpu":"test"}` + "\n") })
	return Directory("/", func() []*Node { return []*Node{probe, sessions} })
}

// client is the least a 9P client needs, for the test.
type client struct {
	t     *testing.T
	c     net.Conn
	msize uint32
	tag   uint16
	fid   uint32
}

func (c *client) rpc(tx *Fcall) *Fcall {
	c.t.Helper()
	if tx.Type != Tversion {
		c.tag++
		tx.Tag = c.tag
	}
	b, err := tx.Bytes()
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := c.c.Write(b); err != nil {
		c.t.Fatal(err)
	}
	rx, err := ReadFcall(c.c, c.msize)
	if err != nil {
		c.t.Fatal(err)
	}
	if rx.Tag != tx.Tag {
		c.t.Fatalf("reply tag %d for request tag %d", rx.Tag, tx.Tag)
	}
	return rx
}

func (c *client) must(tx *Fcall) *Fcall {
	c.t.Helper()
	rx := c.rpc(tx)
	if rx.Type == Rerror {
		c.t.Fatalf("request type %d failed: %s", tx.Type, rx.Ename)
	}
	return rx
}

// open walks from the root (fid 0) to path and opens it.
func (c *client) open(mode uint8, path ...string) uint32 {
	c.t.Helper()
	c.fid++
	if rx := c.must(&Fcall{Type: Twalk, Fid: 0, Newfid: c.fid, Wname: path}); len(rx.Wqid) != len(path) {
		c.t.Fatalf("walk %v stopped after %d elements", path, len(rx.Wqid))
	}
	c.must(&Fcall{Type: Topen, Fid: c.fid, Mode: mode})
	return c.fid
}

func (c *client) cat(path ...string) string {
	c.t.Helper()
	fid := c.open(0, path...)
	var out []byte
	for {
		rx := c.must(&Fcall{Type: Tread, Fid: fid, Offset: uint64(len(out)), Count: 4096})
		if len(rx.Data) == 0 {
			break
		}
		out = append(out, rx.Data...)
	}
	c.must(&Fcall{Type: Tclunk, Fid: fid})
	return string(out)
}

func (c *client) echo(text string, path ...string) {
	c.t.Helper()
	fid := c.open(1, path...)
	if rx := c.must(&Fcall{Type: Twrite, Fid: fid, Data: []byte(text)}); int(rx.Count) != len(text) {
		c.t.Fatalf("short write: %d of %d", rx.Count, len(text))
	}
	c.must(&Fcall{Type: Tclunk, Fid: fid})
}

func (c *client) ls(path ...string) []string {
	c.t.Helper()
	dirs, err := ParseDirs([]byte(c.cat(path...)))
	if err != nil {
		c.t.Fatal(err)
	}
	var names []string
	for _, d := range dirs {
		names = append(names, d.Name)
	}
	return names
}

func dial(t *testing.T, srv *Server) *client {
	a, b := net.Pipe()
	go srv.Serve(a)
	t.Cleanup(func() { b.Close() })
	c := &client{t: t, c: b, msize: 8192}
	if rx := c.must(&Fcall{Type: Tversion, Tag: NoTag, Msize: 8192, Version: Version}); rx.Version != Version {
		t.Fatalf("server speaks %q", rx.Version)
	}
	c.must(&Fcall{Type: Tattach, Fid: 0, Afid: NoFid, Uname: "glenda"})
	return c
}

// The three-line shell client from the plan, spoken in raw 9P:
//
//	n=`{cat /mnt/lm9/sessions/clone}
//	echo hello > /mnt/lm9/sessions/$n/prompt
//	cat /mnt/lm9/sessions/$n/data
func TestSessionThroughTheFileTree(t *testing.T) {
	c := dial(t, &Server{Root: (&engine{}).tree()})

	if got := c.cat("probe"); got != `{"cpu":"test"}`+"\n" {
		t.Fatalf("probe = %q", got)
	}
	n := strings.TrimSpace(c.cat("sessions", "clone"))
	if n != "0" {
		t.Fatalf("first session is %q", n)
	}
	c.echo("why is the sky blue?", "sessions", n, "prompt")
	if got := c.cat("sessions", n, "data"); got != "WHY IS THE SKY BLUE?" {
		t.Fatalf("data = %q", got)
	}
	if m := strings.TrimSpace(c.cat("sessions", "clone")); m != "1" {
		t.Fatalf("second session is %q", m)
	}
	if got := c.ls("sessions"); !slices.Equal(got, []string{"clone", "0", "1"}) {
		t.Fatalf("ls sessions = %v", got)
	}
	if got := c.ls(); !slices.Equal(got, []string{"probe", "sessions"}) {
		t.Fatalf("ls / = %v", got)
	}
}

func TestErrorsAreReportedNotFatal(t *testing.T) {
	c := dial(t, &Server{Root: (&engine{}).tree()})

	if rx := c.rpc(&Fcall{Type: Twalk, Fid: 0, Newfid: 9, Wname: []string{"nope"}}); rx.Type != Rerror {
		t.Fatalf("walk to a missing file: type %d", rx.Type)
	}
	// A walk that fails part way returns the part that worked and creates no fid.
	rx := c.rpc(&Fcall{Type: Twalk, Fid: 0, Newfid: 9, Wname: []string{"sessions", "nope"}})
	if rx.Type != Rwalk || len(rx.Wqid) != 1 {
		t.Fatalf("partial walk: type %d, %d qids", rx.Type, len(rx.Wqid))
	}
	if rx := c.rpc(&Fcall{Type: Tclunk, Fid: 9}); rx.Type != Rerror {
		t.Fatal("fid from a failed walk exists")
	}
	// Writing to a read-only file, and creating files, are refused.
	fid := c.open(0, "probe")
	if rx := c.rpc(&Fcall{Type: Twrite, Fid: fid, Data: []byte("x")}); rx.Type != Rerror {
		t.Fatal("write to probe accepted")
	}
	// ".." from the root stays at the root; stat works on a directory.
	c.must(&Fcall{Type: Twalk, Fid: 0, Newfid: 20, Wname: []string{"sessions", ".."}})
	st := c.must(&Fcall{Type: Tstat, Fid: 20})
	if d, err := ParseDirs(st.Stat); err != nil || len(d) != 1 || d[0].Name != "/" || d[0].Mode&DMDIR == 0 {
		t.Fatalf("stat of root: %+v, %v", d, err)
	}
	// The server is still answering.
	if got := c.cat("probe"); got == "" {
		t.Fatal("server stopped answering after errors")
	}
}

func FuzzParse(f *testing.F) {
	for _, m := range []*Fcall{
		{Type: Tversion, Tag: NoTag, Msize: 8192, Version: Version},
		{Type: Twalk, Tag: 1, Newfid: 1, Wname: []string{"a", "b"}},
		{Type: Twrite, Tag: 2, Fid: 1, Data: []byte("hello")},
	} {
		b, _ := m.Bytes()
		f.Add(b[4:])
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		if m, err := parse(body); err == nil && m.Type != Tcreate && m.Type != Twstat && m.Type != Tauth && m.Type != Tremove {
			if _, err := m.Bytes(); err != nil {
				t.Fatalf("decoded a message that cannot be encoded: %v", err)
			}
		}
	})
}
