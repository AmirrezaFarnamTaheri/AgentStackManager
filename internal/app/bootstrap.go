package app

import (
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/agentstack/agentstack/internal/state"
)

// prepareStore serializes startup maintenance with every other mutation. A
// concurrently running instance owns recovery decisions for its transaction;
// startup must not reclassify that live work as interrupted.
func prepareStore(store state.Store, now time.Time) (resultErr error) {
	lease, err := store.AcquireLease("mutation", 6*time.Hour)
	if errors.Is(err, state.ErrMutationBusy) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("acquire startup recovery lease: %w", err)
	}
	defer func() {
		if closeErr := lease.Close(); resultErr == nil && closeErr != nil {
			resultErr = fmt.Errorf("release startup recovery lease: %w", closeErr)
		}
	}()

	if _, err := store.RecoverIncompleteTransactions(); err != nil {
		return fmt.Errorf("recover incomplete transactions: %w", err)
	}
	if _, err := store.Prune(now, state.DefaultRetentionPolicy()); err != nil {
		return fmt.Errorf("apply data retention policy: %w", err)
	}
	return nil
}

type defaultLocator struct{}

func (defaultLocator) LookPath(name string) (string, error) { return exec.LookPath(name) }
