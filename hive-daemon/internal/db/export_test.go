package db

import "database/sql"

// RawDB exposes the underlying sql.DB for use in tests only.
func (d *DB) RawDB() *sql.DB { return d.sqlDB }
