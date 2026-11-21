package safetensors

import (
	"encoding/binary"
	"testing"
)

func file(header string, dataLen int) []byte {
	b := binary.LittleEndian.AppendUint64(nil, uint64(len(header)))
	b = append(b, header...)
	return append(b, make([]byte, dataLen)...)
}

const good = `{"__metadata__":{"format":"pt"},
 "model.embed.weight":{"dtype":"F16","shape":[4,8],"data_offsets":[0,64]},
 "model.norm.weight":{"dtype":"F32","shape":[8],"data_offsets":[64,96]}}`

func TestParse(t *testing.T) {
	ts, data, err := Parse(file(good, 96))
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 2 || len(data) != 96 || ts[0].Name != "model.embed.weight" || ts[1].End != 96 {
		t.Fatalf("%+v, %d bytes of data", ts, len(data))
	}
}

func TestRejects(t *testing.T) {
	for name, b := range map[string][]byte{
		"data shorter than offsets": file(good, 95),
		"shape and offsets differ":  file(`{"a":{"dtype":"F32","shape":[3],"data_offsets":[0,8]}}`, 8),
		"overlap":                   file(`{"a":{"dtype":"U8","shape":[8],"data_offsets":[0,8]},"b":{"dtype":"U8","shape":[8],"data_offsets":[4,12]}}`, 12),
		"unknown dtype":             file(`{"a":{"dtype":"F128","shape":[1],"data_offsets":[0,16]}}`, 16),
		"size overflow":             file(`{"a":{"dtype":"F64","shape":[4611686018427387904,4],"data_offsets":[0,0]}}`, 0),
		"header longer than file":   binary.LittleEndian.AppendUint64(nil, 1<<40),
		"not JSON":                  file(`not json`, 0),
	} {
		if _, _, err := Parse(b); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	whole := file(good, 96)
	for n := 0; n < len(whole); n++ {
		if _, _, err := Parse(whole[:n]); err == nil {
			t.Fatalf("file truncated to %d of %d bytes was accepted", n, len(whole))
		}
	}
}
