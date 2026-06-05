// Package lookup resolves Oracle Fusion table.column references to their
// canonical lookup table chains (_B / _TL / _VL) and generates join SQL.
package lookup

import (
	"fmt"
	"strings"

	"github.com/birendrakm0508-sudo/oracle-fusion-schema/internal/db"
	"github.com/birendrakm0508-sudo/oracle-fusion-schema/internal/mapping"
	"github.com/birendrakm0508-sudo/oracle-fusion-schema/internal/model"
)

// SourceInfo describes the input column being resolved.
type SourceInfo struct {
	Table    string `json:"table"`
	Column   string `json:"column"`
	DataType string `json:"data_type"`
}

// TargetInfo describes the resolved lookup target.
type TargetInfo struct {
	BaseTable         string  `json:"base_table"`
	TLTable           *string `json:"tl_table"`
	VLView            *string `json:"vl_view"`
	JoinColumn        *string `json:"join_column"` // nil -> null when unresolvable
	NameColumn        string  `json:"name_column"`
	DescriptionColumn string  `json:"description_column,omitempty"`
	DataSource        string  `json:"data_source"`
	LookupType        string  `json:"lookup_type,omitempty"`
}

// ResolutionInfo describes how the target was determined.
type ResolutionInfo struct {
	Method     string `json:"method"`     // fk_metadata, heuristic, static_map
	Confidence string `json:"confidence"` // high, medium, low
	TLExists   bool   `json:"tl_exists"`
	VLExists   bool   `json:"vl_exists"`
	Note       string `json:"note,omitempty"`
	Warning    string `json:"warning,omitempty"` // set when join_column could not be resolved
}

// LookupResult is the complete output of a lookup-target resolution.
//
// Sample SQL fields:
//   - sample_join_sql_vl: paste when joining to the _VL view (most common; the
//     view pre-filters USERENV('LANG'), so no LANGUAGE predicate is needed).
//   - sample_join_sql_tl: paste when joining directly to the _TL translation
//     table (includes the LANGUAGE predicate).
//   - sample_join_sql: DEPRECATED alias for sample_join_sql_tl (kept one release
//     for backward compatibility; migrate to the explicit _tl / _vl fields).
type LookupResult struct {
	Source          SourceInfo     `json:"source"`
	Target          *TargetInfo    `json:"target,omitempty"`
	Resolution      ResolutionInfo `json:"resolution"`
	SampleJoinSQLTL string         `json:"sample_join_sql_tl,omitempty"`
	SampleJoinSQLVL string         `json:"sample_join_sql_vl,omitempty"`
	SampleJoinSQL   string         `json:"sample_join_sql,omitempty"` // deprecated alias for _tl
}

// ErrorResult is returned when no target is found and --strict is set.
type ErrorResult struct {
	Error   string     `json:"error"`
	Source  SourceInfo `json:"source"`
	Message string     `json:"message"`
}

// staticFKTargets maps well-known cross-module FK column names to their
// canonical target tables. These are the top-20 most common joins.
var staticFKTargets = map[string]string{
	"VENDOR_ID":           "POZ_SUPPLIERS_V",
	"VENDOR_SITE_ID":      "POZ_SUPPLIER_SITES_ALL_M",
	"PARTY_ID":            "HZ_PARTIES",
	"CUST_ACCOUNT_ID":     "HZ_CUST_ACCOUNTS",
	"PERSON_ID":           "PER_ALL_PEOPLE_F",
	"ASSIGNMENT_ID":       "PER_ALL_ASSIGNMENTS_M",
	"ORGANIZATION_ID":     "HR_ALL_ORGANIZATION_UNITS_F",
	"BUSINESS_UNIT_ID":    "FUN_ALL_BUSINESS_UNITS_V",
	"INVENTORY_ITEM_ID":   "EGP_SYSTEM_ITEMS_B",
	"LEDGER_ID":           "GL_LEDGERS",
	"LEGAL_ENTITY_ID":     "XLE_ENTITY_PROFILES",
	"SET_OF_BOOKS_ID":     "GL_LEDGERS",
	"CURRENCY_CODE":       "FND_CURRENCIES",
	"CODE_COMBINATION_ID": "GL_CODE_COMBINATIONS",
	"CATEGORY_ID":         "EGP_CATEGORIES_B",
	"PROJECT_ID":          "PJF_PROJECTS_ALL_B",
	"TASK_ID":             "PJF_TASKS_B",
	"PAYROLL_ID":          "PAY_ALL_PAYROLLS_F",
	"JOB_ID":              "PER_JOBS_F",
	"GRADE_ID":            "PER_GRADES_F",
}

// Resolve takes a TABLE.COLUMN reference and resolves it to the canonical
// lookup table chain. Resolution cascade: FK metadata -> naming heuristics -> static map.
func Resolve(store *db.Store, tableName, columnName string) (*LookupResult, error) {
	tableName = strings.ToUpper(tableName)
	columnName = strings.ToUpper(columnName)

	// Step 1: Validate input
	if !store.TableExists(tableName) {
		return nil, fmt.Errorf("table %q not found in schema database", tableName)
	}

	colInfo, err := store.GetColumnInfo(tableName, columnName)
	if err != nil {
		return nil, fmt.Errorf("column lookup error: %w", err)
	}
	if colInfo == nil {
		return nil, fmt.Errorf("column %q not found in table %q", columnName, tableName)
	}

	source := SourceInfo{
		Table:    tableName,
		Column:   columnName,
		DataType: colInfo.DataType,
	}

	// Step 2: Try FK metadata
	fkTarget, err := store.GetFKTarget(tableName, columnName)
	if err != nil {
		return nil, fmt.Errorf("FK lookup error: %w", err)
	}
	if fkTarget != "" {
		result := buildResult(store, source, strings.ToUpper(fkTarget), "fk_metadata", "high", "")
		return result, nil
	}

	// Step 3: Try naming heuristics
	if target, note := heuristicResolve(store, columnName); target != "" {
		result := buildResult(store, source, target, "heuristic", "medium", note)
		return result, nil
	}

	// Step 4: Try static FK map
	if target, ok := staticFKTargets[columnName]; ok {
		if store.TableExists(target) {
			result := buildResult(store, source, target, "static_map", "medium", "")
			return result, nil
		}
	}

	// No match found
	return &LookupResult{
		Source: source,
		Resolution: ResolutionInfo{
			Method:     "none",
			Confidence: "low",
			Note:       "No FK metadata, naming-convention match, or static mapping found. Inspect 'describe' output manually.",
		},
	}, nil
}

// wellKnownTables are Oracle Fusion core tables that exist in every environment
// but may not appear in the domain-specific Tables and Views documentation.
// They are treated as always-available for heuristic resolution.
var wellKnownTables = map[string]bool{
	"FND_LOOKUP_VALUES":    true,
	"FND_LOOKUP_VALUES_VL": true,
	"FND_LOOKUP_VALUES_B":  true,
	"FND_LOOKUP_VALUES_TL": true,
	"FND_LOOKUPS":          true,
	"FND_CURRENCIES":       true,
	"FND_CURRENCIES_VL":    true,
	"FND_TERRITORIES_VL":   true,
}

// tableKnown checks if a table exists in the schema DB or is a well-known core table.
func tableKnown(store *db.Store, name string) bool {
	upper := strings.ToUpper(name)
	if wellKnownTables[upper] {
		return true
	}
	return store.TableExists(upper)
}

// heuristicResolve tries naming-convention-based resolution for the column name.
// Returns the target table name and an optional note, or empty string if no match.
func heuristicResolve(store *db.Store, colName string) (string, string) {
	upper := strings.ToUpper(colName)

	// *_TYPE_ID -> *_TYPES_B / _TL / _VL
	if strings.HasSuffix(upper, "_TYPE_ID") {
		stem := strings.TrimSuffix(upper, "_ID")
		candidates := []string{stem + "S_B", stem + "S_TL", stem + "S_VL"}
		for _, c := range candidates {
			if tableKnown(store, c) {
				base := stem + "S_B"
				if !tableKnown(store, base) {
					base = c
				}
				return base, fmt.Sprintf("Resolved via naming convention: %s -> %sS_*", colName, stem)
			}
		}
	}

	// *_TYPE_CODE or *_TYPE (without _ID) -> try specific _TYPES_* tables, then FND_LOOKUPS
	if strings.HasSuffix(upper, "_TYPE_CODE") || strings.HasSuffix(upper, "_TYPE") {
		var stem string
		if strings.HasSuffix(upper, "_TYPE_CODE") {
			stem = strings.TrimSuffix(upper, "_CODE")
		} else {
			stem = upper
		}
		// Try *_TYPES_B first
		candidates := []string{stem + "S_B", stem + "S_VL"}
		for _, c := range candidates {
			if tableKnown(store, c) {
				return c, fmt.Sprintf("Resolved via naming convention: %s -> %s", colName, c)
			}
		}
		// Fall back to FND_LOOKUPS
		lookupType := upper
		if strings.HasSuffix(lookupType, "_CODE") {
			lookupType = strings.TrimSuffix(lookupType, "_CODE")
		}
		return "FND_LOOKUP_VALUES",
			fmt.Sprintf("Resolved via FND_LOOKUP_VALUES_VL by lookup_type='%s'. Confirm by inspecting actual values.", lookupType)
	}

	// *_STATUS_CODE -> *_STATUSES_B / _VL
	if strings.HasSuffix(upper, "_STATUS_CODE") {
		stem := strings.TrimSuffix(upper, "_CODE")
		candidates := []string{stem + "ES_B", stem + "ES_VL", stem + "ES_TL"}
		for _, c := range candidates {
			if tableKnown(store, c) {
				return c, fmt.Sprintf("Resolved via naming convention: %s -> %s", colName, c)
			}
		}
		// Fall back to FND_LOOKUPS
		return "FND_LOOKUP_VALUES",
			fmt.Sprintf("Resolved via FND_LOOKUP_VALUES_VL by lookup_type='%s'. Confirm by inspecting actual values.", upper)
	}

	// *_FLAG, *_STATUS, *_CODE (short codes) -> FND_LOOKUPS
	if strings.HasSuffix(upper, "_FLAG") || strings.HasSuffix(upper, "_STATUS") ||
		strings.HasSuffix(upper, "_CODE") {
		return "FND_LOOKUP_VALUES",
			fmt.Sprintf("Resolved via FND_LOOKUP_VALUES_VL by lookup_type='%s'. Confirm by inspecting actual values.", upper)
	}

	return "", ""
}

// buildResult constructs the full LookupResult for a resolved target.
func buildResult(store *db.Store, source SourceInfo, targetBase, method, confidence, note string) *LookupResult {
	targetBase = strings.ToUpper(targetBase)

	// Determine the logical root by stripping known suffixes
	root := targetBase
	for _, suffix := range []string{"_B", "_TL", "_VL", "_V"} {
		if strings.HasSuffix(root, suffix) {
			root = strings.TrimSuffix(root, suffix)
			break
		}
	}

	// Check companion tables
	tlTable := root + "_TL"
	vlView := root + "_VL"
	bTable := root + "_B"

	tlExists := tableKnown(store, tlTable)
	vlExists := tableKnown(store, vlView)
	bExists := tableKnown(store, bTable)

	// If base table doesn't exist as _B, use what we have
	if bExists {
		targetBase = bTable
	} else if !tableKnown(store, targetBase) {
		// The resolved name might itself be the table (e.g., HZ_PARTIES, GL_LEDGERS)
		if tableKnown(store, root) {
			targetBase = root
		}
	}

	// Find join column, name column, description column
	joinCol := ""
	nameCol := ""
	descCol := ""

	// Resolve the join column with a robust fallback cascade:
	//   1. Target's primary key (authoritative; single or composite).
	//   2. Same-name column on the target matching the source column. This is a
	//      strong FK signal for natural/code keys (e.g. PAYMENT_METHOD_CODE) when
	//      the docs did not record a primary key for the base table.
	//   3. First _ID column on the target (handles surrogate-key joins).
	baseTable, _ := store.GetTable(targetBase)
	if baseTable != nil && baseTable.PrimaryKey != nil && len(baseTable.PrimaryKey.Columns) > 0 {
		joinCol = strings.Join(baseTable.PrimaryKey.Columns, ", ")
	}
	if joinCol == "" && baseTable != nil {
		for _, c := range baseTable.Columns {
			if c.Name == source.Column {
				joinCol = c.Name
				break
			}
		}
	}
	if joinCol == "" && baseTable != nil {
		for _, c := range baseTable.Columns {
			if strings.HasSuffix(c.Name, "_ID") {
				joinCol = c.Name
				break
			}
		}
	}

	// Look for NAME and DESCRIPTION columns in _TL first, then base
	searchTables := []string{}
	if tlExists {
		searchTables = append(searchTables, tlTable)
	}
	searchTables = append(searchTables, targetBase)
	if vlExists {
		searchTables = append(searchTables, vlView)
	}

	for _, tName := range searchTables {
		t, _ := store.GetTable(tName)
		if t == nil {
			continue
		}
		for _, c := range t.Columns {
			if nameCol == "" && (strings.HasSuffix(c.Name, "_NAME") || c.Name == "NAME" || c.Name == "MEANING") {
				nameCol = c.Name
			}
			if descCol == "" && (c.Name == "DESCRIPTION" || strings.HasSuffix(c.Name, "_DESCRIPTION")) {
				descCol = c.Name
			}
		}
		if nameCol != "" {
			break // Found name column, stop searching
		}
	}

	// Special handling for FND_LOOKUP_VALUES
	isFndLookup := strings.HasPrefix(targetBase, "FND_LOOKUP")
	if isFndLookup {
		joinCol = "LOOKUP_CODE"
		nameCol = "MEANING"
		descCol = "DESCRIPTION"
	}

	// Determine data source
	ds := mapping.IdentifyDataSource(targetBase)

	// Build target info. join_column is a pointer so it can serialize to null
	// when no candidate could be resolved (rather than an empty string).
	target := &TargetInfo{
		BaseTable:         targetBase,
		NameColumn:        nameCol,
		DescriptionColumn: descCol,
		DataSource:        ds.DataSource,
	}
	if joinCol != "" {
		target.JoinColumn = &joinCol
	}
	if tlExists {
		target.TLTable = &tlTable
	}
	if vlExists {
		target.VLView = &vlView
	}

	// Add lookup_type for FND_LOOKUPS resolution
	if isFndLookup && note != "" {
		// Extract lookup_type from the note
		if idx := strings.Index(note, "lookup_type='"); idx >= 0 {
			start := idx + len("lookup_type='")
			end := strings.Index(note[start:], "'")
			if end >= 0 {
				target.LookupType = note[start : start+end]
			}
		}
	}

	resolution := ResolutionInfo{
		Method:     method,
		Confidence: confidence,
		TLExists:   tlExists,
		VLExists:   vlExists,
		Note:       note,
	}
	if target.JoinColumn == nil {
		resolution.Warning = fmt.Sprintf(
			"Could not resolve join column for %s: no primary key, same-name, or _ID column found. Inspect 'describe %s'.",
			targetBase, targetBase)
	}

	// Generate sample join SQL variants.
	tlSQL, vlSQL, singleSQL := generateJoinSQLVariants(source, target, tlExists, vlExists, isFndLookup)

	result := &LookupResult{
		Source:     source,
		Target:     target,
		Resolution: resolution,
	}
	switch {
	case tlExists && vlExists:
		result.SampleJoinSQLTL = tlSQL
		result.SampleJoinSQLVL = vlSQL
		result.SampleJoinSQL = tlSQL // deprecated alias = _tl
	case tlExists:
		result.SampleJoinSQLTL = tlSQL
		result.SampleJoinSQL = tlSQL
	case vlExists:
		result.SampleJoinSQLVL = vlSQL
		result.SampleJoinSQL = vlSQL
	default:
		// Neither companion exists: a single fragment with no LANGUAGE line.
		result.SampleJoinSQL = singleSQL
	}

	return result
}

// generateJoinSQLVariants builds the _TL-style, _VL-style, and single
// WHERE-clause fragments for the resolved join. The _TL variant includes the
// LANGUAGE predicate; the _VL and single variants do not (the _VL view already
// filters USERENV('LANG') internally).
func generateJoinSQLVariants(source SourceInfo, target *TargetInfo, tlExists, vlExists, isFndLookup bool) (tl, vl, single string) {
	srcCol := strings.ToLower(source.Column)

	if isFndLookup {
		lookupType := target.LookupType
		if lookupType == "" {
			lookupType = strings.ToUpper(source.Column)
		}
		base := fmt.Sprintf("AND src.%s = lk.lookup_code\nAND lk.lookup_type = '%s'", srcCol, lookupType)
		tl = base + "\nAND lk.language    = USERENV('LANG')"
		vl = base
		single = base
		return tl, vl, single
	}

	// Use the first column if the join is composite; the full list is still in
	// target.join_column for the consumer.
	joinCol := "<join_column>"
	if target.JoinColumn != nil {
		jc := *target.JoinColumn
		if idx := strings.Index(jc, ","); idx >= 0 {
			jc = strings.TrimSpace(jc[:idx])
		}
		joinCol = strings.ToLower(jc)
	}

	base := fmt.Sprintf("AND src.%s = tgt.%s", srcCol, joinCol)
	tl = base + "\nAND tgt.language = USERENV('LANG')"
	vl = base
	single = base
	return tl, vl, single
}

// SynthesizeViewColumns returns the column list for a view. Oracle's _VL / _V
// view documentation pages do not enumerate columns, so the CLI synthesizes
// them from the underlying _B base table and _TL translation table following
// the standard Fusion convention:
//
//	SELECT _B.*, _TL.<translated columns>
//	FROM _B JOIN _TL ON _B.<pk> = _TL.<pk>
//	WHERE _TL.LANGUAGE = USERENV('LANG')
//
// The LANGUAGE and SOURCE_LANG columns are filtered out by the view definition
// and therefore excluded from the synthesized list.
//
// Returns the columns and a column_source marker:
//   - "docs": the view already carried columns from the documentation
//   - "synthesized_from_b_tl": columns composed from _B (plus optional _TL)
//   - "unknown": no base table found; columns could not be synthesized
func SynthesizeViewColumns(store *db.Store, view *model.Table) ([]model.Column, string) {
	// If the docs already provided columns, trust them.
	if len(view.Columns) > 0 {
		return view.Columns, "docs"
	}

	name := strings.ToUpper(view.Name)

	// Derive the logical root by stripping the view suffix.
	root := name
	for _, suffix := range []string{"_VL", "_V"} {
		if strings.HasSuffix(root, suffix) {
			root = strings.TrimSuffix(root, suffix)
			break
		}
	}

	// Locate the base table: prefer <root>_B, fall back to <root> itself.
	var base *model.Table
	for _, cand := range []string{root + "_B", root} {
		if t, _ := store.GetTable(cand); t != nil && len(t.Columns) > 0 {
			base = t
			break
		}
	}
	if base == nil {
		return nil, "unknown"
	}

	// Start with all base columns.
	cols := make([]model.Column, 0, len(base.Columns))
	seen := make(map[string]bool, len(base.Columns))
	for _, c := range base.Columns {
		cols = append(cols, c)
		seen[c.Name] = true
	}

	// Append _TL columns not already present, excluding the language-filter
	// columns the view does not project.
	if tl, _ := store.GetTable(root + "_TL"); tl != nil {
		for _, c := range tl.Columns {
			if c.Name == "LANGUAGE" || c.Name == "SOURCE_LANG" {
				continue
			}
			if seen[c.Name] {
				continue
			}
			cols = append(cols, c)
			seen[c.Name] = true
		}
	}

	// Renumber positions sequentially for clean display.
	for i := range cols {
		cols[i].Position = i + 1
	}

	return cols, "synthesized_from_b_tl"
}
