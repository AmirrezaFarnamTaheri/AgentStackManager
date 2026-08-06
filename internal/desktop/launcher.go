package desktop

import "context"

// Launch opens the AgentStack loopback UI in a dedicated desktop application
// window and blocks until the window closes or the context is cancelled.
func Launch(ctx context.Context, target string) error {
	return launch(ctx, target)
}

func appArguments(target, dataPath string) []string {
	return []string{
		"--app=" + target,
		"--user-data-dir=" + dataPath,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-mode",
		"--disable-component-update",
		"--disable-default-apps",
		"--disable-features=msEdgeSidebarV2,msEdgeWallet,msEdgeShopping",
	}
}
