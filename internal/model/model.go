// Package model defines the data types for Oracle Fusion schema documentation.
package model

// Domain represents an Oracle Fusion Cloud documentation domain.
type Domain struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	TOCURL  string `json:"toc_url"`
}

// Module represents a functional module within a domain (e.g., "Advanced Collections" under Financials).
type Module struct {
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

// Table represents an Oracle Fusion table or view.
type Table struct {
	ID     int64  `json:"id,omitempty"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
	Module string `json:"module"`
	Type   string `json:"type"` // TABLE or VIEW
	Owner  string `json:"owner,omitempty"`
	Schema string `json:"schema,omitempty"`
	// ColumnSource indicates how Columns was populated for views:
	// "docs", "synthesized_from_b_tl", or "unknown". Empty for base tables.
	ColumnSource string   `json:"column_source,omitempty"`
	Description  string   `json:"description"`
	DocURL       string   `json:"doc_url"`
	Columns      []Column `json:"columns,omitempty"`
	PrimaryKey   *PK      `json:"primary_key,omitempty"`
	Indexes      []Index  `json:"indexes,omitempty"`
	ForeignKeys  []FK     `json:"foreign_keys,omitempty"`
}

// Column represents a column in a table or view.
type Column struct {
	Name        string `json:"name"`
	DataType    string `json:"data_type"`
	Length      string `json:"length,omitempty"`
	Precision   string `json:"precision,omitempty"`
	Nullable    bool   `json:"nullable"`
	Description string `json:"description"`
	Position    int    `json:"position"`
}

// PK represents a primary key constraint.
type PK struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
}

// Index represents a database index.
type Index struct {
	Name       string   `json:"name"`
	Unique     bool     `json:"unique"`
	Tablespace string   `json:"tablespace,omitempty"`
	Columns    []string `json:"columns"`
}

// FK represents a foreign key relationship.
type FK struct {
	ReferencingTable string `json:"referencing_table"`
	FKColumn         string `json:"fk_column"`
	Relationship     string `json:"relationship"`
}

// EBSMapping maps an EBS table name to its Fusion equivalent.
type EBSMapping struct {
	EBSName    string `json:"ebs_name"`
	FusionName string `json:"fusion_name"`
	Module     string `json:"module"`
	Notes      string `json:"notes,omitempty"`
}

// DataSourceInfo identifies the BIP data source for a table.
type DataSourceInfo struct {
	DataSource  string `json:"data_source"`
	TableName   string `json:"table_name"`
	TablePrefix string `json:"table_prefix"`
	Domain      string `json:"domain,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// SearchResult holds a search match with context.
type SearchResult struct {
	TableName   string `json:"table_name"`
	Domain      string `json:"domain"`
	Module      string `json:"module"`
	MatchType   string `json:"match_type"` // table_name, column_name, description
	MatchField  string `json:"match_field"`
	MatchText   string `json:"match_text"`
	Description string `json:"description,omitempty"`
}

// SyncStats tracks sync progress.
type SyncStats struct {
	Domain       string `json:"domain"`
	TablesFound  int    `json:"tables_found"`
	TablesSynced int    `json:"tables_synced"`
	Errors       int    `json:"errors"`
}
