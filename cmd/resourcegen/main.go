package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/agentstack/agentstack/internal/winresource"
)

func main() {
	iconPath := flag.String("icon", "", "input .ico file")
	manifestPath := flag.String("manifest", "", "optional input manifest")
	outputPath := flag.String("out", "", "output AMD64 .syso file")
	flag.Parse()
	if *iconPath == "" || *outputPath == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: resourcegen -icon app.ico [-manifest app.manifest] -out icon_windows_amd64.syso")
		os.Exit(2)
	}
	icon, err := os.ReadFile(*iconPath)
	if err != nil {
		fatal(err)
	}
	var manifest []byte
	if *manifestPath != "" {
		manifest, err = os.ReadFile(*manifestPath)
		if err != nil {
			fatal(err)
		}
	}
	object, err := winresource.BuildAMD64Object(icon, manifest)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*outputPath, object, 0o600); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "resourcegen:", err)
	os.Exit(1)
}
