package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/agentstack/agentstack/internal/catalog"
	"github.com/agentstack/agentstack/internal/supplychain"
)

func main() {
	version := flag.String("version", "dev", "AgentStack version")
	licensesPath := flag.String("licenses", "supply-chain/component-licenses.json", "reviewed license inventory")
	out := flag.String("out", "agentstack-catalog.cdx.json", "CycloneDX output")
	flag.Parse()
	c, err := catalog.LoadDefault()
	if err != nil {
		fatal(err)
	}
	licenses, err := supplychain.LoadLicenses(*licensesPath)
	if err != nil {
		fatal(err)
	}
	bom, err := supplychain.Generate(c, *version, licenses)
	if err != nil {
		fatal(err)
	}
	data, err := json.MarshalIndent(bom, "", "  ")
	if err != nil {
		fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(*out, data, 0o600); err != nil {
		fatal(err)
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "error:", err); os.Exit(1) }
