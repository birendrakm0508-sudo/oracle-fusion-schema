// Package db provides SQLite storage for Oracle Fusion schema documentation.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/birendrakm0508-sudo/oracle-fusion-schema/internal/model"
	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database connection for schema storage.
type Store struct {
	db   *sql.DB
	path string
	mu   sync.Mutex // Serialize write transactions for concurrent access
}

// DefaultDBPath returns the default database file path.
func DefaultDBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".oracle-fusion-schema", "schema.db")
}

// Open opens or creates the SQLite database.
func Open(dbPath string) (*Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	s := &Store{db: db, path: dbPath}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Path returns the database file path.
func (s *Store) Path() string {
	return s.path
}

func (s *Store) migrate() error {
	ddl := `
	CREATE TABLE IF NOT EXISTS domains (
		code TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		base_url TEXT NOT NULL,
		toc_url TEXT NOT NULL,
		last_synced DATETIME
	);

	CREATE TABLE IF NOT EXISTS modules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		domain_code TEXT NOT NULL,
		UNIQUE(name, domain_code),
		FOREIGN KEY (domain_code) REFERENCES domains(code)
	);

	CREATE TABLE IF NOT EXISTS tables (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		domain_code TEXT NOT NULL,
		module TEXT NOT NULL DEFAULT '',
		type TEXT NOT NULL DEFAULT 'TABLE',
		owner TEXT DEFAULT '',
		schema_name TEXT DEFAULT 'FUSION',
		description TEXT DEFAULT '',
		doc_url TEXT DEFAULT '',
		UNIQUE(name, domain_code),
		FOREIGN KEY (domain_code) REFERENCES domains(code)
	);

	CREATE TABLE IF NOT EXISTS columns (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		table_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		data_type TEXT NOT NULL,
		length TEXT DEFAULT '',
		precision TEXT DEFAULT '',
		nullable INTEGER NOT NULL DEFAULT 1,
		description TEXT DEFAULT '',
		position INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (table_id) REFERENCES tables(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS primary_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		table_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		FOREIGN KEY (table_id) REFERENCES tables(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS pk_columns (
		pk_id INTEGER NOT NULL,
		column_name TEXT NOT NULL,
		position INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (pk_id) REFERENCES primary_keys(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS indexes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		table_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		is_unique INTEGER NOT NULL DEFAULT 0,
		tablespace TEXT DEFAULT '',
		FOREIGN KEY (table_id) REFERENCES tables(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS index_columns (
		index_id INTEGER NOT NULL,
		column_name TEXT NOT NULL,
		position INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (index_id) REFERENCES indexes(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS foreign_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		table_id INTEGER NOT NULL,
		referencing_table TEXT NOT NULL,
		fk_column TEXT NOT NULL,
		relationship TEXT DEFAULT '',
		FOREIGN KEY (table_id) REFERENCES tables(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS ebs_mappings (
		ebs_name TEXT PRIMARY KEY,
		fusion_name TEXT NOT NULL,
		module TEXT DEFAULT '',
		notes TEXT DEFAULT '',
		is_custom INTEGER NOT NULL DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_tables_domain ON tables(domain_code);
	CREATE INDEX IF NOT EXISTS idx_tables_name ON tables(name);
	CREATE INDEX IF NOT EXISTS idx_tables_module ON tables(module);
	CREATE INDEX IF NOT EXISTS idx_columns_table ON columns(table_id);
	CREATE INDEX IF NOT EXISTS idx_columns_name ON columns(name);
	`
	_, err := s.db.Exec(ddl)
	return err
}

// UpsertDomain inserts or updates a domain.
func (s *Store) UpsertDomain(d model.Domain) error {
	_, err := s.db.Exec(`INSERT INTO domains (code, name, base_url, toc_url) VALUES (?,?,?,?)
		ON CONFLICT(code) DO UPDATE SET name=excluded.name, base_url=excluded.base_url, toc_url=excluded.toc_url`,
		d.Code, d.Name, d.BaseURL, d.TOCURL)
	return err
}

// MarkDomainSynced updates the last_synced timestamp for a domain.
func (s *Store) MarkDomainSynced(code string) error {
	_, err := s.db.Exec(`UPDATE domains SET last_synced = datetime('now') WHERE code = ?`, code)
	return err
}

// ListDomains returns all domains.
func (s *Store) ListDomains() ([]model.Domain, error) {
	rows, err := s.db.Query(`SELECT code, name, base_url, toc_url FROM domains ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []model.Domain
	for rows.Next() {
		var d model.Domain
		if err := rows.Scan(&d.Code, &d.Name, &d.BaseURL, &d.TOCURL); err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

// DomainTableCount returns the number of tables in a domain.
func (s *Store) DomainTableCount(code string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM tables WHERE domain_code = ?`, code).Scan(&count)
	return count, err
}

// ClearDomain removes all tables and related data for a domain.
func (s *Store) ClearDomain(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete child records via table IDs
	_, err = tx.Exec(`DELETE FROM foreign_keys WHERE table_id IN (SELECT id FROM tables WHERE domain_code = ?)`, code)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM index_columns WHERE index_id IN (SELECT i.id FROM indexes i JOIN tables t ON i.table_id = t.id WHERE t.domain_code = ?)`, code)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM indexes WHERE table_id IN (SELECT id FROM tables WHERE domain_code = ?)`, code)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM pk_columns WHERE pk_id IN (SELECT p.id FROM primary_keys p JOIN tables t ON p.table_id = t.id WHERE t.domain_code = ?)`, code)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM primary_keys WHERE table_id IN (SELECT id FROM tables WHERE domain_code = ?)`, code)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM columns WHERE table_id IN (SELECT id FROM tables WHERE domain_code = ?)`, code)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM modules WHERE domain_code = ?`, code)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM tables WHERE domain_code = ?`, code)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// InsertTable inserts a full table record with columns, PKs, indexes, and FKs.
func (s *Store) InsertTable(t model.Table) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Upsert module
	if t.Module != "" {
		_, err = tx.Exec(`INSERT INTO modules (name, domain_code) VALUES (?,?) ON CONFLICT DO NOTHING`, t.Module, t.Domain)
		if err != nil {
			return err
		}
	}

	// Insert table
	res, err := tx.Exec(`INSERT INTO tables (name, domain_code, module, type, owner, schema_name, description, doc_url)
		VALUES (?,?,?,?,?,?,?,?) ON CONFLICT(name, domain_code) DO UPDATE SET
		module=excluded.module, type=excluded.type, owner=excluded.owner, schema_name=excluded.schema_name,
		description=excluded.description, doc_url=excluded.doc_url`,
		t.Name, t.Domain, t.Module, t.Type, t.Owner, t.Schema, t.Description, t.DocURL)
	if err != nil {
		return fmt.Errorf("insert table %s: %w", t.Name, err)
	}

	tableID, err := res.LastInsertId()
	if err != nil {
		// If ON CONFLICT triggered, get the existing ID
		err = tx.QueryRow(`SELECT id FROM tables WHERE name = ? AND domain_code = ?`, t.Name, t.Domain).Scan(&tableID)
		if err != nil {
			return fmt.Errorf("get table id for %s: %w", t.Name, err)
		}
		// Clear existing child records for update
		tx.Exec(`DELETE FROM columns WHERE table_id = ?`, tableID)
		tx.Exec(`DELETE FROM pk_columns WHERE pk_id IN (SELECT id FROM primary_keys WHERE table_id = ?)`, tableID)
		tx.Exec(`DELETE FROM primary_keys WHERE table_id = ?`, tableID)
		tx.Exec(`DELETE FROM index_columns WHERE index_id IN (SELECT id FROM indexes WHERE table_id = ?)`, tableID)
		tx.Exec(`DELETE FROM indexes WHERE table_id = ?`, tableID)
		tx.Exec(`DELETE FROM foreign_keys WHERE table_id = ?`, tableID)
	}

	// Insert columns
	for _, col := range t.Columns {
		_, err = tx.Exec(`INSERT INTO columns (table_id, name, data_type, length, precision, nullable, description, position)
			VALUES (?,?,?,?,?,?,?,?)`,
			tableID, col.Name, col.DataType, col.Length, col.Precision, boolToInt(col.Nullable), col.Description, col.Position)
		if err != nil {
			return fmt.Errorf("insert column %s.%s: %w", t.Name, col.Name, err)
		}
	}

	// Insert primary key
	if t.PrimaryKey != nil && len(t.PrimaryKey.Columns) > 0 {
		pkRes, err := tx.Exec(`INSERT INTO primary_keys (table_id, name) VALUES (?,?)`, tableID, t.PrimaryKey.Name)
		if err != nil {
			return err
		}
		pkID, _ := pkRes.LastInsertId()
		for i, col := range t.PrimaryKey.Columns {
			_, err = tx.Exec(`INSERT INTO pk_columns (pk_id, column_name, position) VALUES (?,?,?)`, pkID, col, i)
			if err != nil {
				return err
			}
		}
	}

	// Insert indexes
	for _, idx := range t.Indexes {
		idxRes, err := tx.Exec(`INSERT INTO indexes (table_id, name, is_unique, tablespace) VALUES (?,?,?,?)`,
			tableID, idx.Name, boolToInt(idx.Unique), idx.Tablespace)
		if err != nil {
			return err
		}
		idxID, _ := idxRes.LastInsertId()
		for i, col := range idx.Columns {
			_, err = tx.Exec(`INSERT INTO index_columns (index_id, column_name, position) VALUES (?,?,?)`, idxID, col, i)
			if err != nil {
				return err
			}
		}
	}

	// Insert foreign keys
	for _, fk := range t.ForeignKeys {
		_, err = tx.Exec(`INSERT INTO foreign_keys (table_id, referencing_table, fk_column, relationship) VALUES (?,?,?,?)`,
			tableID, fk.ReferencingTable, fk.FKColumn, fk.Relationship)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetTable retrieves a full table by name.
func (s *Store) GetTable(name string) (*model.Table, error) {
	upper := strings.ToUpper(name)
	var t model.Table
	var tableID int64
	err := s.db.QueryRow(`SELECT id, name, domain_code, module, type, owner, schema_name, description, doc_url
		FROM tables WHERE UPPER(name) = ? LIMIT 1`, upper).Scan(
		&tableID, &t.Name, &t.Domain, &t.Module, &t.Type, &t.Owner, &t.Schema, &t.Description, &t.DocURL)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.ID = tableID

	// Load columns
	colRows, err := s.db.Query(`SELECT name, data_type, length, precision, nullable, description, position
		FROM columns WHERE table_id = ? ORDER BY position`, tableID)
	if err != nil {
		return nil, err
	}
	defer colRows.Close()
	for colRows.Next() {
		var c model.Column
		var nullable int
		if err := colRows.Scan(&c.Name, &c.DataType, &c.Length, &c.Precision, &nullable, &c.Description, &c.Position); err != nil {
			return nil, err
		}
		c.Nullable = nullable == 1
		t.Columns = append(t.Columns, c)
	}

	// Load primary keys
	var pkID int64
	var pkName string
	err = s.db.QueryRow(`SELECT id, name FROM primary_keys WHERE table_id = ? LIMIT 1`, tableID).Scan(&pkID, &pkName)
	if err == nil {
		pk := &model.PK{Name: pkName}
		pkColRows, err := s.db.Query(`SELECT column_name FROM pk_columns WHERE pk_id = ? ORDER BY position`, pkID)
		if err == nil {
			defer pkColRows.Close()
			for pkColRows.Next() {
				var col string
				pkColRows.Scan(&col)
				pk.Columns = append(pk.Columns, col)
			}
		}
		t.PrimaryKey = pk
	}

	// Load indexes
	idxRows, err := s.db.Query(`SELECT id, name, is_unique, tablespace FROM indexes WHERE table_id = ?`, tableID)
	if err == nil {
		defer idxRows.Close()
		for idxRows.Next() {
			var idx model.Index
			var idxID int64
			var isUnique int
			idxRows.Scan(&idxID, &idx.Name, &isUnique, &idx.Tablespace)
			idx.Unique = isUnique == 1
			icRows, _ := s.db.Query(`SELECT column_name FROM index_columns WHERE index_id = ? ORDER BY position`, idxID)
			if icRows != nil {
				for icRows.Next() {
					var col string
					icRows.Scan(&col)
					idx.Columns = append(idx.Columns, col)
				}
				icRows.Close()
			}
			t.Indexes = append(t.Indexes, idx)
		}
	}

	// Load foreign keys
	fkRows, err := s.db.Query(`SELECT referencing_table, fk_column, relationship FROM foreign_keys WHERE table_id = ?`, tableID)
	if err == nil {
		defer fkRows.Close()
		for fkRows.Next() {
			var fk model.FK
			fkRows.Scan(&fk.ReferencingTable, &fk.FKColumn, &fk.Relationship)
			t.ForeignKeys = append(t.ForeignKeys, fk)
		}
	}

	return &t, nil
}

// ListTables lists tables with optional filtering.
func (s *Store) ListTables(domain, module, tableType string) ([]model.Table, error) {
	query := `SELECT name, domain_code, module, type, description FROM tables WHERE 1=1`
	var args []interface{}

	if domain != "" {
		query += ` AND UPPER(domain_code) = ?`
		args = append(args, strings.ToUpper(domain))
	}
	if module != "" {
		query += ` AND UPPER(module) LIKE ?`
		args = append(args, "%"+strings.ToUpper(module)+"%")
	}
	if tableType != "" {
		query += ` AND UPPER(type) = ?`
		args = append(args, strings.ToUpper(tableType))
	}
	query += ` ORDER BY domain_code, module, name`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []model.Table
	for rows.Next() {
		var t model.Table
		if err := rows.Scan(&t.Name, &t.Domain, &t.Module, &t.Type, &t.Description); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

// Search performs full-text search across table names, column names, and descriptions.
func (s *Store) Search(query, domain, searchType string, limit int) ([]model.SearchResult, error) {
	if limit <= 0 {
		limit = 50
	}
	pattern := "%" + strings.ToUpper(query) + "%"

	var results []model.SearchResult

	// Search table names
	if searchType == "" || searchType == "table" {
		tq := `SELECT name, domain_code, module, description FROM tables WHERE UPPER(name) LIKE ?`
		var tArgs []interface{}
		tArgs = append(tArgs, pattern)
		if domain != "" {
			tq += ` AND UPPER(domain_code) = ?`
			tArgs = append(tArgs, strings.ToUpper(domain))
		}
		tq += ` ORDER BY name LIMIT ?`
		tArgs = append(tArgs, limit)

		rows, err := s.db.Query(tq, tArgs...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var r model.SearchResult
			rows.Scan(&r.TableName, &r.Domain, &r.Module, &r.Description)
			r.MatchType = "table_name"
			r.MatchField = r.TableName
			r.MatchText = r.TableName
			results = append(results, r)
		}
	}

	// Search column names
	if searchType == "" || searchType == "column" {
		cq := `SELECT DISTINCT c.name, t.name, t.domain_code, t.module, c.description
			FROM columns c JOIN tables t ON c.table_id = t.id
			WHERE UPPER(c.name) LIKE ?`
		var cArgs []interface{}
		cArgs = append(cArgs, pattern)
		if domain != "" {
			cq += ` AND UPPER(t.domain_code) = ?`
			cArgs = append(cArgs, strings.ToUpper(domain))
		}
		cq += ` ORDER BY t.name, c.name LIMIT ?`
		cArgs = append(cArgs, limit)

		rows, err := s.db.Query(cq, cArgs...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var r model.SearchResult
			var colDesc string
			rows.Scan(&r.MatchField, &r.TableName, &r.Domain, &r.Module, &colDesc)
			r.MatchType = "column_name"
			r.MatchText = r.MatchField
			r.Description = colDesc
			results = append(results, r)
		}
	}

	// Search descriptions
	if searchType == "" || searchType == "description" {
		dq := `SELECT name, domain_code, module, description FROM tables WHERE UPPER(description) LIKE ?`
		var dArgs []interface{}
		dArgs = append(dArgs, pattern)
		if domain != "" {
			dq += ` AND UPPER(domain_code) = ?`
			dArgs = append(dArgs, strings.ToUpper(domain))
		}
		dq += ` ORDER BY name LIMIT ?`
		dArgs = append(dArgs, limit)

		rows, err := s.db.Query(dq, dArgs...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var r model.SearchResult
			rows.Scan(&r.TableName, &r.Domain, &r.Module, &r.Description)
			r.MatchType = "description"
			r.MatchField = "description"
			r.MatchText = r.Description
			results = append(results, r)
		}
	}

	return results, nil
}

// SearchColumns searches columns by name pattern.
func (s *Store) SearchColumns(pattern, domain string, exact bool, limit int) ([]struct {
	ColumnName  string
	TableName   string
	Domain      string
	DataType    string
	Description string
}, error) {
	if limit <= 0 {
		limit = 50
	}

	var q string
	var args []interface{}

	if exact {
		q = `SELECT c.name, t.name, t.domain_code, c.data_type, c.description
			FROM columns c JOIN tables t ON c.table_id = t.id
			WHERE UPPER(c.name) = ?`
		args = append(args, strings.ToUpper(pattern))
	} else {
		q = `SELECT c.name, t.name, t.domain_code, c.data_type, c.description
			FROM columns c JOIN tables t ON c.table_id = t.id
			WHERE UPPER(c.name) LIKE ?`
		args = append(args, "%"+strings.ToUpper(pattern)+"%")
	}

	if domain != "" {
		q += ` AND UPPER(t.domain_code) = ?`
		args = append(args, strings.ToUpper(domain))
	}
	q += ` ORDER BY c.name, t.name LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type colResult struct {
		ColumnName  string
		TableName   string
		Domain      string
		DataType    string
		Description string
	}
	var results []colResult
	for rows.Next() {
		var r colResult
		rows.Scan(&r.ColumnName, &r.TableName, &r.Domain, &r.DataType, &r.Description)
		results = append(results, r)
	}

	// Convert to the return type
	type R = struct {
		ColumnName  string
		TableName   string
		Domain      string
		DataType    string
		Description string
	}
	out := make([]R, len(results))
	for i, r := range results {
		out[i] = R(r)
	}
	return out, rows.Err()
}

// TotalTableCount returns the total number of tables across all domains.
func (s *Store) TotalTableCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM tables`).Scan(&count)
	return count, err
}

// UpsertEBSMapping inserts or updates a custom EBS mapping.
func (s *Store) UpsertEBSMapping(m model.EBSMapping, isCustom bool) error {
	custom := 0
	if isCustom {
		custom = 1
	}
	_, err := s.db.Exec(`INSERT INTO ebs_mappings (ebs_name, fusion_name, module, notes, is_custom)
		VALUES (?,?,?,?,?) ON CONFLICT(ebs_name) DO UPDATE SET
		fusion_name=excluded.fusion_name, module=excluded.module, notes=excluded.notes, is_custom=excluded.is_custom`,
		m.EBSName, m.FusionName, m.Module, m.Notes, custom)
	return err
}

// GetCustomMappings returns all custom EBS mappings from the database.
func (s *Store) GetCustomMappings() ([]model.EBSMapping, error) {
	rows, err := s.db.Query(`SELECT ebs_name, fusion_name, module, notes FROM ebs_mappings WHERE is_custom = 1 ORDER BY ebs_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mappings []model.EBSMapping
	for rows.Next() {
		var m model.EBSMapping
		rows.Scan(&m.EBSName, &m.FusionName, &m.Module, &m.Notes)
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}

// ListModules returns modules for a domain.
func (s *Store) ListModules(domain string) ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT module FROM tables WHERE UPPER(domain_code) = ? AND module != '' ORDER BY module`,
		strings.ToUpper(domain))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var modules []string
	for rows.Next() {
		var m string
		rows.Scan(&m)
		modules = append(modules, m)
	}
	return modules, rows.Err()
}

// TableExists checks if a table exists in the schema DB (case-insensitive).
func (s *Store) TableExists(name string) bool {
	var dummy int
	err := s.db.QueryRow(`SELECT 1 FROM tables WHERE UPPER(name) = ? LIMIT 1`, strings.ToUpper(name)).Scan(&dummy)
	return err == nil
}

// GetColumnInfo retrieves a single column's metadata from a specific table.
// Returns nil, nil if not found.
func (s *Store) GetColumnInfo(tableName, colName string) (*model.Column, error) {
	var c model.Column
	var nullable int
	err := s.db.QueryRow(`
		SELECT c.name, c.data_type, c.length, c.precision, c.nullable, c.description, c.position
		FROM columns c JOIN tables t ON c.table_id = t.id
		WHERE UPPER(t.name) = ? AND UPPER(c.name) = ?
		LIMIT 1`,
		strings.ToUpper(tableName), strings.ToUpper(colName)).Scan(
		&c.Name, &c.DataType, &c.Length, &c.Precision, &nullable, &c.Description, &c.Position)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Nullable = nullable == 1
	return &c, nil
}

// GetFKTarget finds the target (referenced) table for a given table's FK column.
// The foreign_keys table stores: table_id = source table, referencing_table = target table,
// fk_column = the column name on the source that holds the FK.
// Returns empty string if no FK metadata found.
func (s *Store) GetFKTarget(sourceTable, fkColumn string) (string, error) {
	var target string
	err := s.db.QueryRow(`
		SELECT fk.referencing_table
		FROM foreign_keys fk JOIN tables t ON fk.table_id = t.id
		WHERE UPPER(t.name) = ? AND UPPER(fk.fk_column) = ?
		LIMIT 1`,
		strings.ToUpper(sourceTable), strings.ToUpper(fkColumn)).Scan(&target)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.ToUpper(target), nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
