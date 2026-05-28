# oracle-fusion-schema -- Agent Tool Use Reference

Binary: `oracle-fusion-schema` (or `oracle-fusion-schema.exe` on Windows)
Database: `~/.oracle-fusion-schema/schema.db` (166 MB, 20,142 tables/views)
All commands accept `--json` for machine-readable output.

---

## Quick Decision Tree

```
Need to build a BIP report?
  1. datasource  <table>  --> which JDBC connection string to use
  2. describe    <table>  --> full column schema for your SQL
  3. columns     <col>    --> find join candidates across tables
  4. search      <term>   --> discover tables you don't know yet

Migrating from EBS?
  1. mapping lookup <ebs-table>  --> get the Fusion equivalent
  2. describe <fusion-table>     --> verify columns exist

Don't know the table name?
  1. search <keyword>            --> searches names + columns + descriptions
  2. tables <domain>             --> browse all tables in a domain
```

---

## Domains

Six indexed Oracle Fusion Cloud documentation domains:

| Code  | Alias         | Tables | Data Source        | Prefix Examples              |
|-------|---------------|--------|--------------------|------------------------------|
| OEDMH | hcm           | 5,632  | ApplicationDB_HCM  | PAY_, PER_, HR_, BEN_, HWM_  |
| OEDSC | scm           | 5,352  | ApplicationDB_FSCM | INV_, EGP_, MNT_, WIE_      |
| OEDMS | sales, cx     | 4,439  | ApplicationDB_CRM  | ZMM_, ZCA_, ZSO_, MOO_      |
| OEDMF | financials    | 4,014  | ApplicationDB_FSCM | GL_, AP_, AR_, FA_, XLA_     |
| OEDMP | procurement   | 652    | ApplicationDB_FSCM | PO_, PON_, POZ_, ICX_        |
| OEDMA | common        | 53     | ApplicationDB_FSCM | FND_, PER_                   |

Use domain aliases in commands: `tables hcm`, `search payroll --domain hcm`.

---

## Command Reference

### 1. `describe <table-name> [--json] [--columns-only] [--compact]`

**Purpose:** Get the full schema for a specific table -- columns, data types, lengths, nullability, descriptions, primary keys, indexes, and foreign keys. This is the primary command when writing SQL for a BIP data model.

**When to use:** You know the table name and need its column definitions.

```
oracle-fusion-schema describe GL_JE_HEADERS --json
```

**JSON shape:**
```json
{
  "id": 1234,
  "name": "GL_JE_HEADERS",
  "domain": "OEDMF",
  "module": "",
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

**Key fields per column:** `name`, `data_type` (NUMBER, VARCHAR2, DATE, TIMESTAMP, CLOB), `length` (for VARCHAR2), `precision` (for NUMBER), `nullable`, `description`.

**EBS auto-mapping:** Passing an EBS table name automatically resolves it:
```
oracle-fusion-schema describe MTL_SYSTEM_ITEMS_B
# Automatically maps to and describes EGP_SYSTEM_ITEMS_B
```

**Flags:**
- `--columns-only` -- Skip PKs, indexes, FKs. Useful when you only need the SELECT column list.
- `--compact` -- Condensed output for quick scanning.

---

### 2. `datasource <table-name> [--json]`

**Purpose:** Determine which BIP JDBC data source to configure for a table. Every BIP data model XML needs a `<dataSource>` element -- this command tells you which one.

**When to use:** Starting a new BIP report data model. Call this first.

```
oracle-fusion-schema datasource PER_ALL_PEOPLE_F --json
```

**JSON shape:**
```json
{
  "data_source": "ApplicationDB_HCM",
  "table_name": "PER_ALL_PEOPLE_F",
  "table_prefix": "PER_",
  "notes": "HCM, Payroll, Benefits, Talent, Workforce Management"
}
```

**Data source rules (prefix-based):**

| Data Source          | Prefixes                                                    |
|----------------------|-------------------------------------------------------------|
| ApplicationDB_HCM   | PAY_, PER_, HR_, BEN_, HWM_, HRC_, HRI_, ANC_, CMP_, ORA_HCM_ |
| ApplicationDB_FSCM  | GL_, AP_, AR_, PO_, PON_, POZ_, INV_, EGP_, PJC_, PJF_, FA_, XLA_, CST_, RCV_, ICX_, ASO_ |
| ApplicationDB_CRM   | ZMM_, ZCA_, ZSO_, MOO_, MKL_, MKT_, HBY_, CN_              |

If a report joins tables from different data sources, you need separate data sets in the BIP data model (one per data source), linked at the data model level.

---

### 3. `search <query> [--json] [--domain <d>] [--column] [--description] [--limit N]`

**Purpose:** Full-text search across table names, column names, and descriptions. Returns ranked results with match context.

**When to use:** You have a business concept (e.g., "payroll", "invoice", "absence") but don't know which tables hold that data.

```
oracle-fusion-schema search payroll --domain hcm --json --limit 5
```

**JSON shape (array):**
```json
[
  {
    "table_name": "PAY_PAYROLL_ACTIONS",
    "domain": "OEDMH",
    "module": "",
    "match_type": "table_name",
    "match_field": "PAY_PAYROLL_ACTIONS",
    "match_text": "PAY_PAYROLL_ACTIONS",
    "description": "This table contains general details of the execution of payroll processes..."
  },
  {
    "table_name": "ANC_ABSENCE_PLANS_F",
    "domain": "OEDMH",
    "module": "",
    "match_type": "column_name",
    "match_field": "ACC_PERIOD_PAYROLL_FREQ_ID",
    "match_text": "ACC_PERIOD_PAYROLL_FREQ_ID",
    "description": "ACC_PERIOD_PAYROLL_FREQ_ID"
  }
]
```

**`match_type` values:** `table_name`, `column_name`, `description`. Use `match_type` to prioritize results -- table name matches are strongest.

**Flags:**
- `--column` -- Only search column names.
- `--description` -- Only search table/column descriptions.
- `--domain hcm` -- Restrict to a single domain.
- `--limit 20` -- Cap results (default 50).

---

### 4. `columns <pattern> [--json] [--domain <d>] [--exact] [--limit N]`

**Purpose:** Find every table that contains a specific column. Critical for discovering join paths between tables.

**When to use:** You need to join two tables and want to find the shared key column, or you want to know which tables carry a specific attribute.

```
oracle-fusion-schema columns PERSON_ID --domain hcm --json --limit 10
```

**JSON shape (array):**
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

**Common join columns to search:**
- `PERSON_ID` -- Links HCM people tables
- `ORGANIZATION_ID` -- Links org hierarchy tables
- `BUSINESS_UNIT_ID` / `PRC_BU_ID` -- Links business unit scoped tables
- `LEDGER_ID` -- Links GL tables
- `INVENTORY_ITEM_ID` -- Links SCM/inventory tables
- `PO_HEADER_ID` -- Links procurement document tables
- `INVOICE_ID` -- Links AP invoice tables
- `JE_HEADER_ID` / `JE_BATCH_ID` -- Links GL journal tables

**Flags:**
- `--exact` -- Exact column name match (default is substring/pattern).
- `--domain scm` -- Restrict to one domain.
- `--limit 100` -- Show more results (default 50).

---

### 5. `tables [domain] [--json] [--module <m>] [--type <t>] [--count]`

**Purpose:** List all tables in a domain. Use for browsing or getting a count.

```
oracle-fusion-schema tables hcm --count --json
oracle-fusion-schema tables financials --type view --json
```

**JSON shape (array):**
```json
[
  {
    "name": "PER_ALL_PEOPLE_F",
    "domain": "OEDMH",
    "module": "",
    "type": "Tables",
    "description": "This table will store core personal data..."
  }
]
```

**Flags:**
- `--type table` or `--type view` -- Filter by object type.
- `--module "General Ledger"` -- Filter by module (when available).
- `--count` -- Return count only, no table list.

---

### 6. `mapping lookup <ebs-table> [--json]`

**Purpose:** Map an Oracle EBS (E-Business Suite) table name to its Fusion Cloud equivalent.

**When to use:** Migrating an existing EBS report to Fusion, or when requirements reference EBS table names.

```
oracle-fusion-schema mapping lookup MTL_SYSTEM_ITEMS_B --json
```

**JSON shape:**
```json
{
  "ebs_name": "MTL_SYSTEM_ITEMS_B",
  "fusion_name": "EGP_SYSTEM_ITEMS_B",
  "module": "INV",
  "notes": "item_number replaces segment1"
}
```

**30 built-in mappings include:**

| EBS Table                    | Fusion Table                      | Notes                          |
|------------------------------|-----------------------------------|--------------------------------|
| MTL_SYSTEM_ITEMS_B           | EGP_SYSTEM_ITEMS_B                | item_number replaces segment1  |
| MTL_ONHAND_QUANTITIES_DETAIL | INV_ONHAND_QUANTITIES_DETAIL      |                                |
| PO_VENDORS                   | POZ_SUPPLIERS_V                   |                                |
| PO_VENDOR_SITES_ALL          | POZ_SUPPLIER_SITES_V              |                                |
| PA_PROJECTS_ALL              | PJF_PROJECTS_ALL_B                |                                |
| AP_INVOICES_ALL              | AP_INVOICES_ALL                   | same name                      |
| GL_JE_HEADERS                | GL_JE_HEADERS                     | same name                      |
| HR_ALL_ORGANIZATION_UNITS    | HR_ALL_ORGANIZATION_UNITS_F       | date-effective in Fusion       |
| PER_ALL_PEOPLE_F             | PER_ALL_PEOPLE_F                  | same name                      |
| FND_LOOKUP_VALUES            | FND_LOOKUP_VALUES_VL              | _VL for translated view        |

**Subcommands:**
- `mapping list [--json]` -- Show all built-in + custom mappings.
- `mapping add <ebs> <fusion> [--json]` -- Register a custom mapping.

---

### 7. `domains [--json]`

**Purpose:** List all indexed domains with their table counts.

```
oracle-fusion-schema domains --json
```

**JSON shape (array):**
```json
[
  {"code": "OEDMH", "name": "HCM", "base_url": "https://docs.oracle.com/...", "table_count": 5632},
  {"code": "OEDSC", "name": "SCM", "base_url": "https://docs.oracle.com/...", "table_count": 5352}
]
```

---

### 8. `export [--format json|csv] [--domain <d>] [--output <file>] [--json]`

**Purpose:** Bulk export cached schema data. Useful for feeding into other tools or building custom indexes.

```
oracle-fusion-schema export --format json --domain hcm --output hcm-schema.json
```

---

### 9. `sync [--domain <d>] [--workers N] [--force] [--quiet]`

**Purpose:** Download and index documentation from docs.oracle.com. Only needed on first setup or to refresh data.

```
oracle-fusion-schema sync                        # All domains
oracle-fusion-schema sync --domain hcm --force   # Re-sync one domain
```

The database is pre-populated. You should NOT need to run sync during normal BIP report development.

---

## BIP Report Development Workflows

### Workflow A: Build a new report from a business requirement

```
Step 1: Identify tables
  oracle-fusion-schema search "employee absence" --json

Step 2: Get data source for each table
  oracle-fusion-schema datasource ANC_ABSENCE_ENTRIES --json
  --> ApplicationDB_HCM

Step 3: Get full schema for primary tables
  oracle-fusion-schema describe ANC_ABSENCE_ENTRIES --json
  oracle-fusion-schema describe PER_ALL_PEOPLE_F --json

Step 4: Find join columns
  oracle-fusion-schema columns PERSON_ID --domain hcm --json
  --> Both tables have PERSON_ID, use it as the join key

Step 5: Write the SQL using column names, types, and nullability from describe output
```

### Workflow B: Convert an EBS report to Fusion

```
Step 1: Map EBS tables to Fusion
  oracle-fusion-schema mapping lookup MTL_SYSTEM_ITEMS_B --json
  --> EGP_SYSTEM_ITEMS_B

Step 2: Compare columns (old EBS SQL references SEGMENT1, Fusion uses ITEM_NUMBER)
  oracle-fusion-schema describe EGP_SYSTEM_ITEMS_B --json

Step 3: Get the correct data source
  oracle-fusion-schema datasource EGP_SYSTEM_ITEMS_B --json
  --> ApplicationDB_FSCM

Step 4: Rewrite SQL using new table/column names
```

### Workflow C: Find the right table when you only know the column

```
Step 1: Search for the column
  oracle-fusion-schema columns INVOICE_NUM --json

Step 2: Pick the best table from results, describe it
  oracle-fusion-schema describe AP_INVOICES_ALL --json

Step 3: Get data source
  oracle-fusion-schema datasource AP_INVOICES_ALL --json
```

### Workflow D: Cross-data-source report (e.g., HR + Finance)

```
Step 1: Identify data sources for each table
  oracle-fusion-schema datasource PER_ALL_PEOPLE_F --json   --> ApplicationDB_HCM
  oracle-fusion-schema datasource GL_JE_HEADERS --json      --> ApplicationDB_FSCM

Step 2: These are DIFFERENT data sources -- you need two data sets in the BIP data model
  Data Set 1 (ApplicationDB_HCM): SELECT ... FROM PER_ALL_PEOPLE_F WHERE ...
  Data Set 2 (ApplicationDB_FSCM): SELECT ... FROM GL_JE_HEADERS WHERE ...

Step 3: Link data sets at the data model level using a shared key (e.g., PERSON_ID)
```

---

## Tips for Agents

1. **Always use `--json`** for programmatic consumption. Human-readable table output is for display only.

2. **Call `datasource` before writing any SQL.** The data source determines the JDBC connection in the BIP data model XML. Getting it wrong means the report fails at runtime.

3. **Use `describe` to verify column existence before referencing in SQL.** Oracle Fusion column names differ from EBS. Don't assume -- verify.

4. **`search` returns three match types.** Prioritize `table_name` matches over `column_name` over `description`. Table name matches are the most reliable indicator.

5. **Views (suffix `_V`, `_VL`, `_ALL_V`) are often better for BIP reports** than base tables. They pre-join common lookups and handle translations. Search for `_V` or `_VL` variants when available.

6. **Date-effective tables (suffix `_F`)** require `SYSDATE BETWEEN EFFECTIVE_START_DATE AND EFFECTIVE_END_DATE` in your WHERE clause.

7. **The `describe` output includes `doc_url`** linking to the official Oracle docs page. Reference it when you need to understand business rules beyond column definitions.

8. **`columns` with common FK names** is the fastest way to discover join paths:
   - `PERSON_ID`, `ASSIGNMENT_ID` -- HCM joins
   - `LEDGER_ID`, `CODE_COMBINATION_ID` -- GL joins
   - `PO_HEADER_ID`, `PO_LINE_ID` -- Procurement joins
   - `INVENTORY_ITEM_ID`, `ORGANIZATION_ID` -- SCM joins
   - `PARTY_ID`, `CUST_ACCOUNT_ID` -- CRM/Sales joins
   - `BUSINESS_UNIT_ID` -- Cross-module joins

9. **Prefix tells you the module:**
   - `PER_` = People, `PAY_` = Payroll, `BEN_` = Benefits, `ANC_` = Absence
   - `GL_` = General Ledger, `AP_` = Payables, `AR_` = Receivables
   - `PO_` = Purchasing, `INV_` = Inventory, `EGP_` = Product Hub
   - `PJF_`/`PJC_` = Projects, `FA_` = Fixed Assets, `XLA_` = Subledger Accounting
   - `ZCA_` = Common CRM, `ZMM_` = Sales Activities, `ZSO_` = Sales Content

10. **If mapping lookup returns nothing,** the table name may be the same in EBS and Fusion (e.g., `AP_INVOICES_ALL`, `GL_JE_HEADERS`). Try `describe` directly with the EBS name.
