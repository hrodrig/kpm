package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Backup writes a consistent copy of the database to destPath using
// SQLite VACUUM INTO (safe while the store is open).
func (s *Store) Backup(ctx context.Context, destPath string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store: not open")
	}
	dest := filepath.Clean(destPath)
	if dest == "" || dest == "." {
		return fmt.Errorf("backup destination must be non-empty")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("backup destination already exists: %s", dest)
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, dest); err != nil {
		return fmt.Errorf("vacuum into: %w", err)
	}
	return nil
}
