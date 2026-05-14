package storage

import (
	"database/sql"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

type Store struct {
	db *sql.DB
}

func NewStore(dsn string) (*Store, error) {
	if err := ensureDatabaseExists(dsn); err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) Ping() error {
	return s.db.Ping()
}

func (s *Store) Close() error {
	return s.db.Close()
}

func ensureDatabaseExists(dsn string) error {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("invalid MySQL DSN: %w", err)
	}
	if cfg.DBName == "" {
		return fmt.Errorf("MySQL DSN must include a database name")
	}

	dbName := cfg.DBName
	cfg.DBName = ""

	adminDB, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}
	defer adminDB.Close()

	query := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", dbName)
	if _, err := adminDB.Exec(query); err != nil {
		return fmt.Errorf("create database %s: %w", dbName, err)
	}

	return nil
}
