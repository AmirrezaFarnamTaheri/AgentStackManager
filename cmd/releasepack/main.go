package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/agentstack/agentstack/internal/releasepack"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("releasepack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "root directory")
	out := flags.String("out", "", "output ZIP")
	prefix := flags.String("prefix", "", "archive prefix")
	manifestMode := flags.String("manifest-mode", "none", "source manifest mode: none, write, verify, or require")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "error: unexpected positional argument %q\n", flags.Arg(0))
		return 2
	}
	if *root == "" {
		fmt.Fprintln(stderr, "error: --root is required")
		return 2
	}

	var err error
	switch *manifestMode {
	case "none":
		if *out == "" {
			err = fmt.Errorf("--out is required when --manifest-mode=none")
			break
		}
		err = releasepack.Pack(*root, *out, *prefix)
	case "write":
		var verification releasepack.SourceVerification
		verification, err = releasepack.WriteSourceManifest(*root)
		if err == nil {
			fmt.Fprintf(stdout, "wrote %s for %d files at %s\n", releasepack.SourceManifestName, verification.FileCount, verification.Revision)
		}
	case "verify":
		var verification releasepack.SourceVerification
		verification, err = releasepack.VerifySourceClosure(*root)
		if err == nil {
			fmt.Fprintf(stdout, "verified %d source files at %s\n", verification.FileCount, verification.Revision)
		}
	case "require":
		if *out == "" {
			err = fmt.Errorf("--out is required when --manifest-mode=require")
			break
		}
		var verification releasepack.SourceVerification
		verification, err = releasepack.PackVerifiedSource(*root, *out, *prefix)
		if err == nil {
			fmt.Fprintf(stdout, "packed %d verified source files at %s\n", verification.FileCount, verification.Revision)
		}
	default:
		err = fmt.Errorf("invalid --manifest-mode %q", *manifestMode)
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}
