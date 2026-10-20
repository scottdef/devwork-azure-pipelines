// Command hello greets someone, Plan 9 style.
package main

import (
	"flag"
	"fmt"
	"os"
)

var (
	greeting = flag.String("greeting", "hello", "greeting word")
	who      = flag.String("who", "world", "who to greet")
)

func main() {
	flag.Parse()
	if flag.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "usage: hello [-greeting] [-who]")
		os.Exit(2)
	}
	fmt.Printf("%s, %s\n", *greeting, *who)
}
