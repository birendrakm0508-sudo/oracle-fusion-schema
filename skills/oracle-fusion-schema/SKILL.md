---
name: oracle-fusion-schema
description: >-
  Use this skill whenever you need Oracle Fusion Cloud table/view schema
  information: column names, data types, primary keys, indexes, foreign keys,
  table descriptions, BIP data source identification, EBS-to-Fusion table
  mapping, or join-path discovery. Triggers include: looking up a Fusion table
  before writing SQL, verifying column names exist, finding which JDBC data
  source a table belongs to, discovering join columns between tables, mapping
  EBS table names to Fusion equivalents, or searching for tables by business
  concept. This skill replaces the web_search + web_fetch fallback in the BIP
  report builder workflow with instant offline lookups across 21,718 indexed
  tables/views. Use this skill BEFORE the deloitte-bip-fusion-report-builder
  skill's oracle_docs.md recipe — it is faster, offline, and covers every
  table in every domain.
---

# Oracle Fusion Schema CLI

Instant offline lookup for Oracle Fusion Cloud Tables and Views documentation.
Covers **21,718 tables/views** across all 7 SaaS domains. Designed as the
schema-lookup companion for AI agents building BI Publisher (BIP) reports.

**Binary:** `oracle-fusion-schema`
**Database:** `~/.oracle-fusion-schema/schema.db`
**All commands accept `--json` for machine-readable output.**

## Preflight — Run Before First Use

Before running any command in this skill, check that the CLI is available
and the database is populated. Run this check ONCE per session, not per command.

```bash
# Step 1: Check if binary exists
oracle-fusion-schema version --json
```

**If the command succeeds** — the CLI is installed. Proceed to step 2.

**If the command fails** (`command not found` / not recognized):
- The CLI is not installed. Tell the user:
  ```
  oracle-fusion-schema is not installed. Install it with:
    go install github.com/birendrakm0508-sudo/oracle-fusion-schema@latest
  Then run: oracle-fusion-schema sync
  ```
- Do NOT attempt to install it yourself. Do NOT fall back to web_search.
  Stop and wait for the user to install.

```bash
# Step 2: Check if database is populated
oracle-fusion-schema domains --json
```

**If table counts are all > 0** — the database is ready. Skip to commands.

**If any domain shows 0 tables or the command errors** — the database needs
syncing. Tell the user:
  ```
  The schema database needs to be populated. Run:
    oracle-fusion-schema sync
  At default parallelism (--workers 20, --throttle-ms 50) this takes
  ~15-25 min on a typical corporate network. For ~5-8 min on a fast
  network, use: oracle-fusion-schema sync --workers 40 --throttle-ms 0
  Do NOT run two sync commands at the same time — the CLI has no
  process-level lock and they will collide on SQLITE_BUSY.
  ```

**Once preflight passes, do not repeat it.** Cache the result mentally for
the rest of the session.

## When To Use This Skill

| Situation | What to run |
|-----------|-------------|
| Need column names/types before writing SQL | `describe <TABLE> --json` |
| Need the JDBC data source for a BIP data model | `datasource <TABLE> --json` |
| SQL has a coded column (_TYPE_ID, _CODE, _FLAG) | `lookup-target <TABLE>.<COLUMN> --json` |
| Don't know which table holds the data | `search <keyword> --json` |
| Need to find join columns between tables | `columns <COL_NAME> --json` |
| Migrating an EBS report to Fusion | `mapping lookup <EBS_TABLE> --json` |
| Browsing all tables in a module | `tables <domain> --json` |

**Always pass `--json`.** Parse the structured output — don't regex human tables.

## Domains

| Code  | Alias          | Tables | Data Source         | Common Prefixes               |
|-------|----------------|--------|---------------------|-------------------------------|
| OEDMH | hcm            | 5,632  | ApplicationDB_HCM   | PAY_, PER_, HR_, BEN_, HWM_   |
| OEDSC | scm            | 5,352  | ApplicationDB_FSCM  | INV_, EGP_, MNT_, WIE_        |
| OEDMS | sales, cx      | 4,439  | ApplicationDB_CRM   | ZMM_, ZCA_, ZSO_, MOO_        |
| OEDMF | financials     | 4,014  | ApplicationDB_FSCM  | GL_, AP_, AR_, FA_, XLA_       |
| OEDPP | ppm, projects  | 1,334  | ApplicationDB_FSCM  | PJF_, PJC_, PJB_, PJS_, GMS_   |
| OEDMP | procurement    | 652    | ApplicationDB_FSCM  | PO_, PON_, POZ_, ICX_          |
| OEDMA | common         | 295    | ApplicationDB_FSCM  | FND_, PER_                     |

Use aliases in commands: `tables hcm`, `search payroll --domain hcm`, `tables ppm`.

---

## Command Reference

### 1. `describe <table-name> [--json] [--columns-only]`

Get the full schema for a table: columns, data types, lengths, nullability,
descriptions, primary keys, indexes, foreign keys.

**This is the primary command when writing SQL for a BIP data model.** Use it
to verify every column name before referencing it in SQL — wrong column names
produce a data model that imports cleanly but fails at execute time.

```bash
oracle-fusion-schema describe GL_JE_HEADERS --json
```

**JSON output:**
```json
{
  "name": "GL_JE_HEADERS",
  "domain": "OEDMF",
  "type": "Tables",
  "schema": "FUSION",
  "description": "GL_JE_HEADERS contains journal entries...",
  "doc_url": "https://docs.oracle.com/en/cloud/saas/.../gljeheaders-6523.html",
  "columns": [
    {
      "name": "JE_HEADER_ID",
      "data_type": "NUMBER",
      "length": "",
      "precision": "18",
      "nullable": false,
      "description": "Journal entry header identifier.",
      "position": 1
    }
  ],
  "primary_keys": [...],
  "indexes": [...],
  "foreign_keys": [...]
}
```

Key column fields: `name`, `data_type` (NUMBER, VARCHAR2, DATE, TIMESTAMP,
CLOB), `length` (VARCHAR2), `precision` (NUMBER), `nullable`, `description`.

**Views (`_VL`/`_V`) return their published columns, type-enriched.** Oracle's
view pages list the projected column names (names only); the CLI captures that
authoritative list (including view-specific derived columns) and enriches each
column's datatype/description from the underlying `_B`/`_TL` tables. A
`column_source` field marks the origin: `docs` (scraped from the view page, the
normal case), `synthesized_from_b_tl` (fallback), or `unknown`. Use this to
verify a column exists on the `_VL` view before referencing it — and note the
published list never contains `LANGUAGE`/`SOURCE_LANG`.

**EBS auto-mapping:** Passing an EBS name automatically resolves it:
```bash
oracle-fusion-schema describe MTL_SYSTEM_ITEMS_B --json
# Resolves to EGP_SYSTEM_ITEMS_B and returns its full schema
```

**Flags:**
- `--columns-only` — Skip PKs, indexes, FKs. Use when you only need the
  SELECT column list.

---

### 2. `datasource <table-name> [--json]`

Determine which BIP JDBC data source to use for a table. Every BIP data model
needs a `data_source_ref` value — this command tells you which one.

**Run this before writing any SQL.** Getting the data source wrong means the
report fails at runtime.

```bash
oracle-fusion-schema datasource PER_ALL_PEOPLE_F --json
```

```json
{
  "data_source": "ApplicationDB_HCM",
  "table_name": "PER_ALL_PEOPLE_F",
  "table_prefix": "PER_",
  "notes": "HCM, Payroll, Benefits, Talent, Workforce Management"
}
```

**Data source rules:**

| Data Source          | Prefixes                                                              |
|----------------------|-----------------------------------------------------------------------|
| ApplicationDB_HCM   | PAY_, PER_, HR_, BEN_, HWM_, HRC_, HRI_, ANC_, CMP_, ORA_HCM_        |
| ApplicationDB_FSCM  | GL_, AP_, AR_, PO_, POZ_, INV_, EGP_, FA_, XLA_, CST_, PJF_, PJC_, PJB_, PJS_, PJT_, PJO_, GMS_ (Projects/Grants) |
| ApplicationDB_CRM   | ZMM_, ZCA_, ZSO_, MOO_, MKL_, MKT_, HBY_, CN_                        |

**Cross-data-source reports:** If your report joins tables from different data
sources (e.g., PER_ + GL_), you need **two separate data sets** in the BIP data
model, one per data source, linked at the data model level using a shared key.

---

### 3. `search <query> [--json] [--domain <d>] [--column] [--description] [--limit N]`

Full-text search across table names, column names, and descriptions. Use when
you have a business concept but don't know which tables hold the data.

```bash
oracle-fusion-schema search "employee absence" --domain hcm --json --limit 10
```

```json
[
  {
    "table_name": "ANC_ABSENCE_ENTRIES",
    "domain": "OEDMH",
    "match_type": "table_name",
    "match_field": "ANC_ABSENCE_ENTRIES",
    "description": "This table stores absence entries..."
  }
]
```

**`match_type` priority:** `table_name` > `column_name` > `description`.
Table-name matches are the strongest signal.

**Flags:**
- `--column` — Search column names only.
- `--description` — Search descriptions only.
- `--domain hcm` — Restrict to one domain.
- `--limit 20` — Cap results (default 50).

---

### 4. `columns <pattern> [--json] [--domain <d>] [--exact] [--limit N]`

Find every table that contains a specific column. **This is the fastest way to
discover join paths between tables.**

```bash
oracle-fusion-schema columns PERSON_ID --domain hcm --exact --json --limit 10
```

```json
[
  {
    "ColumnName": "PERSON_ID",
    "TableName": "PER_ALL_PEOPLE_F",
    "Domain": "OEDMH",
    "DataType": "NUMBER",
    "Description": "System generated surrogate key..."
  }
]
```

**How join discovery works:** You have table A and table B and need to join
them. Run `columns` on the suspected FK column — every table that shares
that column is a potential join target.

**Common FK columns by module:**

| Module      | Key Columns                                       |
|-------------|---------------------------------------------------|
| HCM         | `PERSON_ID`, `ASSIGNMENT_ID`, `PAYROLL_ID`        |
| GL          | `LEDGER_ID`, `JE_HEADER_ID`, `CODE_COMBINATION_ID`|
| AP          | `INVOICE_ID`, `CHECK_ID`, `VENDOR_ID`             |
| AR          | `CUSTOMER_TRX_ID`, `CASH_RECEIPT_ID`              |
| Procurement | `PO_HEADER_ID`, `PO_LINE_ID`, `REQ_HEADER_ID`    |
| SCM         | `INVENTORY_ITEM_ID`, `ORGANIZATION_ID`            |
| CRM/Sales   | `PARTY_ID`, `CUST_ACCOUNT_ID`                     |
| Cross-module| `BUSINESS_UNIT_ID`, `LEGAL_ENTITY_ID`             |

**Flags:**
- `--exact` — Exact column name match (default is substring).
- `--domain scm` — Restrict to one domain.
- `--limit 100` — Show more results (default 50).

---

### 5. `mapping lookup <ebs-table> [--json]`

Map an Oracle EBS table name to its Fusion equivalent. Use when migrating
existing EBS reports or when requirements reference EBS table names.

```bash
oracle-fusion-schema mapping lookup MTL_SYSTEM_ITEMS_B --json
```

```json
{
  "ebs_name": "MTL_SYSTEM_ITEMS_B",
  "fusion_name": "EGP_SYSTEM_ITEMS_B",
  "module": "INV",
  "notes": "item_number replaces segment1"
}
```

**Key mappings:**

| EBS Table                    | Fusion Table                 | Notes                         |
|------------------------------|------------------------------|-------------------------------|
| MTL_SYSTEM_ITEMS_B           | EGP_SYSTEM_ITEMS_B           | item_number replaces segment1 |
| MTL_ONHAND_QUANTITIES_DETAIL | INV_ONHAND_QUANTITIES_DETAIL |                               |
| PO_VENDORS                   | POZ_SUPPLIERS_V              |                               |
| PO_VENDOR_SITES_ALL          | POZ_SUPPLIER_SITES_V         |                               |
| PA_PROJECTS_ALL              | PJF_PROJECTS_ALL_B           |                               |
| HR_ALL_ORGANIZATION_UNITS    | HR_ALL_ORGANIZATION_UNITS_F  | date-effective in Fusion      |
| FND_LOOKUP_VALUES            | FND_LOOKUP_VALUES_VL         | _VL for translated view       |

If lookup returns nothing, the table name may be identical in EBS and Fusion
(e.g., `AP_INVOICES_ALL`, `GL_JE_HEADERS`). Try `describe` directly.

**Subcommands:**
- `mapping list [--json]` — Show all 30 built-in + custom mappings.
- `mapping add <ebs> <fusion>` — Register a custom mapping.

---

### 6. `lookup-target <TABLE.COLUMN> [--json] [--strict]`

Resolve a coded column to its canonical lookup table chain (_B/_TL/_VL) in one
call. Returns the target table, join column, name column, data source, and
sample JOIN SQL. **Use this instead of the 5-call search+describe chain when
your SQL has a `*_TYPE_ID`, `*_CODE`, `*_STATUS`, or `*_FLAG` column.**

```bash
oracle-fusion-schema lookup-target INV_RESERVATIONS.SUPPLY_SOURCE_TYPE_ID --json
```

```json
{
  "source": { "table": "INV_RESERVATIONS", "column": "SUPPLY_SOURCE_TYPE_ID", "data_type": "NUMBER" },
  "target": {
    "base_table": "INV_TXN_SOURCE_TYPES_B",
    "tl_table": "INV_TXN_SOURCE_TYPES_TL",
    "vl_view": "INV_TXN_SOURCE_TYPES_VL",
    "join_column": "TRANSACTION_SOURCE_TYPE_ID",
    "name_column": "TRANSACTION_SOURCE_TYPE_NAME",
    "description_column": "DESCRIPTION",
    "data_source": "ApplicationDB_FSCM"
  },
  "resolution": { "method": "fk_metadata", "confidence": "high", "tl_exists": true, "vl_exists": true },
  "sample_join_sql_vl": "AND src.supply_source_type_id = tgt.transaction_source_type_id",
  "sample_join_sql_tl": "AND src.supply_source_type_id = tgt.transaction_source_type_id\nAND tgt.language = USERENV('LANG')",
  "sample_join_sql": "AND src.supply_source_type_id = tgt.transaction_source_type_id\nAND tgt.language = USERENV('LANG')"
}
```

For FND_LOOKUPS-based codes (ITEM_TYPE, PARTY_TYPE, etc.):
```bash
oracle-fusion-schema lookup-target EGP_SYSTEM_ITEMS_B.ITEM_TYPE --json
```
Returns `target.lookup_type = "ITEM_TYPE"` with FND_LOOKUP_VALUES join SQL.

**Resolution cascade:** `fk_metadata` (high) → naming heuristic (medium) → static map (medium).

**Two sample SQL variants — pick by join target:**
- **`sample_join_sql_vl`** — joining to the `_VL` view (the usual choice). **No** `LANGUAGE` line; the view filters `USERENV('LANG')` itself. Pasting a `LANGUAGE` predicate against a `_VL` view throws `ORA-00904: "LANGUAGE": invalid identifier`.
- **`sample_join_sql_tl`** — joining the raw `_TL` table (rare). Includes the `LANGUAGE` predicate.
- **`sample_join_sql`** — DEPRECATED alias for `_tl`. Migrate to the explicit fields.

**`join_column` is always populated** when a target resolves (same-name column → PK → first `_ID` → `LOOKUP_CODE`). It is `null` (with `resolution.warning`) only when genuinely unresolvable.

**Key fields:** `target.join_column` for WHERE, `target.name_column` for display,
`target.data_source` for BIP connection, `sample_join_sql_vl` for copy-paste.

**Flags:**
- `--strict` — Exit code 1 if no target found.

---

### 7. `tables [domain] [--json] [--type <t>] [--count]`

List all tables in a domain. Use for browsing or getting a count.

```bash
oracle-fusion-schema tables financials --type view --json
```

**Flags:**
- `--type table` or `--type view` — Filter by object type.
- `--count` — Return count only.

---

### 8. `domains [--json]`

List all indexed domains with table counts.

```bash
oracle-fusion-schema domains --json
```

---

### 9. `export [--format json|csv] [--domain <d>] [--output <file>]`

Bulk export schema data for feeding into other tools.

---

## BIP Report Development Workflows

### Workflow A: New report from a business requirement

```
1. search "employee absence" --json            --> find candidate tables
2. datasource ANC_ABSENCE_ENTRIES --json       --> ApplicationDB_HCM
3. describe ANC_ABSENCE_ENTRIES --json         --> get column schema
4. describe PER_ALL_PEOPLE_F --json            --> get people columns
5. columns PERSON_ID --domain hcm --exact --json --> confirm join column
6. Write SQL using verified column names, types, and the correct data_source_ref
```

### Workflow B: EBS report migration

```
1. mapping lookup MTL_SYSTEM_ITEMS_B --json    --> EGP_SYSTEM_ITEMS_B
2. describe EGP_SYSTEM_ITEMS_B --json          --> check column differences
3. datasource EGP_SYSTEM_ITEMS_B --json        --> ApplicationDB_FSCM
4. Rewrite SQL: swap table names, fix renamed columns (SEGMENT1 -> ITEM_NUMBER)
```

### Workflow C: Find the right table from a column name

```
1. columns INVOICE_NUM --json                  --> lists all tables with this column
2. describe AP_INVOICES_ALL --json             --> pick best match, get full schema
3. datasource AP_INVOICES_ALL --json           --> ApplicationDB_FSCM
```

### Workflow D: Cross-data-source report (HR + Finance)

```
1. datasource PER_ALL_PEOPLE_F --json          --> ApplicationDB_HCM
2. datasource GL_JE_HEADERS --json             --> ApplicationDB_FSCM
3. Different data sources = TWO data sets in the BIP data model
   Data Set 1 (ApplicationDB_HCM): SELECT ... FROM PER_ALL_PEOPLE_F
   Data Set 2 (ApplicationDB_FSCM): SELECT ... FROM GL_JE_HEADERS
4. Link data sets at data model level via shared key (e.g., PERSON_ID)
```

### Workflow E: Join-path discovery using `columns`

When you need to join two tables but don't know the FK column:

```
1. describe TABLE_A --json                     --> scan columns for _ID suffixes
2. Pick a candidate FK (e.g., JE_HEADER_ID)
3. columns JE_HEADER_ID --exact --domain financials --json
   --> returns every table sharing that column
4. Find TABLE_B in the results — that's your join path
5. describe TABLE_B --json                     --> verify the column exists and types match
```

This replaces reading ER diagrams. Three `columns` calls can map an entire
multi-table join chain.

### Workflow F: Resolve coded columns to display names

When your SQL has a `*_TYPE_ID`, `*_CODE`, or `*_FLAG` column and you need the
human-readable name:

```
1. lookup-target INV_RESERVATIONS.SUPPLY_SOURCE_TYPE_ID --json
   --> Returns in ONE call:
       target table:  INV_TXN_SOURCE_TYPES_B
       _TL table:     INV_TXN_SOURCE_TYPES_TL
       _VL view:      INV_TXN_SOURCE_TYPES_VL
       join column:   TRANSACTION_SOURCE_TYPE_ID
       name column:   TRANSACTION_SOURCE_TYPE_NAME
       data source:   ApplicationDB_FSCM
       sample_join_sql_vl:  AND src.supply_source_type_id = tgt.transaction_source_type_id
       sample_join_sql_tl:  ...same, plus AND tgt.language = USERENV('LANG')

2. If your data set joins to the _VL view (usual), paste sample_join_sql_vl.
   If you join the raw _TL, paste sample_join_sql_tl. Done.
   NEVER paste the _tl LANGUAGE line against a _VL view -> ORA-00904.
```

For FND_LOOKUPS-based codes:
```
1. lookup-target EGP_SYSTEM_ITEMS_B.ITEM_TYPE --json
   --> lookup_type: ITEM_TYPE
       sample_join_sql_vl: AND src.item_type = lk.lookup_code
                           AND lk.lookup_type = 'ITEM_TYPE'
       sample_join_sql_tl: ...same, plus AND lk.language = USERENV('LANG')
```

---

## Integration with BIP Report Builder

This skill replaces the `references/oracle_docs.md` web-search fallback in the
`deloitte-bip-fusion-report-builder` skill. Instead of:

```
web_search "<TABLE> Oracle Fusion 26b table"    --> find the docs page
web_fetch <url>                                  --> scrape the column list
```

Use:

```bash
oracle-fusion-schema describe <TABLE> --json     --> instant, offline, structured
```

**Advantages over the web fallback:**
- **Instant** — no network latency, no rate limiting
- **Offline** — works without internet after initial sync
- **Structured** — JSON output with typed fields, not scraped HTML
- **Complete** — 21,718 tables vs. one table at a time
- **Join discovery** — `columns` command finds FK paths across the entire schema
- **Data source** — `datasource` command returns the exact `data_source_ref` value
  the BIP builder needs

**Use oracle-fusion-schema FIRST.** Fall back to web_search only if the table
is genuinely not in the index (run `oracle-fusion-schema sync --force` to
refresh if you suspect stale data).

---

## SQL Conventions Reminder

When using column information from `describe` to write BIP SQL:

- **Traditional Oracle joins.** Comma-separated FROM, predicates in WHERE,
  `(+)` for outer joins. Never ANSI `JOIN ... ON`.
- **`_ALL` tables need a Business Unit filter.** Add `AND t.org_id IN (:P_BUSINESS_UNIT)`.
- **`_F` / `_M` tables are date-effective.** Add
  `AND :P_AS_OF BETWEEN t.effective_start_date AND t.effective_end_date`.
  For `_M` tables also add `AND t.effective_latest_change = 'Y'`.
- **Prefer `_VL` views** over `_B` / `_TL` base tables for user-visible labels.
- **Suppliers = `POZ_SUPPLIERS_V`**, never `PO_VENDORS`.
- **Customers = `HZ_PARTIES` + `HZ_CUST_ACCOUNTS`**, never `RA_CUSTOMERS`.

## Prefix-to-Module Quick Reference

| Prefix   | Module                | Prefix   | Module                  |
|----------|-----------------------|----------|-------------------------|
| PER_     | People                | GL_      | General Ledger          |
| PAY_     | Payroll               | AP_      | Payables                |
| BEN_     | Benefits              | AR_      | Receivables             |
| ANC_     | Absence Management    | FA_      | Fixed Assets            |
| CMP_     | Compensation          | XLA_     | Subledger Accounting    |
| HWM_     | Workforce Management  | PO_      | Purchasing              |
| HRC_     | Recruiting            | INV_     | Inventory               |
| PJF_     | Projects Foundation   | EGP_     | Product Hub / Items     |
| PJC_     | Project Costing       | CST_     | Cost Management         |
| PJB_     | Project Billing       | PJO_     | Project Control         |
| GMS_     | Grants Management     | PJS_/PJT_| Projects (security/tasks) |
| ZCA_     | Common CRM            | ZMM_     | Sales Activities        |
| ZSO_     | Sales Content         | MOO_     | Order Management        |
| MKT_     | Marketing             | FND_     | Foundation / Lookups    |
