// Package ledger is the append-only record of everything the staff was told
// and everything it did.
//
// Each line is a JSON record carrying the SHA-256 of its own content chained
// to the previous record's hash, so a removed, reordered or edited line is
// detectable with Verify. Actions are logged before they are performed and
// the write is synced to disk; a caller that cannot log must not act.
//
// The chain detects tampering by anyone who cannot rewrite the whole file.
// Ship the lines to another machine (and note the latest hash somewhere the
// box cannot reach) if you need more than that.
package ledger

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Record is one ledger line.
type Record struct {
	Seq  uint64          `json:"seq"`
	Time string          `json:"ts"`
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
	Prev string          `json:"prev"`
	Hash string          `json:"hash"`
}

func (r Record) sum() string {
	h := sha256.New()
	fmt.Fprintf(h, "%d\n%s\n%s\n%s\n%s", r.Seq, r.Time, r.Kind, r.Data, r.Prev)
	return hex.EncodeToString(h.Sum(nil))
}

// Ledger appends to one file.
type Ledger struct {
	mu   sync.Mutex
	f    *os.File
	seq  uint64
	prev string
	Now  func() time.Time
}

// Open opens or creates a ledger, verifying any existing chain first so a
// damaged file is noticed at start-up instead of being silently extended.
func Open(path string) (*Ledger, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o640)
	if err != nil {
		return nil, err
	}
	seq, prev, err := Verify(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("ledger %s: %w", path, err)
	}
	return &Ledger{f: f, seq: seq, prev: prev, Now: time.Now}, nil
}

// Append writes one record and syncs it to disk.
func (l *Ledger) Append(kind string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	r := Record{Seq: l.seq + 1, Time: l.Now().UTC().Format(time.RFC3339Nano), Kind: kind, Data: raw, Prev: l.prev}
	r.Hash = r.sum()
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err := l.f.Write(append(line, '\n')); err != nil {
		return err
	}
	if err := l.f.Sync(); err != nil {
		return err
	}
	l.seq, l.prev = r.Seq, r.Hash
	return nil
}

// Head returns the sequence number and hash of the newest record.
func (l *Ledger) Head() (uint64, string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.seq, l.prev
}

// Close closes the file.
func (l *Ledger) Close() error { return l.f.Close() }

// Verify walks a ledger and returns the last sequence number and hash.
func Verify(r io.Reader) (seq uint64, last string, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			return seq, last, fmt.Errorf("record after seq %d is not valid JSON: %w", seq, err)
		}
		if rec.Seq != seq+1 {
			return seq, last, fmt.Errorf("sequence jumps from %d to %d", seq, rec.Seq)
		}
		if rec.Prev != last {
			return seq, last, fmt.Errorf("record %d does not chain to record %d", rec.Seq, seq)
		}
		if rec.sum() != rec.Hash {
			return seq, last, fmt.Errorf("record %d has been altered", rec.Seq)
		}
		seq, last = rec.Seq, rec.Hash
	}
	return seq, last, sc.Err()
}
