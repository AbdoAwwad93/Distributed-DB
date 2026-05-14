package storage

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/go-sql-driver/mysql"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

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

func (s *Store) CreateTable(table string, columns map[string]string) error {
	if !isValidIdentifier(table) {
		return fmt.Errorf("invalid table name")
	}
	if len(columns) == 0 {
		return fmt.Errorf("at least one column is required")
	}

	columnNames := make([]string, 0, len(columns))
	for name := range columns {
		columnNames = append(columnNames, name)
	}
	sort.Strings(columnNames)

	definitions := make([]string, 0, len(columns))
	for _, name := range columnNames {
		definition := strings.TrimSpace(columns[name])
		if !isValidIdentifier(name) {
			return fmt.Errorf("invalid column name: %s", name)
		}
		if !isSafeColumnDefinition(definition) {
			return fmt.Errorf("invalid column definition for %s", name)
		}

		definitions = append(definitions, fmt.Sprintf("%s %s", name, definition))
	}

	query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", table, strings.Join(definitions, ", "))
	_, err := s.db.Exec(query)
	return err
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

func isValidIdentifier(value string) bool {
	return identifierPattern.MatchString(value)
}

func isSafeColumnDefinition(value string) bool {
	if value == "" {
		return false
	}

	unsafeTokens := []string{";", "--", "/*", "*/"}
	for _, token := range unsafeTokens {
		if strings.Contains(value, token) {
			return false
		}
	}

	return true
}