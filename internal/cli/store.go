package cli

import (
	"context"
	"fmt"

	sqliteadapter "github.com/evanmschultz/valv/internal/adapters/sqlite"
	"github.com/evanmschultz/valv/internal/config"
)

func openStore(paths config.Paths) (*sqliteadapter.Store, error) {
	if err := paths.Ensure(); err != nil {
		return nil, fmt.Errorf("ensure paths: %w", err)
	}

	store, err := sqliteadapter.NewStore(paths.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	if err := store.Bootstrap(context.Background()); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("bootstrap store: %w", err)
	}
	return store, nil
}
