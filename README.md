# pgtxdb

[![test](https://github.com/kanmu/pgtxdb/actions/workflows/test.yml/badge.svg)](https://github.com/kanmu/pgtxdb/actions/workflows/test.yml)
[![GitHub license](https://img.shields.io/badge/license-MIT-blue.svg)](https://raw.githubusercontent.com/kanmu/pgtxdb/master/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/kanmu/pgtxdb)](https://goreportcard.com/report/github.com/kanmu/pgtxdb)

## Description

Single transaction sql driver for Golang x PostgreSQL. This is almost clone of [go-txdb](https://github.com/DATA-DOG/go-txdb) with a bit of PostgreSQL tweeks.

- When `conn.Begin()` is called, this library executes `SAVEPOINT pgtxdb_xxx;` instead of actually begins transaction.
- `tx.Commit()` does nothing.
- `ROLLBACK TO SAVEPOINT pgtxdb_xxx;` will be executed upon `tx.Rollback()` call so that it can emulate transaction rollback.
- Above features enable us to emulate multiple transactions in one test case.


## Run test

```
docker compose up -d
make test
```
