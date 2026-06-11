package db

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

const DriverName = "sqlite"

// Open opens a SQLite database using the pure-Go modernc.org/sqlite driver.
func Open(path string) (*sql.DB, error) {
	database, err := sql.Open(DriverName, path)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	for _, pragma := range []string{"PRAGMA busy_timeout = 5000", "PRAGMA foreign_keys = ON"} {
		if _, err := database.Exec(pragma); err != nil {
			database.Close()
			return nil, err
		}
	}
	return database, nil
}
