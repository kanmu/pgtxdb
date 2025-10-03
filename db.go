/*
Package pgtxdb is a single transaction based database sql driver for PostgreSQL.
When the connection is opened, it starts a transaction and all operations performed on this *sql.DB
will be within that transaction. If concurrent actions are performed, the lock is
acquired and connection is always released the statements and rows are not holding the
connection.

Why is it useful. A very basic use case would be if you want to make functional tests
you can prepare a test database and within each test you do not have to reload a database.
All tests are isolated within transaction and though, performs fast. And you do not have
to interface your sql.DB reference in your code, txdb is like a standard sql.Driver.

This driver supports any sql.Driver connection to be opened. You can register txdb
for different sql drivers and have it under different driver names. Under the hood
whenever a txdb driver is opened, it attempts to open a real connection and starts
transaction. When close is called, it rollbacks transaction leaving your prepared
test database in the same state as before.

Given, you have a mysql database called txdb_test and a table users with a username
column.

Example:

			package main

			import (
				"database/sql"
				"log"

				"github.com/DATA-DOG/go-txdb"
				_ "github.com/go-sql-driver/mysql"
			)

			func init() {
				// we register an sql driver named "txdb"
				txdb.Register("txdb", "mysql", "root@/txdb_test")
			}

			func main() {
	      // dsn serves as an unique identifier for connection pool
				db, err := sql.Open("txdb", "identifier")
				if err != nil {
					log.Fatal(err)
				}
				defer db.Close()

				if _, err := db.Exec(`INSERT INTO users(username) VALUES("gopher")`); err != nil {
					log.Fatal(err)
				}
			}

Every time you will run this application, it will remain in the same state as before.
*/
package pgtxdb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pkg/errors"
)

// Register a txdb sql driver under the given sql driver name
// which can be used to open a single transaction based database
// connection.
//
// When Open is called any number of times it returns
// the same transaction connection. Any Begin, Commit calls
// will not start or close the transaction.
//
// When Close is called, the transaction is rolled back.
//
// Use drv (Driver) and dsn (DataSourceName) as the standard sql properties for
// your test database connection to be isolated within transaction.
//
// The drv and dsn are the same items passed into `sql.Open(drv, dsn)`.
//
// Note: if you open a secondary database, make sure to differianciate
// the dsn string when opening the sql.DB. The transaction will be
// isolated within that dsn
func Register(name, drv, dsn string) {
	register0(name, drv, dsn, false)
}

func RegisterReadOnly(name, drv, dsn string) {
	register0(name, drv, dsn, true)
}

func register0(name, drv, dsn string, readOnly bool) {
	sql.Register(name, &txDriver{
		dsn:      dsn,
		drv:      drv,
		conns:    make(map[string]*conn),
		log:      io.Discard,
		readOnly: readOnly,
	})
}

// txDriver is an sql driver which runs on single transaction
// when the Close is called, transaction is rolled back
type conn struct {
	sync.Mutex
	tx         *sql.Tx
	dsn        string
	opened     int
	drv        *txDriver
	savepoints []int
	log        io.Writer
}

type txDriver struct {
	sync.Mutex
	db    *sql.DB
	conns map[string]*conn
	log   io.Writer

	drv      string
	dsn      string
	readOnly bool
}

type txConnector struct {
	driver *txDriver
	dsn    string
}

func (c *txConnector) Driver() driver.Driver {
	return c.driver
}

func (c *txConnector) Connect(ctx context.Context) (driver.Conn, error) {
	return c.driver.Open(c.dsn)
}

func NewConnector(dsn, srcDrv, srcDsn string, log io.Writer) driver.Connector {
	if log == nil {
		log = io.Discard
	}

	return &txConnector{
		driver: &txDriver{
			dsn:   srcDsn,
			drv:   srcDrv,
			conns: make(map[string]*conn),
			log:   log,
		},
		dsn: dsn,
	}
}

func (d *txDriver) Open(dsn string) (driver.Conn, error) {
	d.Lock()
	defer d.Unlock()
	// first open a real database connection
	var err error
	if d.db == nil {
		if d.drv == "pgx" {
			cc, err := pgx.ParseConfig(d.dsn)
			if err != nil {
				return nil, err
			}
			opts := []stdlib.OptionOpenDB{}
			if d.readOnly {
				opts = append(opts, stdlib.OptionAfterConnect(func(ctx context.Context, c *pgx.Conn) error {
					_, err := c.Exec(ctx, "SET default_transaction_read_only = true")
					return err
				}))
			}
			connector := stdlib.GetConnector(*cc, opts...)
			d.db = sql.OpenDB(connector)
		} else {
			var db *sql.DB
			db, err = sql.Open(d.drv, d.dsn)
			if err != nil {
				return nil, err
			}
			d.db = db
		}
	}
	c, ok := d.conns[dsn]
	if !ok {
		c = &conn{dsn: dsn, drv: d, savepoints: []int{0}, log: d.log}
		fmt.Fprintf(c.log, "%s: open\n", c.dsn)
		c.tx, err = d.db.Begin()
		d.conns[dsn] = c
	}
	c.opened++
	return c, err
}

func (c *conn) Close() (err error) {
	c.drv.Lock()
	defer c.drv.Unlock()
	c.opened--
	if c.opened == 0 {
		err = c.tx.Rollback()
		if err != nil {
			return
		}
		c.tx = nil
		delete(c.drv.conns, c.dsn)
	}
	fmt.Fprintf(c.log, "%s: close\n", c.dsn)
	return
}

func (c *conn) Begin() (driver.Tx, error) {
	savepointID := len(c.savepoints)
	c.savepoints = append(c.savepoints, savepointID)
	sql := fmt.Sprintf("SAVEPOINT pgtxdb_%d", savepointID)
	_, err := c.tx.Exec(sql)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create savepoint")
	}
	fmt.Fprintf(c.log, "%s: begin\n", c.dsn)
	return c, nil
}

func (c *conn) Commit() error {
	fmt.Fprintf(c.log, "%s: commit\n", c.dsn)
	return nil
}

func (c *conn) Rollback() error {
	savepointID := c.savepoints[len(c.savepoints)-1]
	c.savepoints = c.savepoints[:len(c.savepoints)-1]
	sql := fmt.Sprintf("ROLLBACK TO SAVEPOINT pgtxdb_%d", savepointID)
	_, err := c.tx.Exec(sql)
	if err != nil {
		return errors.Wrap(err, "failed to rollback to savepoint")
	}
	fmt.Fprintf(c.log, "%s: rollback\n", c.dsn)
	return nil
}

func (c *conn) Prepare(query string) (driver.Stmt, error) {
	c.Lock()
	defer c.Unlock()

	st, err := c.tx.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &stmt{st: st, dsn: c.dsn, log: c.log}, nil
}

func (c *conn) Exec(query string, args []driver.Value) (driver.Result, error) {
	c.Lock()
	defer c.Unlock()

	fmt.Fprintf(c.log, "%s: exec %s\n", c.dsn, query)
	return c.tx.Exec(query, mapArgs(args)...)
}

func mapArgs(args []driver.Value) (res []interface{}) {
	res = make([]interface{}, len(args))
	for i := range args {
		res[i] = args[i]
	}
	return
}

func (c *conn) Query(query string, args []driver.Value) (driver.Rows, error) {
	c.Lock()
	defer c.Unlock()

	// query rows
	rs, err := c.tx.Query(query, mapArgs(args)...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()

	return buildRows(rs)
}

type stmt struct {
	st  *sql.Stmt
	dsn string
	log io.Writer
}

func (s *stmt) Exec(args []driver.Value) (driver.Result, error) {
	fmt.Fprintf(s.log, "%s: exec prepared statement\n", s.dsn)
	return s.st.Exec(mapArgs(args)...)
}

func (s *stmt) NumInput() int {
	return -1
}

func (s *stmt) Close() error {
	return s.st.Close()
}

func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	rows, err := s.st.Query(mapArgs(args)...)
	if err != nil {
		return nil, err
	}
	return buildRows(rows)
}

type rows struct {
	rows [][]driver.Value
	pos  int
	cols []string
}

func (r *rows) Columns() (cols []string) {
	return r.cols
}

func (r *rows) Next(dest []driver.Value) error {
	r.pos++
	if r.pos > len(r.rows) {
		return io.EOF
	}

	for i, val := range r.rows[r.pos-1] {
		dest[i] = *(val.(*interface{}))
	}

	return nil
}

func (r *rows) Close() error {
	return nil
}

func (r *rows) read(rs *sql.Rows) error {
	var err error
	r.cols, err = rs.Columns()
	if err != nil {
		return err
	}
	for rs.Next() {
		values := make([]interface{}, len(r.cols))
		for i := range values {
			values[i] = new(interface{})
		}
		if err := rs.Scan(values...); err != nil {
			return err
		}
		row := make([]driver.Value, len(r.cols))
		for i, v := range values {
			row[i] = driver.Value(v)
		}
		r.rows = append(r.rows, row)
	}
	return rs.Err()
}

type rowSets struct {
	sets []*rows
	pos  int
}

func (rs *rowSets) Columns() []string {
	return rs.sets[rs.pos].cols
}

func (rs *rowSets) Close() error {
	return nil
}

// advances to next row
func (rs *rowSets) Next(dest []driver.Value) error {
	return rs.sets[rs.pos].Next(dest)
}

func buildRows(r *sql.Rows) (driver.Rows, error) {
	set := &rowSets{}
	rs := &rows{}
	if err := rs.read(r); err != nil {
		return set, err
	}
	set.sets = append(set.sets, rs)
	for r.NextResultSet() {
		rss := &rows{}
		if err := rss.read(r); err != nil {
			return set, err
		}
		set.sets = append(set.sets, rss)
	}
	return set, nil
}

// Implement the "RowsNextResultSet" interface
func (rs *rowSets) HasNextResultSet() bool {
	return rs.pos+1 < len(rs.sets)
}

// Implement the "RowsNextResultSet" interface
func (rs *rowSets) NextResultSet() error {
	if !rs.HasNextResultSet() {
		return io.EOF
	}

	rs.pos++
	return nil
}

// Implement the "QueryerContext" interface
func (c *conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.Lock()
	defer c.Unlock()

	rs, err := c.tx.QueryContext(ctx, query, mapNamedArgs(args)...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()

	return buildRows(rs)
}

// Implement the "ExecerContext" interface
func (c *conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.Lock()
	defer c.Unlock()

	fmt.Fprintf(c.log, "%s: exec %s\n", c.dsn, query)
	return c.tx.ExecContext(ctx, query, mapNamedArgs(args)...)
}

// Implement the "ConnBeginTx" interface
func (c *conn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	savepointID := len(c.savepoints)
	c.savepoints = append(c.savepoints, savepointID)
	sql := fmt.Sprintf("SAVEPOINT pgtxdb_%d", savepointID)
	_, err := c.tx.Exec(sql)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create savepoint")
	}
	fmt.Fprintf(c.log, "%s: begin\n", c.dsn)
	return c, nil
}

// Implement the "ConnPrepareContext" interface
func (c *conn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	c.Lock()
	defer c.Unlock()

	st, err := c.tx.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return &stmt{st: st, dsn: c.dsn, log: c.log}, nil
}

// Implement the "Pinger" interface
func (c *conn) Ping(ctx context.Context) error {
	return c.drv.db.PingContext(ctx)
}

// Implement the "StmtExecContext" interface
func (s *stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	fmt.Fprintf(s.log, "%s: exec prepared statement\n", s.dsn)
	return s.st.ExecContext(ctx, mapNamedArgs(args)...)
}

// Implement the "StmtQueryContext" interface
func (s *stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := s.st.QueryContext(ctx, mapNamedArgs(args)...)
	if err != nil {
		return nil, err
	}
	return buildRows(rows)
}

func mapNamedArgs(args []driver.NamedValue) (res []interface{}) {
	res = make([]interface{}, len(args))
	for i := range args {
		name := args[i].Name
		if name != "" {
			res[i] = sql.Named(name, args[i].Value)
		} else {
			res[i] = args[i].Value
		}
	}
	return
}
