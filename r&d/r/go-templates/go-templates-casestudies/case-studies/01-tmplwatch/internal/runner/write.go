package runner

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// tempMark is part of every temporary file name, so leftovers from a
// crash can be recognised and removed.
const tempMark = ".tmplwatch-"

// syncFile is replaced in tests to simulate a failure between write and
// rename.
var syncFile = (*os.File).Sync

// WriteAtomic replaces path with data so that readers see either the old
// file or the new one, never a mix (on Plan 9, see rename below):
//
//  1. if the existing file already holds data (by SHA-256), do nothing;
//  2. write data to a temporary file in the same directory (rename is
//     only atomic within a file system);
//  3. fsync it, set perm, close it;
//  4. optionally copy the old file to path.bak (atomically as well);
//  5. rename over path, then fsync the directory so the rename survives a
//     power cut. On Plan 9, wstat cannot replace an existing name, so Go's
//     os.Rename removes the old file first: a reader may briefly find no
//     file, but never a partial one.
//
// On any failure the temporary file is removed and path is untouched.
// It reports whether path was changed.
func WriteAtomic(path string, data []byte, perm fs.FileMode, backup bool) (changed bool, err error) {
	old, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if exists && sha256.Sum256(old) == sha256.Sum256(data) {
		// Content is right; repair the mode if someone changed it.
		if fi, err := os.Stat(path); err == nil && fi.Mode().Perm() != perm {
			return false, os.Chmod(path, perm)
		}
		return false, nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	if exists && backup {
		if err := replace(path+".bak", old, perm); err != nil {
			return false, fmt.Errorf("backup: %w", err)
		}
	}
	if err := replace(path, data, perm); err != nil {
		return false, err
	}
	return true, nil
}

func replace(path string, data []byte, perm fs.FileMode) (err error) {
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	f, err := os.CreateTemp(dir, "."+base+tempMark+"*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = syncFile(f); err != nil {
		return err
	}
	// CreateTemp makes 0600; set the configured mode before the file
	// becomes visible under its real name.
	if err = f.Chmod(perm); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// Rename to the same spelling of the directory CreateTemp used: Go's
	// Plan 9 rename requires the new name to share the old one's literal
	// directory prefix (it is a wstat of the name, not a path operation).
	// CreateTemp joins without cleaning, so derive the target from tmp.
	target := tmp[:strings.LastIndexByte(tmp, os.PathSeparator)+1] + base
	if err = os.Rename(tmp, target); err != nil {
		return err
	}
	syncDir(dir)
	return nil
}

// syncDir makes a rename durable. Not every system can fsync a directory
// (Plan 9 has no need to), so errors are ignored.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}

// CleanStale removes temporary files left next to path by a process that
// died between create and rename. It returns the names removed.
func CleanStale(path string) ([]string, error) {
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var gone []string
	for _, e := range ents {
		n := e.Name()
		for _, b := range []string{base, base + ".bak"} {
			if strings.HasPrefix(n, "."+b+tempMark) && !e.IsDir() {
				if err := os.Remove(filepath.Join(dir, n)); err == nil {
					gone = append(gone, n)
				}
			}
		}
	}
	return gone, nil
}

// Same reports whether path holds exactly data.
func Same(path string, data []byte) bool {
	old, err := os.ReadFile(path)
	return err == nil && bytes.Equal(old, data)
}
