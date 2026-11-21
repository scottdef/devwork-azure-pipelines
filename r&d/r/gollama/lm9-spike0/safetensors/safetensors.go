// Package safetensors reads the directory of a safetensors file: an 8-byte
// little-endian header length, a JSON header, then raw tensor data. Reading
// this format directly is what lets the engine run a model that nobody has
// converted to GGUF yet.
package safetensors

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/bits"
	"sort"
)

// Tensor is one entry in the header. Begin and End are offsets into Data.
type Tensor struct {
	Name  string
	DType string
	Shape []uint64
	Begin uint64
	End   uint64
}

var dtypeSize = map[string]uint64{
	"F64": 8, "F32": 4, "F16": 2, "BF16": 2,
	"I64": 8, "I32": 4, "I16": 2, "I8": 1,
	"U64": 8, "U32": 4, "U16": 2, "U8": 1, "BOOL": 1,
}

const maxHeader = 100 << 20 // the format's own limit on header size

// Parse returns the tensors, sorted by offset, and the data section of b.
func Parse(b []byte) (tensors []Tensor, data []byte, err error) {
	if len(b) < 8 {
		return nil, nil, errors.New("safetensors: file too short")
	}
	n := binary.LittleEndian.Uint64(b)
	if n > maxHeader || n > uint64(len(b)-8) {
		return nil, nil, errors.New("safetensors: header length exceeds the file or the format limit")
	}
	var hdr map[string]json.RawMessage
	if err := json.Unmarshal(b[8:8+n], &hdr); err != nil {
		return nil, nil, fmt.Errorf("safetensors: header: %w", err)
	}
	data = b[8+n:]
	for name, raw := range hdr {
		if name == "__metadata__" {
			continue
		}
		var e struct {
			DType   string    `json:"dtype"`
			Shape   []uint64  `json:"shape"`
			Offsets [2]uint64 `json:"data_offsets"`
		}
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, nil, fmt.Errorf("safetensors: tensor %q: %w", name, err)
		}
		size, ok := dtypeSize[e.DType]
		if !ok {
			return nil, nil, fmt.Errorf("safetensors: tensor %q has unknown dtype %q", name, e.DType)
		}
		for _, d := range e.Shape {
			hi, lo := bits.Mul64(size, d)
			if hi != 0 {
				return nil, nil, fmt.Errorf("safetensors: tensor %q size overflows", name)
			}
			size = lo
		}
		begin, end := e.Offsets[0], e.Offsets[1]
		if begin > end || end > uint64(len(data)) || end-begin != size {
			return nil, nil, fmt.Errorf("safetensors: tensor %q has offsets that do not match its shape or the file", name)
		}
		tensors = append(tensors, Tensor{Name: name, DType: e.DType, Shape: e.Shape, Begin: begin, End: end})
	}
	sort.Slice(tensors, func(i, j int) bool {
		if tensors[i].Begin != tensors[j].Begin {
			return tensors[i].Begin < tensors[j].Begin
		}
		return tensors[i].Name < tensors[j].Name
	})
	for i := 1; i < len(tensors); i++ {
		if tensors[i].Begin < tensors[i-1].End {
			return nil, nil, fmt.Errorf("safetensors: tensors %q and %q overlap", tensors[i-1].Name, tensors[i].Name)
		}
	}
	return tensors, data, nil
}
