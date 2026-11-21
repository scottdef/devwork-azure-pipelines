// Package gguf reads the header, metadata and tensor directory of a GGUF
// file from a byte slice, normally a read-only mapping of the whole file.
//
// Model files are untrusted input. Every length is checked against the bytes
// that remain before anything is allocated or sliced, so a hostile file can
// produce an error but not a panic, an out-of-range read or a huge allocation.
package gguf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/bits"
)

// Magic is "GGUF" read as a little-endian uint32.
const Magic = 0x46554747

const (
	maxDims      = 4
	maxArrayNest = 4
	defaultAlign = 32
)

// ValueType is the type tag of a metadata value.
type ValueType uint32

const (
	Uint8 ValueType = iota
	Int8
	Uint16
	Int16
	Uint32
	Int32
	Float32
	Bool
	String
	Array
	Uint64
	Int64
	Float64
)

// TensorType is ggml's tensor type number.
type TensorType uint32

const (
	F32  TensorType = 0
	F16  TensorType = 1
	Q4_0 TensorType = 2
	Q4_1 TensorType = 3
	Q5_0 TensorType = 6
	Q5_1 TensorType = 7
	Q8_0 TensorType = 8
	Q2_K TensorType = 10
	Q3_K TensorType = 11
	Q4_K TensorType = 12
	Q5_K TensorType = 13
	Q6_K TensorType = 14
	I8   TensorType = 24
	I16  TensorType = 25
	I32  TensorType = 26
	I64  TensorType = 27
	F64  TensorType = 28
	BF16 TensorType = 30
)

// Layout gives the weights per block and bytes per block of a tensor type.
// Types missing from this table are rejected by Parse; adding a format means
// adding a row here and a kernel for it.
var Layout = map[TensorType][2]uint64{
	F32: {1, 4}, F16: {1, 2}, BF16: {1, 2}, F64: {1, 8},
	I8: {1, 1}, I16: {1, 2}, I32: {1, 4}, I64: {1, 8},
	Q4_0: {32, 18}, Q4_1: {32, 20}, Q5_0: {32, 22}, Q5_1: {32, 24}, Q8_0: {32, 34},
	Q2_K: {256, 84}, Q3_K: {256, 110}, Q4_K: {256, 144}, Q5_K: {256, 176}, Q6_K: {256, 210},
}

// Tensor describes one tensor in the file.
type Tensor struct {
	Name   string
	Dims   []uint64 // innermost dimension first, as stored
	Type   TensorType
	Offset uint64 // from File.DataOffset
	Bytes  uint64 // size of the tensor data
}

// File is the parsed directory of a GGUF file.
type File struct {
	Version    uint32
	Meta       map[string]any
	Tensors    []Tensor
	Alignment  uint64
	DataOffset uint64 // where tensor data begins
}

var errShort = errors.New("gguf: file is shorter than its own header claims")

type reader struct {
	b   []byte
	off uint64
}

func (r *reader) left() uint64 { return uint64(len(r.b)) - r.off }

func (r *reader) take(n uint64) ([]byte, error) {
	if n > r.left() {
		return nil, errShort
	}
	s := r.b[r.off : r.off+n : r.off+n]
	r.off += n
	return s, nil
}

func (r *reader) u32() (uint32, error) {
	b, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

func (r *reader) u64() (uint64, error) {
	b, err := r.take(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b), nil
}

func (r *reader) str() (string, error) {
	n, err := r.u64()
	if err != nil {
		return "", err
	}
	b, err := r.take(n)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// minSize is the fewest bytes one value of each type can occupy. It bounds
// array lengths before the array is allocated.
var minSize = [...]uint64{Uint8: 1, Int8: 1, Uint16: 2, Int16: 2, Uint32: 4, Int32: 4,
	Float32: 4, Bool: 1, String: 8, Array: 12, Uint64: 8, Int64: 8, Float64: 8}

func (r *reader) value(t ValueType, depth int) (any, error) {
	switch t {
	case Uint8, Int8, Bool:
		b, err := r.take(1)
		if err != nil {
			return nil, err
		}
		switch t {
		case Uint8:
			return b[0], nil
		case Int8:
			return int8(b[0]), nil
		}
		return b[0] != 0, nil
	case Uint16, Int16:
		b, err := r.take(2)
		if err != nil {
			return nil, err
		}
		if t == Uint16 {
			return binary.LittleEndian.Uint16(b), nil
		}
		return int16(binary.LittleEndian.Uint16(b)), nil
	case Uint32, Int32, Float32:
		v, err := r.u32()
		if err != nil {
			return nil, err
		}
		switch t {
		case Uint32:
			return v, nil
		case Int32:
			return int32(v), nil
		}
		return math.Float32frombits(v), nil
	case Uint64, Int64, Float64:
		v, err := r.u64()
		if err != nil {
			return nil, err
		}
		switch t {
		case Uint64:
			return v, nil
		case Int64:
			return int64(v), nil
		}
		return math.Float64frombits(v), nil
	case String:
		return r.str()
	case Array:
		if depth >= maxArrayNest {
			return nil, errors.New("gguf: arrays nested too deeply")
		}
		et, err := r.u32()
		if err != nil {
			return nil, err
		}
		n, err := r.u64()
		if err != nil {
			return nil, err
		}
		elem := ValueType(et)
		if elem > Float64 {
			return nil, fmt.Errorf("gguf: unknown array element type %d", et)
		}
		if n > r.left()/minSize[elem] {
			return nil, errShort
		}
		if elem == String { // vocabularies: by far the largest arrays
			out := make([]string, n)
			for i := range out {
				if out[i], err = r.str(); err != nil {
					return nil, err
				}
			}
			return out, nil
		}
		out := make([]any, n)
		for i := range out {
			if out[i], err = r.value(elem, depth+1); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("gguf: unknown value type %d", uint32(t))
}

// Parse reads the directory of a GGUF file held in b.
func Parse(b []byte) (*File, error) {
	r := &reader{b: b}
	magic, err := r.u32()
	if err != nil {
		return nil, err
	}
	if magic != Magic {
		return nil, errors.New("gguf: not a GGUF file")
	}
	f := &File{Meta: map[string]any{}, Alignment: defaultAlign}
	if f.Version, err = r.u32(); err != nil {
		return nil, err
	}
	if f.Version != 2 && f.Version != 3 {
		if f.Version&0xffff == 0 {
			return nil, errors.New("gguf: big-endian files are not supported")
		}
		return nil, fmt.Errorf("gguf: unsupported version %d", f.Version)
	}
	nTensors, err := r.u64()
	if err != nil {
		return nil, err
	}
	nKV, err := r.u64()
	if err != nil {
		return nil, err
	}
	// Smallest key-value pair: 8 (key length) + 4 (type) + 1 (value).
	// Smallest tensor record: 8 (name length) + 4 (dims) + 4 (type) + 8 (offset).
	if nKV > r.left()/13 || nTensors > r.left()/24 {
		return nil, errShort
	}
	for i := uint64(0); i < nKV; i++ {
		key, err := r.str()
		if err != nil {
			return nil, err
		}
		t, err := r.u32()
		if err != nil {
			return nil, err
		}
		v, err := r.value(ValueType(t), 0)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", key, err)
		}
		f.Meta[key] = v
	}
	if a, ok := f.Meta["general.alignment"].(uint32); ok {
		if a == 0 || a&(a-1) != 0 || a > 1<<20 {
			return nil, fmt.Errorf("gguf: alignment %d is not a sane power of two", a)
		}
		f.Alignment = uint64(a)
	}
	f.Tensors = make([]Tensor, nTensors)
	for i := range f.Tensors {
		t := &f.Tensors[i]
		if t.Name, err = r.str(); err != nil {
			return nil, err
		}
		nd, err := r.u32()
		if err != nil {
			return nil, err
		}
		if nd > maxDims {
			return nil, fmt.Errorf("gguf: tensor %q has %d dimensions", t.Name, nd)
		}
		t.Dims = make([]uint64, nd)
		elems := uint64(1)
		for d := range t.Dims {
			if t.Dims[d], err = r.u64(); err != nil {
				return nil, err
			}
			hi, lo := bits.Mul64(elems, t.Dims[d])
			if hi != 0 {
				return nil, fmt.Errorf("gguf: tensor %q element count overflows", t.Name)
			}
			elems = lo
		}
		ty, err := r.u32()
		if err != nil {
			return nil, err
		}
		t.Type = TensorType(ty)
		if t.Offset, err = r.u64(); err != nil {
			return nil, err
		}
		lay, ok := Layout[t.Type]
		if !ok {
			return nil, fmt.Errorf("gguf: tensor %q has unsupported type %d", t.Name, ty)
		}
		if elems%lay[0] != 0 {
			return nil, fmt.Errorf("gguf: tensor %q is not a whole number of blocks", t.Name)
		}
		hi, lo := bits.Mul64(elems/lay[0], lay[1])
		if hi != 0 {
			return nil, fmt.Errorf("gguf: tensor %q byte size overflows", t.Name)
		}
		t.Bytes = lo
		if t.Offset%f.Alignment != 0 {
			return nil, fmt.Errorf("gguf: tensor %q is not aligned", t.Name)
		}
	}
	f.DataOffset = (r.off + f.Alignment - 1) / f.Alignment * f.Alignment
	size := uint64(len(b))
	for i := range f.Tensors {
		t := &f.Tensors[i]
		// Written so that no step can overflow.
		if f.DataOffset > size || t.Offset > size-f.DataOffset || t.Bytes > size-f.DataOffset-t.Offset {
			return nil, fmt.Errorf("gguf: tensor %q lies outside the file", t.Name)
		}
	}
	return f, nil
}

// Data returns the bytes of tensor t inside the mapped file b. It shares
// memory with b: no copy is made.
func (f *File) Data(b []byte, t *Tensor) []byte {
	start := f.DataOffset + t.Offset
	return b[start : start+t.Bytes : start+t.Bytes]
}
