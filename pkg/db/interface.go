package db

import "database/sql"

// DBProvider is an interface for database clients that provide access to a sql.DB handle.
// This allows both PostgresClient and SupabaseClient to be used interchangeably.
type DBProvider interface {
	DB() *sql.DB
}


