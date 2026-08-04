package notify_test

import (
	"testing"

	"github.com/agentstack/agentstack/internal/notify"
)

func TestNotifyInfoAndErrorSafety(t *testing.T) {
	t.Run("Info notification call safety", func(t *testing.T) {
		// Should execute safely without panic or runtime panic
		notify.Info("Test Title", "Test Information Message")
	})

	t.Run("Error notification call safety", func(t *testing.T) {
		// Should execute safely without panic or runtime panic
		notify.Error("Test Error Title", "Test Error Message")
	})
}
