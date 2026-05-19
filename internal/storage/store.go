package storage

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

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

func (s *Store) CreateTable(table string, columns map[string]string) (string, error) {
	query, err := BuildCreateTableQuery(table, columns)
	if err != nil {
		return "", err
	}

	_, err = s.db.Exec(query)
	return query, err
}

func (s *Store) DropTable(name string) (string, error) {
	if !isValidIdentifier(name) {
		return "", fmt.Errorf("invalid table name")
	}

	query := fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdentifier(name))
	_, err := s.db.Exec(query)
	return query, err
}

func (s *Store) DropDatabase(name string) (string, error) {
	if !isValidIdentifier(name) {
		return "", fmt.Errorf("invalid database name")
	}

	query := fmt.Sprintf("DROP DATABASE %s", quoteIdentifier(name))
	_, err := s.db.Exec(query)
	return query, err
}

func (s *Store) Exec(query string) error {
	_, err := s.db.Exec(query)
	return err
}

func (s *Store) Select(query string) ([]map[string]any, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	results := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}

		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}

		row := make(map[string]any, len(columns))
		for i, column := range columns {
			row[column] = normalizeDBValue(values[i])
		}
		results = append(results, row)
	}

	return results, rows.Err()
}

func (s *Store) ExportSnapshot() ([]string, error) {
	tables, err := s.listTables()
	if err != nil {
		return nil, err
	}

	queries := make([]string, 0)
	for _, table := range tables {
		createQuery, err := s.showCreateTable(table)
		if err != nil {
			return nil, err
		}
		queries = append(queries, createQuery)

		insertQueries, err := s.exportTableRows(table)
		if err != nil {
			return nil, err
		}
		queries = append(queries, insertQueries...)
	}

	return queries, nil
}

func (s *Store) ReplaceWithSnapshot(queries []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.Exec("SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		return err
	}

	tables, err := s.listTablesTx(tx)
	if err != nil {
		return err
	}
	for _, table := range tables {
		if _, err := tx.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdentifier(table))); err != nil {
			return err
		}
	}

	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		if _, err := tx.Exec(query); err != nil {
			return fmt.Errorf("apply snapshot query %q: %w", truncateForError(query), err)
		}
	}

	if _, err := tx.Exec("SET FOREIGN_KEY_CHECKS = 1"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	committed = true
	return nil
}

func BuildCreateTableQuery(table string, columns map[string]string) (string, error) {
	if !isValidIdentifier(table) {
		return "", fmt.Errorf("invalid table name")
	}
	if len(columns) == 0 {
		return "", fmt.Errorf("at least one column is required")
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
			return "", fmt.Errorf("invalid column name: %s", name)
		}
		if !isSafeColumnDefinition(definition) {
			return "", fmt.Errorf("invalid column definition for %s", name)
		}

		definitions = append(definitions, fmt.Sprintf("%s %s", quoteIdentifier(name), definition))
	}

	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", quoteIdentifier(table), strings.Join(definitions, ", ")), nil
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

	query := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", quoteIdentifier(dbName))
	if _, err := adminDB.Exec(query); err != nil {
		return fmt.Errorf("create database %s: %w", dbName, err)
	}

	return nil
}

func (s *Store) listTables() ([]string, error) {
	rows, err := s.db.Query("SHOW TABLES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTableNames(rows)
}

func (s *Store) listTablesTx(tx *sql.Tx) ([]string, error) {
	rows, err := tx.Query("SHOW TABLES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTableNames(rows)
}

func scanTableNames(rows *sql.Rows) ([]string, error) {
	tables := make([]string, 0)
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Strings(tables)
	return tables, nil
}

func (s *Store) showCreateTable(table string) (string, error) {
	row := s.db.QueryRow(fmt.Sprintf("SHOW CREATE TABLE %s", quoteIdentifier(table)))

	var name string
	var createQuery string
	if err := row.Scan(&name, &createQuery); err != nil {
		return "", err
	}

	return createQuery, nil
}

func (s *Store) exportTableRows(table string) ([]string, error) {
	rows, err := s.db.Query(fmt.Sprintf("SELECT * FROM %s", quoteIdentifier(table)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	if len(columns) == 0 {
		return nil, nil
	}

	quotedColumns := make([]string, len(columns))
	for i, column := range columns {
		quotedColumns[i] = quoteIdentifier(column)
	}

	queries := make([]string, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}

		formattedValues := make([]string, len(values))
		for i, value := range values {
			formattedValues[i] = formatSQLValue(value)
		}

		queries = append(queries, fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES (%s)",
			quoteIdentifier(table),
			strings.Join(quotedColumns, ", "),
			strings.Join(formattedValues, ", "),
		))
	}

	return queries, rows.Err()
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

func quoteIdentifier(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}

func formatSQLValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "NULL"
	case []byte:
		return quoteSQLString(string(typed))
	case string:
		return quoteSQLString(typed)
	case bool:
		if typed {
			return "1"
		}
		return "0"
	case time.Time:
		return quoteSQLString(typed.Format("2006-01-02 15:04:05.999999"))
	case int:
		return strconv.Itoa(typed)
	case int8, int16, int32, int64:
		return fmt.Sprintf("%d", typed)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", typed)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return quoteSQLString(fmt.Sprint(typed))
	}
}

func quoteSQLString(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `'`, `''`)
	return "'" + replacer.Replace(value) + "'"
}

func truncateForError(query string) string {
	const limit = 120
	if len(query) <= limit {
		return query
	}
	return query[:limit] + "..."
}

func normalizeDBValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	default:
		return typed
	}
}
