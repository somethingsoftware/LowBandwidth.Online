package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func NewDB() (*sql.DB, error) {
	var (
		dbHost = os.Getenv("PG_HOST")
		dbPort = os.Getenv("PG_PORT")
		dbUser = os.Getenv("PG_USER")
		dbPass = os.Getenv("PG_PASS")
		dbName = os.Getenv("PG_NAME")
		db     *sql.DB
		err    error
	)
	if dbHost == "" || dbPort == "" || dbUser == "" || dbPass == "" || dbName == "" {
		return db, fmt.Errorf("missing one or more required environment variables " +
			"(PG_HOST, PG_PORT, PG_USER, PG_PASS, PG_NAME)")
	}

	// #region Load db
	psqlConfig := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPass, dbName,
	)
	db, err = sql.Open("postgres", psqlConfig)
	if err != nil {
		return db, fmt.Errorf("error connecting to database: %w", err)
	}
	if err = db.Ping(); err != nil {
		return db, fmt.Errorf("error pinging database: %w", err)
	}
	return db, nil
}
