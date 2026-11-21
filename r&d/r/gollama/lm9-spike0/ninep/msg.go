// Package ninep is a small 9P2000 implementation: the wire codec and a server
// for synthetic file trees. It covers the messages a control file system
// needs (version, attach, walk, open, read, write, clunk, stat, flush) and
// answers the rest with an error.
//
// Status: the codec is checked against hand-computed byte sequences and the
// server against this package's own client. It has not been mounted by the
// Linux kernel, plan9port or 9front yet; the plan lists that as a gate.
package ninep

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Message types.
const (
	Tversion = 100 + iota
	Rversion
	Tauth
	Rauth
	Tattach
	Rattach
	Terror // never sent
	Rerror
	Tflush
	Rflush
	Twalk
	Rwalk
	Topen
	Ropen
	Tcreate
	Rcreate
	Tread
	Rread
	Twrite
	Rwrite
	Tclunk
	Rclunk
	Tremove
	Rremove
	Tstat
	Rstat
	Twstat
	Rwstat
)

const (
	NoTag    = 0xFFFF
	NoFid    = 0xFFFFFFFF
	QTDIR    = 0x80
	DMDIR    = 0x80000000
	IOHdrSz  = 24 // room a read or write message needs besides its data
	MaxWElem = 16
	Version  = "9P2000"
)

// Qid is the server's identity for a file.
type Qid struct {
	Type uint8
	Vers uint32
	Path uint64
}

// Fcall is one 9P message. Only the fields its Type uses are meaningful.
type Fcall struct {
	Type    uint8
	Tag     uint16
	Fid     uint32
	Msize   uint32 // Tversion, Rversion
	Version string
	Afid    uint32 // Tattach
	Uname   string
	Aname   string
	Ename   string // Rerror
	Oldtag  uint16 // Tflush
	Newfid  uint32 // Twalk
	Wname   []string
	Wqid    []Qid // Rwalk
	Qid     Qid   // Rattach, Ropen
	Iounit  uint32
	Mode    uint8  // Topen
	Offset  uint64 // Tread, Twrite
	Count   uint32 // Tread, Rwrite
	Data    []byte // Rread, Twrite
	Stat    []byte // Rstat
}

func put16(b []byte, v uint16) []byte { return binary.LittleEndian.AppendUint16(b, v) }
func put32(b []byte, v uint32) []byte { return binary.LittleEndian.AppendUint32(b, v) }
func put64(b []byte, v uint64) []byte { return binary.LittleEndian.AppendUint64(b, v) }
func putStr(b []byte, s string) []byte {
	return append(put16(b, uint16(len(s))), s...)
}
func putQid(b []byte, q Qid) []byte {
	return put64(put32(append(b, q.Type), q.Vers), q.Path)
}

// Bytes encodes the message: size[4] type[1] tag[2] and then the body.
func (f *Fcall) Bytes() ([]byte, error) {
	for _, s := range append([]string{f.Version, f.Uname, f.Aname, f.Ename}, f.Wname...) {
		if len(s) > 0xFFFF {
			return nil, errors.New("9p: string too long")
		}
	}
	b := make([]byte, 4, 64+len(f.Data)+len(f.Stat))
	b = put16(append(b, f.Type), f.Tag)
	switch f.Type {
	case Tversion, Rversion:
		b = putStr(put32(b, f.Msize), f.Version)
	case Tattach:
		b = putStr(putStr(put32(put32(b, f.Fid), f.Afid), f.Uname), f.Aname)
	case Rattach:
		b = putQid(b, f.Qid)
	case Rerror:
		b = putStr(b, f.Ename)
	case Tflush:
		b = put16(b, f.Oldtag)
	case Rflush, Rclunk:
	case Twalk:
		if len(f.Wname) > MaxWElem {
			return nil, errors.New("9p: too many walk elements")
		}
		b = put16(put32(put32(b, f.Fid), f.Newfid), uint16(len(f.Wname)))
		for _, n := range f.Wname {
			b = putStr(b, n)
		}
	case Rwalk:
		b = put16(b, uint16(len(f.Wqid)))
		for _, q := range f.Wqid {
			b = putQid(b, q)
		}
	case Topen:
		b = append(put32(b, f.Fid), f.Mode)
	case Ropen:
		b = put32(putQid(b, f.Qid), f.Iounit)
	case Tread:
		b = put32(put64(put32(b, f.Fid), f.Offset), f.Count)
	case Rread:
		b = append(put32(b, uint32(len(f.Data))), f.Data...)
	case Twrite:
		b = append(put32(put64(put32(b, f.Fid), f.Offset), uint32(len(f.Data))), f.Data...)
	case Rwrite:
		b = put32(b, f.Count)
	case Tclunk, Tstat:
		b = put32(b, f.Fid)
	case Rstat:
		b = append(put16(b, uint16(len(f.Stat))), f.Stat...)
	default:
		return nil, fmt.Errorf("9p: cannot encode message type %d", f.Type)
	}
	binary.LittleEndian.PutUint32(b, uint32(len(b)))
	return b, nil
}

var errMalformed = errors.New("9p: malformed message")

// dec reads fields from a message body and remembers the first failure.
type dec struct {
	b   []byte
	bad bool
}

func (d *dec) take(n int) []byte {
	if d.bad || n > len(d.b) {
		d.bad = true
		return make([]byte, n&0xFFFF)
	}
	s := d.b[:n]
	d.b = d.b[n:]
	return s
}
func (d *dec) u8() uint8   { return d.take(1)[0] }
func (d *dec) u16() uint16 { return binary.LittleEndian.Uint16(d.take(2)) }
func (d *dec) u32() uint32 { return binary.LittleEndian.Uint32(d.take(4)) }
func (d *dec) u64() uint64 { return binary.LittleEndian.Uint64(d.take(8)) }
func (d *dec) str() string { return string(d.take(int(d.u16()))) }
func (d *dec) qid() Qid    { return Qid{d.u8(), d.u32(), d.u64()} }

// ReadFcall reads one message from r. msize bounds the message length, so a
// peer cannot make the reader allocate more than was negotiated.
func ReadFcall(r io.Reader, msize uint32) (*Fcall, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(hdr[:])
	if size < 7 || size > msize {
		return nil, fmt.Errorf("9p: message size %d outside 7..%d", size, msize)
	}
	body := make([]byte, size-4)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return parse(body)
}

func parse(body []byte) (*Fcall, error) {
	d := &dec{b: body}
	f := &Fcall{Type: d.u8(), Tag: d.u16()}
	switch f.Type {
	case Tversion, Rversion:
		f.Msize, f.Version = d.u32(), d.str()
	case Tattach:
		f.Fid, f.Afid, f.Uname, f.Aname = d.u32(), d.u32(), d.str(), d.str()
	case Tauth:
		f.Afid, f.Uname, f.Aname = d.u32(), d.str(), d.str()
	case Rattach:
		f.Qid = d.qid()
	case Rerror:
		f.Ename = d.str()
	case Tflush:
		f.Oldtag = d.u16()
	case Rflush, Rclunk:
	case Twalk:
		f.Fid, f.Newfid = d.u32(), d.u32()
		n := int(d.u16())
		if n > MaxWElem {
			return nil, errMalformed
		}
		for i := 0; i < n; i++ {
			f.Wname = append(f.Wname, d.str())
		}
	case Rwalk:
		n := int(d.u16())
		if n > MaxWElem {
			return nil, errMalformed
		}
		for i := 0; i < n; i++ {
			f.Wqid = append(f.Wqid, d.qid())
		}
	case Topen:
		f.Fid, f.Mode = d.u32(), d.u8()
	case Ropen:
		f.Qid, f.Iounit = d.qid(), d.u32()
	case Tread:
		f.Fid, f.Offset, f.Count = d.u32(), d.u64(), d.u32()
	case Rread:
		n := d.u32()
		if uint64(n) > uint64(len(d.b)) {
			return nil, errMalformed
		}
		f.Data = d.take(int(n))
	case Twrite:
		f.Fid, f.Offset = d.u32(), d.u64()
		n := d.u32()
		if uint64(n) > uint64(len(d.b)) {
			return nil, errMalformed
		}
		f.Data = d.take(int(n))
	case Rwrite:
		f.Count = d.u32()
	case Tclunk, Tstat, Tremove:
		f.Fid = d.u32()
	case Rstat:
		f.Stat = d.take(int(d.u16()))
	case Tcreate, Twstat:
		return f, nil // recognised so the server can refuse it by name
	default:
		return nil, fmt.Errorf("9p: unknown message type %d", f.Type)
	}
	if d.bad || len(d.b) != 0 {
		return nil, errMalformed
	}
	return f, nil
}

// Dir is the stat record of a file.
type Dir struct {
	Qid    Qid
	Mode   uint32
	Atime  uint32
	Mtime  uint32
	Length uint64
	Name   string
	UID    string
	GID    string
	MUID   string
}

// Bytes encodes the record with its leading two-byte size, as it appears in
// Rstat and in directory reads.
func (d Dir) Bytes() []byte {
	b := make([]byte, 2, 64+len(d.Name))
	b = put32(put16(b, 0), 0) // type, dev: for kernel use
	b = putQid(b, d.Qid)
	b = put64(put32(put32(put32(b, d.Mode), d.Atime), d.Mtime), d.Length)
	b = putStr(putStr(putStr(putStr(b, d.Name), d.UID), d.GID), d.MUID)
	binary.LittleEndian.PutUint16(b, uint16(len(b)-2))
	return b
}

// ParseDirs decodes the concatenated records returned by a directory read.
func ParseDirs(b []byte) ([]Dir, error) {
	var out []Dir
	for len(b) > 0 {
		if len(b) < 2 {
			return nil, errMalformed
		}
		n := int(binary.LittleEndian.Uint16(b))
		if n+2 > len(b) {
			return nil, errMalformed
		}
		d := &dec{b: b[2 : 2+n]}
		d.u16()
		d.u32()
		e := Dir{Qid: d.qid(), Mode: d.u32(), Atime: d.u32(), Mtime: d.u32(), Length: d.u64(),
			Name: d.str(), UID: d.str(), GID: d.str(), MUID: d.str()}
		if d.bad {
			return nil, errMalformed
		}
		out = append(out, e)
		b = b[2+n:]
	}
	return out, nil
}
