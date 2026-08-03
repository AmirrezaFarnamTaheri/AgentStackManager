package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/agentstack/agentstack/internal/catalog"
	"github.com/agentstack/agentstack/internal/supplychain"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("agentstack-sbom", flag.ContinueOnError)
	flags.SetOutput(stderr)
	version := flags.String("version", "dev", "AgentStack version")
	licensesPath := flags.String("licenses", "supply-chain/component-licenses.json", "reviewed license inventory")
	out := flags.String("out", "agentstack-catalog.cdx.json", "CycloneDX output")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "error: unexpected positional argument %q\n", flags.Arg(0))
		return 2
	}
	c, err := catalog.LoadDefault()
	if err != nil {
		return fail(stderr, err)
	}
	licenses, err := supplychain.LoadLicenses(*licensesPath)
	if err != nil {
		return fail(stderr, err)
	}
	bom, err := supplychain.Generate(c, *version, licenses)
	if err != nil {
		return fail(stderr, err)
	}
	data, err := json.MarshalIndent(bom, "", "  ")
	if err != nil {
		return fail(stderr, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(*out, data, 0o600); err != nil {
		return fail(stderr, err)
	}
	return 0
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "error:", err)
	return 1
}
