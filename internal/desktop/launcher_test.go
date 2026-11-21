package desktop

import (
	"strings"
	"testing"
)

func TestAppArgumentsCreateDedicatedAddressBarFreeWindow(t *testing.T) {
	args := appArguments("http://127.0.0.1:1234/session/token/", `C:\Data\AgentStack\Desktop`)
	joined := strings.Join(args, "\n")
	for _, required := range []string{
		"--app=http://127.0.0.1:1234/session/token/",
		`--user-data-dir=C:\Data\AgentStack\Desktop`,
		"--no-first-run",
		"--disable-background-mode",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("desktop arguments missing %q: %v", required, args)
		}
	}
	for _, forbidden := range []string{"--new-window", "--incognito"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("desktop arguments contain browser-window flag %q: %v", forbidden, args)
		}
	}
}
