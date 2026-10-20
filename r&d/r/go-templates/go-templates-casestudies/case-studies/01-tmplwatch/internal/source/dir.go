package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Dir is a source backed by a local directory tree. Each regular file
// under Root/Prefix is a key. It is the source used by tests, by -once in
// CI, and on Plan 9, where no other transport is assumed.
type Dir struct {
	Root   string
	Prefix string

	stamp string
	last  Snapshot
}

// NewDir returns a directory source.
func NewDir(root, prefix string) *Dir {
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return &Dir{Root: root, Prefix: prefix}
}

// String names the source in logs.
func (d *Dir) String() string { return "dir:" + filepath.Join(d.Root, d.Prefix) }

// Fetch walks the tree. If no file's name, size or mtime moved since the
// last walk, nothing is read and changed is false.
func (d *Dir) Fetch(ctx context.Context) (Snapshot, bool, error) {
	type ent struct {
		key, file string
	}
	var ents []ent
	h := sha256.New()
	err := filepath.WalkDir(d.Root, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, err := filepath.Rel(d.Root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if de.IsDir() {
			if rel != "." && strings.HasPrefix(de.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !de.Type().IsRegular() {
			return nil
		}
		k, ok := keyFor(rel, d.Prefix)
		if !ok {
			return nil
		}
		fi, err := de.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\n", rel, fi.Size(), fi.ModTime().UnixNano())
		ents = append(ents, ent{k, p})
		return nil
	})
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("%s: %w", d, err)
	}
	stamp := hex.EncodeToString(h.Sum(nil))
	if stamp == d.stamp {
		return d.last, false, nil
	}
	data := make(map[string]string, len(ents))
	for _, e := range ents {
		b, err := os.ReadFile(e.file)
		if err != nil {
			return Snapshot{}, false, err
		}
		data[e.key] = value(b)
	}
	s := Snapshot{Data: data, Version: Hash(data)}
	// An mtime bump with identical content is not a change.
	changed := s.Version != d.last.Version
	d.stamp, d.last = stamp, s
	return s, changed, nil
}
