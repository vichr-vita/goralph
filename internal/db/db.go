package db

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

const DriverName = "sqlite"

// Open opens a SQLite database using the pure-Go modernc.org/sqlite driver.
func Open(path string) (*sql.DB, error) {
	return sql.Open(DriverName, path)
}
