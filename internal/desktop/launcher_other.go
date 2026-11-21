//go:build !windows

package desktop

import (
	"context"
	"fmt"
)

func launch(context.Context, string) error {
	return fmt.Errorf("the unified AgentStack desktop host is currently available on Windows")
}
