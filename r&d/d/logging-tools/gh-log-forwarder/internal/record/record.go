// Package record builds Dynatrace log records as JSON objects without
// reflection: keys appear in the order written, so output is
// deterministic and cheap. Billing counts raw bytes, so this is also
// where the record schema is kept deliberately lean.
package record

import (
	"net/url"
	"strconv"
	"unicode/utf8"
)

// KV is one attribute.
type KV struct {
	K string
	V any // string, int, int64, bool
}

// Builder appends one JSON object.
type Builder struct {
	buf []byte
	n   int
}

// Reset starts a new object, reusing the buffer.
func (b *Builder) Reset() {
	b.buf = append(b.buf[:0], '{')
	b.n = 0
}

func (b *Builder) key(k string) {
	if b.n > 0 {
		b.buf = append(b.buf, ',')
	}
	b.n++
	b.buf = AppendString(b.buf, k)
	b.buf = append(b.buf, ':')
}

// Str adds a string field.
func (b *Builder) Str(k, v string) { b.key(k); b.buf = AppendString(b.buf, v) }

// Int adds an integer field.
func (b *Builder) Int(k string, v int64) { b.key(k); b.buf = strconv.AppendInt(b.buf, v, 10) }

// Bool adds a boolean field.
func (b *Builder) Bool(k string, v bool) { b.key(k); b.buf = strconv.AppendBool(b.buf, v) }

// Any adds a field of one of the KV value types.
func (b *Builder) Any(kv KV) {
	switch v := kv.V.(type) {
	case string:
		b.Str(kv.K, v)
	case int:
		b.Int(kv.K, int64(v))
	case int64:
		b.Int(kv.K, v)
	case bool:
		b.Bool(kv.K, v)
	default:
		b.Str(kv.K, "")
	}
}

// Bytes closes the object and returns a copy.
func (b *Builder) Bytes() []byte {
	out := make([]byte, len(b.buf)+1)
	copy(out, b.buf)
	out[len(b.buf)] = '}'
	return out
}

const hex = "0123456789abcdef"

// AppendString appends s as a JSON string: invalid UTF-8 becomes
// U+FFFD, control characters and U+2028/9 are escaped. HTML characters
// are left alone, unlike encoding/json, to save bytes.
func AppendString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		if c := s[i]; c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			switch c {
			case '"', '\\':
				dst = append(dst, '\\', c)
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xF])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			dst = append(dst, s[start:i]...)
			dst = append(dst, `�`...)
			i += size
			start = i
			continue
		}
		if r == ' ' || r == ' ' {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hex[r&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}

// HeaderValue encodes attributes for the X-Dynatrace-Attr header as
// ampersand-separated key=value pairs, URL-escaped. (The escaping rule
// is not documented by Dynatrace; verify in the Phase-0 spike.)
func HeaderValue(attrs []KV) string {
	var b []byte
	for i, kv := range attrs {
		if i > 0 {
			b = append(b, '&')
		}
		b = append(b, url.QueryEscape(kv.K)...)
		b = append(b, '=')
		var v string
		switch x := kv.V.(type) {
		case string:
			v = x
		case int:
			v = strconv.Itoa(x)
		case int64:
			v = strconv.FormatInt(x, 10)
		case bool:
			v = strconv.FormatBool(x)
		}
		b = append(b, url.QueryEscape(v)...)
	}
	return string(b)
}
