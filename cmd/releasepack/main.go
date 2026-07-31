package main

import (
	"flag"
	"fmt"
	"github.com/agentstack/agentstack/internal/releasepack"
	"os"
)

func main() {
	root := flag.String("root", "", "root directory")
	out := flag.String("out", "", "output ZIP")
	prefix := flag.String("prefix", "", "archive prefix")
	flag.Parse()
	if *root == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "error: --root and --out are required")
		os.Exit(2)
	}
	if err := releasepack.Pack(*root, *out, *prefix); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
