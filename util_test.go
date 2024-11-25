package pgtxdb_test

import (
	"database/sql"
	"testing"

	"github.com/kanmu/pgtxdb"
)

func TestRollback(t *testing.T) {
	db, err := sql.Open("pgtxdb", "one")
	if err != nil {
		t.Fatalf("failed to open a postgres connection, have you run 'make test'? err: %s", err)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin the transaction: %s", err)
	}

	_, err = tx.Exec(`INSERT INTO app_user(username, email) VALUES('txdb', 'txdb@test.com')`)
	if err != nil {
		t.Fatalf("failed to insert an app_user: %s", err)
	}
	tx.Rollback()
	tx.Commit()

	committed, err := pgtxdb.Committed(db)
	if err != nil {
		t.Fatalf("failed to checke commit: %s", err)
	}
	if committed {
		t.Error("unexpected commit")
	}

	rolledback, err := pgtxdb.Rolledback(db)
	if err != nil {
		t.Fatalf("failed to checke rolledback: %s", err)
	}
	if !rolledback {
		t.Error("unexpected no rollback")
	}
}

func TestCommit(t *testing.T) {
	t.Cleanup(func() {
		db, _ := sql.Open("pgx", "postgres://pgtxdbtest@localhost:5432/pgtxdbtest?sslmode=disable")
		db.Exec("TRUNCATE TABLE app_user")
	})

	db, err := sql.Open("pgtxdb", "one")
	if err != nil {
		t.Fatalf("failed to open a postgres connection, have you run 'make test'? err: %s", err)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin the transaction: %s", err)
	}

	_, err = tx.Exec(`INSERT INTO app_user(username, email) VALUES('txdb', 'txdb@test.com')`)
	if err != nil {
		t.Fatalf("failed to insert an app_user: %s", err)
	}
	tx.Commit()
	tx.Rollback()

	committed, err := pgtxdb.Committed(db)
	if err != nil {
		t.Fatalf("failed to checke commit: %s", err)
	}
	if !committed {
		t.Error("unexpected no commit")
	}

	rolledback, err := pgtxdb.Rolledback(db)
	if err != nil {
		t.Fatalf("failed to checke rolledback: %s", err)
	}
	if rolledback {
		t.Error("unexpected rollback")
	}
}

func TestNoCommitNoRollback(t *testing.T) {
	db, err := sql.Open("pgtxdb", "one")
	if err != nil {
		t.Fatalf("failed to open a postgres connection, have you run 'make test'? err: %s", err)
	}
	defer db.Close()

	_, err = db.Exec(`INSERT INTO app_user(username, email) VALUES('txdb', 'txdb@test.com')`)
	if err != nil {
		t.Fatalf("failed to insert an app_user: %s", err)
	}

	committed, err := pgtxdb.Committed(db)
	if err != nil {
		t.Fatalf("failed to checke commit: %s", err)
	}
	if committed {
		t.Error("unexpected commit")
	}

	rolledback, err := pgtxdb.Rolledback(db)
	if err != nil {
		t.Fatalf("failed to checke rolledback: %s", err)
	}
	if rolledback {
		t.Error("unexpected rollback")
	}
}
