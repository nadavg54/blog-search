package db

import "database/sql"

// DBProvider is an interface for database clients that provide access to a sql.DB handle.
type DBProvider interface {
	DB() *sql.DB
}
