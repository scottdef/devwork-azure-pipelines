// Package audit keeps an append-only log, one JSON object per line.
//
// Each record carries the hash of the one before it, so a deleted or
// edited line breaks every hash after it. It is a text file: grep it,
// tail it, commit it. Verify tells you whether to believe it.
package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Record is one line of the log.
type Record struct {
	Seq     int               `json:"seq"`
	Time    time.Time         `json:"time"`
	Actor   string            `json:"actor"`
	Kind    string            `json:"kind"`    // dispatch.request, dispatch.sent, probe.api, report.write, ...
	Subject string            `json:"subject"` // deployment, workflow or file the record is about
	Detail  map[string]string `json:"detail,omitempty"`
	Prev    string            `json:"prev"`
	Hash    string            `json:"hash"`
}

// sum hashes the record with Hash blanked. encoding/json writes struct
// fields in order and map keys sorted, so the bytes are reproducible.
func (r Record) sum() string {
	r.Hash = ""
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Log is a file of records.
type Log struct {
	Path string
	mu   sync.Mutex
}

// Open makes the directory and returns the log. The file is created on
// first Append.
func Open(dir string) (*Log, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Log{Path: filepath.Join(dir, "audit.jsonl")}, nil
}

// Read returns every record. A missing file is an empty log.
func (l *Log) Read() ([]Record, error) {
	f, err := os.Open(l.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var recs []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for n := 1; sc.Scan(); n++ {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return recs, fmt.Errorf("%s:%d: %w", l.Path, n, err)
		}
		recs = append(recs, r)
	}
	return recs, sc.Err()
}

// Append writes one record, chained to the last.
func (l *Log) Append(actor, kind, subject string, detail map[string]string) (Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	recs, err := l.Read()
	if err != nil {
		return Record{}, err
	}
	r := Record{
		Seq:     1,
		Time:    time.Now().UTC().Truncate(time.Second),
		Actor:   actor,
		Kind:    kind,
		Subject: subject,
		Detail:  detail,
	}
	if n := len(recs); n > 0 {
		r.Seq, r.Prev = recs[n-1].Seq+1, recs[n-1].Hash
	}
	r.Hash = r.sum()
	line, err := json.Marshal(r)
	if err != nil {
		return Record{}, err
	}
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return Record{}, err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return r, err
}

// Verify walks the chain. It returns the number of good records and
// the first problem found, if any.
func Verify(recs []Record) (int, error) {
	prev := ""
	for i, r := range recs {
		switch {
		case r.Seq != i+1:
			return i, fmt.Errorf("record %d: sequence is %d: a line was removed or reordered", i+1, r.Seq)
		case r.Prev != prev:
			return i, fmt.Errorf("record %d: does not follow record %d", r.Seq, i)
		case r.Hash != r.sum():
			return i, fmt.Errorf("record %d: content does not match its hash", r.Seq)
		}
		prev = r.Hash
	}
	return len(recs), nil
}
