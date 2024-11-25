package pgtxdb

import (
	"context"
	"database/sql"
	"fmt"
)

func unwrapConn(db *sql.DB, f func(driverConn *conn)) error {
	sqlConn, err := db.Conn(context.Background())

	if err != nil {
		return err
	}

	defer sqlConn.Close()
	err = sqlConn.Raw(func(driverConn any) error {
		rawConn, ok := driverConn.(*conn)

		if !ok {
			return fmt.Errorf("cannot cast to *pgtxdb.conn: %T", driverConn)
		}

		f(rawConn)

		return nil
	})

	return err
}

func Committed(db *sql.DB) (bool, error) {
	committed := false

	err := unwrapConn(db, func(c *conn) {
		committed = c.committed
	})

	return committed, err
}

func Rolledback(db *sql.DB) (bool, error) {
	rolledback := false

	err := unwrapConn(db, func(c *conn) {
		rolledback = c.rolledback
	})

	return rolledback, err
}
