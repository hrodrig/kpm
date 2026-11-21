package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Backup writes a consistent copy of the database to destPath using
// SQLite VACUUM INTO (safe while the store is open). The file is materialized
// inside a private 0700 temp directory and chmod'd to 0600 before atomic rename
// so the destination is never exposed under a broader umask.
func (s *Store) Backup(ctx context.Context, destPath string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store: not open")
	}
	dest := filepath.Clean(destPath)
	if dest == "" || dest == "." {
		return fmt.Errorf("backup destination must be non-empty")
	}
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("backup destination already exists: %s", dest)
	}
	tmpDir, err := os.MkdirTemp(parent, ".kpm-backup-*")
	if err != nil {
		return fmt.Errorf("create backup staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tmp := filepath.Join(tmpDir, "backup.db")
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, tmp); err != nil {
		return fmt.Errorf("vacuum into: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return fmt.Errorf("chmod backup: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return fmt.Errorf("install backup: %w", err)
	}
	return nil
}
