package gguf

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// builder writes a small, valid GGUF file for the tests.
type builder struct{ bytes.Buffer }

func (w *builder) u32(v uint32) { binary.Write(w, binary.LittleEndian, v) }
func (w *builder) u64(v uint64) { binary.Write(w, binary.LittleEndian, v) }
func (w *builder) str(s string) { w.u64(uint64(len(s))); w.WriteString(s) }

func sample() []byte {
	var w builder
	w.u32(Magic)
	w.u32(3)
	w.u64(2) // tensors
	w.u64(5) // key-value pairs

	w.str("general.architecture")
	w.u32(uint32(String))
	w.str("llama")

	w.str("general.alignment")
	w.u32(uint32(Uint32))
	w.u32(32)

	w.str("llama.context_length")
	w.u32(uint32(Uint64))
	w.u64(8192)

	w.str("tokenizer.ggml.tokens")
	w.u32(uint32(Array))
	w.u32(uint32(String))
	w.u64(3)
	w.str("<s>")
	w.str("hello")
	w.str("▁world")

	w.str("tokenizer.ggml.scores")
	w.u32(uint32(Array))
	w.u32(uint32(Float32))
	w.u64(3)
	for _, f := range []float32{0, -1.5, -2.25} {
		w.u32(math.Float32bits(f))
	}

	// Tensor "w": Q8_0, 64 x 2 = 128 weights = 4 blocks = 136 bytes, at offset 0.
	w.str("w")
	w.u32(2)
	w.u64(64)
	w.u64(2)
	w.u32(uint32(Q8_0))
	w.u64(0)
	// Tensor "b": F32, 4 values = 16 bytes, at the next 32-byte boundary after 136.
	w.str("b")
	w.u32(1)
	w.u64(4)
	w.u32(uint32(F32))
	w.u64(160)

	for w.Len()%32 != 0 {
		w.WriteByte(0)
	}
	data := make([]byte, 160+16)
	for i := range data {
		data[i] = byte(i)
	}
	w.Write(data)
	return w.Bytes()
}

func TestParse(t *testing.T) {
	b := sample()
	f, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != 3 || f.Alignment != 32 || f.DataOffset%32 != 0 {
		t.Fatalf("header: %+v", f)
	}
	if got := f.Meta["general.architecture"]; got != "llama" {
		t.Errorf("architecture = %v", got)
	}
	if got := f.Meta["llama.context_length"]; got != uint64(8192) {
		t.Errorf("context length = %v", got)
	}
	toks, ok := f.Meta["tokenizer.ggml.tokens"].([]string)
	if !ok || len(toks) != 3 || toks[2] != "▁world" {
		t.Errorf("tokens = %v", f.Meta["tokenizer.ggml.tokens"])
	}
	if s, ok := f.Meta["tokenizer.ggml.scores"].([]any); !ok || len(s) != 3 || s[1] != float32(-1.5) {
		t.Errorf("scores = %v", f.Meta["tokenizer.ggml.scores"])
	}
	if len(f.Tensors) != 2 {
		t.Fatalf("tensors = %d", len(f.Tensors))
	}
	w, bias := &f.Tensors[0], &f.Tensors[1]
	if w.Name != "w" || w.Bytes != 136 || bias.Bytes != 16 {
		t.Fatalf("tensor sizes: %+v %+v", w, bias)
	}
	if d := f.Data(b, bias); len(d) != 16 || d[0] != 160 {
		t.Fatalf("bias data starts with %d, want 160", d[0])
	}
}

// Cutting the file anywhere must give an error, never a panic.
func TestEveryTruncationIsAnError(t *testing.T) {
	b := sample()
	for n := 0; n < len(b); n++ {
		if _, err := Parse(b[:n]); err == nil {
			t.Fatalf("file truncated to %d of %d bytes was accepted", n, len(b))
		}
	}
}

func TestHostileCountsAreRejectedBeforeAllocation(t *testing.T) {
	var w builder
	w.u32(Magic)
	w.u32(3)
	w.u64(1 << 60) // tensors
	w.u64(1 << 60) // key-value pairs
	if _, err := Parse(w.Bytes()); err == nil {
		t.Fatal("absurd counts accepted")
	}

	var a builder
	a.u32(Magic)
	a.u32(3)
	a.u64(0)
	a.u64(1)
	a.str("x")
	a.u32(uint32(Array))
	a.u32(uint32(Uint8))
	a.u64(1 << 62) // array far longer than the file
	if _, err := Parse(a.Bytes()); err == nil {
		t.Fatal("absurd array length accepted")
	}
}

func FuzzParse(f *testing.F) {
	f.Add(sample())
	f.Add([]byte("GGUF"))
	f.Fuzz(func(t *testing.T, b []byte) {
		file, err := Parse(b)
		if err != nil {
			return
		}
		for i := range file.Tensors { // anything Parse accepts must be safe to slice
			_ = file.Data(b, &file.Tensors[i])
		}
	})
}
