package testutils

import (
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/migrations"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func GetTestDB() *sqlx.DB {
	db, err := sqlx.Connect("sqlite", "file::memory:?_pragma=foreign_keys=ON")
	if err != nil {
		panic(err)
	}

	if err := goose.SetDialect("sqlite3"); err != nil {
		panic(err)
	}

	goose.SetBaseFS(migrations.MigrationFS)
	if err := goose.Up(db.DB, "."); err != nil {
		panic(err)
	}

	config.SetSQLite(db)
	return db
}
