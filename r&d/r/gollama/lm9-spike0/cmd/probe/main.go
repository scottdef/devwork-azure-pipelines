// Command probe prints the hardware report as JSON: the same data `lm9 probe`
// and the /probe file serve.
package main

import (
	"encoding/json"
	"os"

	"lm9spike/probe"
)

func main() {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(probe.Run()); err != nil {
		os.Exit(1)
	}
}
