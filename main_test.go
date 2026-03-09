package pgtxdb_test

import (
	"database/sql"
	"log"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx
)

// TestMain service package setup/teardonw
func TestMain(m *testing.M) {
	db, err := sql.Open("pgx", "postgres://pgtxdbtest@localhost:5432/pgtxdbtest?sslmode=disable")
	if err != nil {
		log.Fatalf("failed to connect test db: %s", err.Error())
	}
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS app_user (
	  id BIGSERIAL NOT NULL,
	  username TEXT NOT NULL,
	  email TEXT NOT NULL,
	  PRIMARY KEY (id),
	  UNIQUE (email)
	);
	CREATE TABLE IF NOT EXISTS error_event (
	  id BIGSERIAL NOT NULL,
	  message TEXT NOT NULL,
	  UNIQUE (id)
	);
	CREATE TABLE IF NOT EXISTS test_inet (
	  id BIGSERIAL PRIMARY KEY,
	  ip INET NOT NULL,
	  network CIDR
	);
	`)
	if err != nil {
		log.Fatalf("failed to create test table: %s", err.Error())
	}
	code := m.Run()
	_, err = db.Exec(`
	DROP TABLE IF EXISTS app_user;
	DROP TABLE IF EXISTS error_event;
	DROP TABLE IF EXISTS test_inet;
	`)
	if err != nil {
		log.Fatalf("failed to create test table: %s", err.Error())
	}
	os.Exit(code)
}
