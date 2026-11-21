package ninep

import (
	"errors"
	"io"
	"sync"
)

// Handle is one open file. Synthetic files decide what an offset means: a
// ctl file ignores it, a data file can use it to resume a stream.
type Handle interface {
	Read(off uint64, count uint32) ([]byte, error)
	Write(off uint64, data []byte) (uint32, error)
	Close()
}

// Node is a file or directory in the served tree.
type Node struct {
	Name  string
	IsDir bool
	Open  func() (Handle, error) // files
	List  func() []*Node         // directories: the children right now
}

// Server serves one tree to any number of connections.
type Server struct {
	Root *Node

	mu    sync.Mutex
	paths map[*Node]uint64
}

// Common errors, spelled the way Plan 9 clients expect.
var (
	ErrPerm     = errors.New("permission denied")
	ErrNotFound = errors.New("file does not exist")
	ErrNotDir   = errors.New("not a directory")
	ErrBadFid   = errors.New("unknown fid")
)

func (s *Server) qid(n *Node) Qid {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.paths == nil {
		s.paths = map[*Node]uint64{}
	}
	p, ok := s.paths[n]
	if !ok {
		p = uint64(len(s.paths) + 1)
		s.paths[n] = p
	}
	q := Qid{Path: p}
	if n.IsDir {
		q.Type = QTDIR
	}
	return q
}

func (s *Server) stat(n *Node) Dir {
	d := Dir{Qid: s.qid(n), Mode: 0o666, Name: n.Name, UID: "lm9", GID: "lm9", MUID: "lm9"}
	if n.IsDir {
		d.Mode = DMDIR | 0o555
	}
	return d
}

// fid is the per-connection state of one client handle.
type fid struct {
	path []*Node // from the root to this file; the walk history gives ".."
	h    Handle
	dir  []byte // directory contents, captured when the read starts at 0
	open bool
}

// Serve speaks 9P2000 on rw until the peer hangs up. Requests are handled one
// at a time. The engine's real server runs each request in its own goroutine
// and implements Tflush, so a blocked read of a token stream can be cancelled.
func (s *Server) Serve(rw io.ReadWriter) error {
	msize := uint32(8192)
	fids := map[uint32]*fid{}
	defer func() {
		for _, f := range fids {
			if f.h != nil {
				f.h.Close()
			}
		}
	}()
	for {
		t, err := ReadFcall(rw, msize)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		r := &Fcall{Type: t.Type + 1, Tag: t.Tag}
		fail := func(e error) { r.Type, r.Ename = Rerror, e.Error() }
		switch t.Type {
		case Tversion:
			if t.Msize < 256 {
				fail(errors.New("msize too small"))
				break
			}
			if t.Msize < msize {
				msize = t.Msize
			}
			r.Msize, r.Version = msize, Version
			if len(t.Version) < len(Version) || t.Version[:len(Version)] != Version {
				r.Version = "unknown"
			}
			for k, f := range fids { // a new session aborts all outstanding I/O
				if f.h != nil {
					f.h.Close()
				}
				delete(fids, k)
			}
		case Tauth:
			fail(errors.New("authentication not required"))
		case Tattach:
			if _, dup := fids[t.Fid]; dup {
				fail(errors.New("fid in use"))
				break
			}
			fids[t.Fid] = &fid{path: []*Node{s.Root}}
			r.Qid = s.qid(s.Root)
		case Tflush:
			// nothing is ever outstanding in a sequential server
		case Twalk:
			s.walk(fids, t, r, fail)
		case Topen:
			f, ok := fids[t.Fid]
			switch {
			case !ok:
				fail(ErrBadFid)
			case f.open:
				fail(errors.New("fid already open"))
			default:
				n := f.path[len(f.path)-1]
				if !n.IsDir {
					if n.Open == nil {
						fail(ErrPerm)
						break
					}
					h, err := n.Open()
					if err != nil {
						fail(err)
						break
					}
					f.h = h
				} else if t.Mode&3 != 0 {
					fail(ErrPerm)
					break
				}
				f.open = true
				r.Qid, r.Iounit = s.qid(n), msize-IOHdrSz
			}
		case Tread:
			f, ok := fids[t.Fid]
			if !ok || !f.open {
				fail(ErrBadFid)
				break
			}
			if t.Count > msize-IOHdrSz {
				t.Count = msize - IOHdrSz
			}
			if n := f.path[len(f.path)-1]; n.IsDir {
				r.Data = s.readDir(f, n, t.Offset, t.Count)
			} else if r.Data, err = f.h.Read(t.Offset, t.Count); err != nil {
				fail(err)
			} else if uint32(len(r.Data)) > t.Count {
				r.Data = r.Data[:t.Count]
			}
		case Twrite:
			f, ok := fids[t.Fid]
			if !ok || !f.open || f.h == nil {
				fail(ErrBadFid)
				break
			}
			if r.Count, err = f.h.Write(t.Offset, t.Data); err != nil {
				fail(err)
			}
		case Tclunk:
			f, ok := fids[t.Fid]
			if !ok {
				fail(ErrBadFid)
				break
			}
			if f.h != nil {
				f.h.Close()
			}
			delete(fids, t.Fid)
		case Tstat:
			f, ok := fids[t.Fid]
			if !ok {
				fail(ErrBadFid)
				break
			}
			r.Stat = s.stat(f.path[len(f.path)-1]).Bytes()
		default: // Tcreate, Tremove, Twstat: the tree is not user-modifiable
			fail(ErrPerm)
		}
		b, err := r.Bytes()
		if err != nil {
			return err
		}
		if _, err := rw.Write(b); err != nil {
			return err
		}
	}
}

func (s *Server) walk(fids map[uint32]*fid, t, r *Fcall, fail func(error)) {
	f, ok := fids[t.Fid]
	if !ok {
		fail(ErrBadFid)
		return
	}
	if f.open {
		fail(errors.New("cannot walk from an open fid"))
		return
	}
	if _, dup := fids[t.Newfid]; dup && t.Newfid != t.Fid {
		fail(errors.New("fid in use"))
		return
	}
	path := append([]*Node(nil), f.path...)
	for i, name := range t.Wname {
		cur := path[len(path)-1]
		var next *Node
		var err error
		switch {
		case !cur.IsDir:
			err = ErrNotDir
		case name == "..":
			if len(path) > 1 {
				path = path[:len(path)-1]
			}
			next = path[len(path)-1]
		default:
			err = ErrNotFound
			if cur.List != nil {
				for _, c := range cur.List() {
					if c.Name == name {
						next, err = c, nil
						path = append(path, c)
						break
					}
				}
			}
		}
		if err != nil {
			if i == 0 {
				fail(err) // nothing walked: an error
			}
			return // partly walked: a short Rwalk, and newfid is not created
		}
		r.Wqid = append(r.Wqid, s.qid(next))
	}
	fids[t.Newfid] = &fid{path: path}
}

// readDir returns whole stat records only, as the protocol requires.
func (s *Server) readDir(f *fid, n *Node, off uint64, count uint32) []byte {
	if off == 0 {
		f.dir = f.dir[:0]
		if n.List != nil {
			for _, c := range n.List() {
				f.dir = append(f.dir, s.stat(c).Bytes()...)
			}
		}
	}
	if off >= uint64(len(f.dir)) {
		return nil
	}
	rest, end := f.dir[off:], 0
	for end+2 <= len(rest) {
		size := 2 + int(rest[end]) + int(rest[end+1])<<8
		if end+size > int(count) || end+size > len(rest) {
			break
		}
		end += size
	}
	return rest[:end]
}

// ReadOnly makes a file whose content is produced each time it is opened.
func ReadOnly(name string, content func() []byte) *Node {
	return &Node{Name: name, Open: func() (Handle, error) { return &snapshot{data: content()}, nil }}
}

// WriteOnly makes a file that hands each write to fn as one command.
func WriteOnly(name string, fn func(cmd []byte) error) *Node {
	return &Node{Name: name, Open: func() (Handle, error) { return &sink{fn: fn}, nil }}
}

// Directory makes a directory whose children are computed on demand.
func Directory(name string, list func() []*Node) *Node {
	return &Node{Name: name, IsDir: true, List: list}
}

type snapshot struct{ data []byte }

func (s *snapshot) Read(off uint64, count uint32) ([]byte, error) {
	if off >= uint64(len(s.data)) {
		return nil, nil
	}
	end := min(off+uint64(count), uint64(len(s.data)))
	return s.data[off:end], nil
}
func (s *snapshot) Write(uint64, []byte) (uint32, error) { return 0, ErrPerm }
func (s *snapshot) Close()                               {}

type sink struct{ fn func([]byte) error }

func (s *sink) Read(uint64, uint32) ([]byte, error) { return nil, ErrPerm }
func (s *sink) Write(_ uint64, data []byte) (uint32, error) {
	if err := s.fn(data); err != nil {
		return 0, err
	}
	return uint32(len(data)), nil
}
func (s *sink) Close() {}
