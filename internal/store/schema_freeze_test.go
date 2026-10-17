package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// A store from a newer build must be refused, and refused *without being touched*:
// an older binary that starts adding the columns it thinks are missing would
// corrupt a schema it cannot read. The version has to be checked before any DDL.
func TestOpenRefusesNewerSchemaWithoutTouchingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE schema_meta (version INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO schema_meta(version) VALUES (?)`, SchemaVersion+1); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(path); err == nil {
		t.Fatal("expected Open to refuse a store from a newer schema")
	} else if !strings.Contains(err.Error(), "newer") {
		t.Fatalf("the error should name the problem: %v", err)
	}

	// Nothing this build knows about may exist in there afterwards.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	for _, table := range []string{"samples", "recommendations", "node_snapshots"} {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("refused store was modified: %s was created", table)
		}
	}

	var ver int
	if err := db.QueryRow(`SELECT version FROM schema_meta LIMIT 1`).Scan(&ver); err != nil {
		t.Fatal(err)
	}
	if ver != SchemaVersion+1 {
		t.Fatalf("version rewritten: got %d want %d", ver, SchemaVersion+1)
	}
}

// A store at the current version opens without rewriting anything.
func TestOpenAcceptsCurrentSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = again.Close() }()

	ver, err := again.SchemaVersionOf(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if ver != SchemaVersion {
		t.Fatalf("version: got %d want %d", ver, SchemaVersion)
	}
}
