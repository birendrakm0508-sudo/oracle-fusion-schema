# oracle-fusion-schema

Offline CLI for querying Oracle Fusion Cloud's **Tables and Views**
documentation. Indexes 21,718 tables/views across all 7 SaaS domains into a
local SQLite database for instant schema lookup — no network required after
initial sync.

Built as a companion tool for AI agents that write BI Publisher (BIP) reports
against Oracle Fusion Cloud ERP.

## Why

Writing BIP report SQL requires knowing exact column names, data types, join
keys, and the correct JDBC data source. Getting any of these wrong produces a
data model that imports cleanly but fails at execute time — the worst kind of
failure.

Oracle's Tables and Views documentation spans 21,000+ pages across 7 domains.
Searching it manually is slow. Scraping it at runtime burns tokens and network
calls.

This CLI downloads everything once, indexes it locally, and answers schema
questions in milliseconds.

## Install

Requires [Go 1.22+](https://go.dev/dl/).

```bash
go install github.com/birendrakm0508-sudo/oracle-fusion-schema@latest
```

Verify:

```bash
oracle-fusion-schema version
# oracle-fusion-schema 1.0.0 (linux/amd64, go1.24.4)
```

## First-Time Setup

Download and index all Oracle Fusion documentation:

```bash
oracle-fusion-schema sync
```

This fetches the Table of Contents page for each domain, discovers every
table/view page, scrapes column definitions, primary keys, indexes, and
foreign keys, then stores everything in a local SQLite database.

- **Duration:** ~15-25 minutes on a typical corporate network with the
  default parallelism (`--workers 20`, `--throttle-ms 50`). See
  [Performance & Parallelism Tuning](#performance--parallelism-tuning)
  below for faster configs.
- **Database location:** `~/.oracle-fusion-schema/schema.db`
- **Database size:** ~191 MB

Sync individual domains:

```bash
oracle-fusion-schema sync --domain hcm
oracle-fusion-schema sync --domain financials --workers 40 --throttle-ms 0
oracle-fusion-schema sync --domain scm --force     # re-download even if cached
```

## Performance & Parallelism Tuning

The sync command's throughput is governed by two flags. Defaults were raised
in the 2026-06-27 release after measuring real-world wall-clock times.

| Flag | Default | What it does |
|------|---------|--------------|
| `--workers N` | **20** (was 5 prior to 2026-06-27) | Concurrent fetcher goroutines. Doubling workers roughly doubles throughput until the network or `docs.oracle.com`'s CDN becomes the bottleneck. |
| `--throttle-ms M` | **50** (was hardcoded 200 prior to 2026-06-27) | Per-request sleep in milliseconds after each fetch. Set to `0` to disable on fast networks. Raise if you see HTTP 429 responses. |

**Three tuning presets:**

| Config | Estimated wall-clock for full `--force` sync | When to use |
|--------|----------------------------------------------|-------------|
| Defaults (`--workers 20 --throttle-ms 50`) | **~15-25 min** | Typical corporate network with proxy. |
| Aggressive (`--workers 40 --throttle-ms 0`) | **~5-8 min** | Fast network, no proxy, low latency to `docs.oracle.com`. |
| Conservative (`--workers 10 --throttle-ms 200`) | ~30-45 min | If `docs.oracle.com` starts returning HTTP 429. |

**Benchmark** (Common domain, 295 tables, default config): **14 seconds wall
clock** — extrapolates to ~17 min for a full 21,718-table `--force` sync.

### Important: do not run two `sync` commands concurrently

The CLI does not (yet) hold a process-level lock on the SQLite database file.
Running two `sync` invocations at the same time will produce a
`SQLITE_BUSY` cascade plus the cryptic
`cannot start a transaction within a transaction` error, and can leave
domains partially populated. Run one sync at a time.

## What Gets Indexed

| Domain | Code | Tables/Views | Data Source | Example Prefixes |
|--------|------|-------------|-------------|------------------|
| HCM | OEDMH | 5,632 | ApplicationDB_HCM | PAY_, PER_, HR_, BEN_, HWM_ |
| SCM | OEDSC | 5,352 | ApplicationDB_FSCM | INV_, EGP_, MNT_, WIE_ |
| Sales/CX | OEDMS | 4,439 | ApplicationDB_CRM | ZMM_, ZCA_, ZSO_, MOO_ |
| Financials | OEDMF | 4,014 | ApplicationDB_FSCM | GL_, AP_, AR_, FA_, XLA_ |
| Project Management | OEDPP | 1,334 | ApplicationDB_FSCM | PJF_, PJC_, PJB_, PJS_, GMS_ |
| Procurement | OEDMP | 652 | ApplicationDB_FSCM | PO_, PON_, POZ_, ICX_ |
| Common | OEDMA | 295 | ApplicationDB_FSCM | FND_, PER_ |

For each table, the index stores:
- Table name, type (Table/View), domain, module
- Full description from Oracle docs
- Every column: name, data type, length, precision, nullability, description
- Primary keys with column lists
- Indexes with column lists and uniqueness
- Foreign key relationships
- Direct link to the Oracle documentation page

## Usage

### Look up a table schema

```bash
oracle-fusion-schema describe PER_ALL_PEOPLE_F
```

```
Table:       PER_ALL_PEOPLE_F
Type:        Tables
Domain:      OEDMH (HCM)
Data Source: ApplicationDB_HCM
Description: This table will store core personal data...

Columns (105):
  #  COLUMN                TYPE       LEN  NULL  DESCRIPTION
  1  PERSON_ID             NUMBER          N     System generated surrogate key.
  2  EFFECTIVE_START_DATE  DATE            N     Date at the beginning of the date range...
  3  EFFECTIVE_END_DATE    DATE            N     Date at the end of the date range...
  ...
```

### Identify the BIP data source

```bash
oracle-fusion-schema datasource GL_JE_HEADERS
```

```
Table:       GL_JE_HEADERS
Data Source: ApplicationDB_FSCM
Prefix:      GL_
Notes:       Financials, SCM, Procurement, Projects, Order Management
```

### Search by business concept

```bash
oracle-fusion-schema search "employee absence" --domain hcm
```

```
TABLE                    DOMAIN  MATCH TYPE   MATCH            DESCRIPTION
ANC_ABSENCE_ENTRIES      OEDMH   table_name   ANC_ABSENCE...   This table stores absence entries...
ANC_ABSENCE_TYPES_F_VL   OEDMH   table_name   ANC_ABSENCE...   Translated view for absence types...
```

### Find join columns

```bash
oracle-fusion-schema columns PERSON_ID --domain hcm --exact --limit 10
```

```
COLUMN      TABLE                DOMAIN  TYPE    DESCRIPTION
PERSON_ID   PER_ALL_PEOPLE_F     OEDMH   NUMBER  System generated surrogate key.
PERSON_ID   PER_ALL_ASSIGNMENTS  OEDMH   NUMBER  Person identifier.
PERSON_ID   PAY_PAYROLL_ACTIONS  OEDMH   NUMBER  Foreign key to PER_ALL_PEOPLE_F.
```

### Map EBS tables to Fusion

```bash
oracle-fusion-schema mapping lookup MTL_SYSTEM_ITEMS_B
```

```
EBS:    MTL_SYSTEM_ITEMS_B
Fusion: EGP_SYSTEM_ITEMS_B
Module: INV
Notes:  item_number replaces segment1
```

EBS table names passed to `describe` are auto-mapped:

```bash
oracle-fusion-schema describe MTL_SYSTEM_ITEMS_B
# Automatically describes EGP_SYSTEM_ITEMS_B
```

### List tables in a domain

```bash
oracle-fusion-schema tables hcm
oracle-fusion-schema tables financials --type view
oracle-fusion-schema tables scm --count
```

### List domains

```bash
oracle-fusion-schema domains
```

### Export

```bash
oracle-fusion-schema export --format json --domain hcm --output hcm-schema.json
oracle-fusion-schema export --format csv --output all-tables.csv
```

## JSON Output

Every command supports `--json` for machine-readable output:

```bash
oracle-fusion-schema describe GL_JE_HEADERS --json
oracle-fusion-schema datasource PER_ALL_PEOPLE_F --json
oracle-fusion-schema search payroll --json --limit 5
oracle-fusion-schema columns LEDGER_ID --json --exact
oracle-fusion-schema mapping lookup PO_VENDORS --json
oracle-fusion-schema domains --json
```

## EBS-to-Fusion Mappings

30 built-in mappings for common EBS tables:

| EBS | Fusion | Notes |
|-----|--------|-------|
| MTL_SYSTEM_ITEMS_B | EGP_SYSTEM_ITEMS_B | item_number replaces segment1 |
| MTL_ONHAND_QUANTITIES_DETAIL | INV_ONHAND_QUANTITIES_DETAIL | |
| PO_VENDORS | POZ_SUPPLIERS_V | |
| PO_VENDOR_SITES_ALL | POZ_SUPPLIER_SITES_V | |
| PA_PROJECTS_ALL | PJF_PROJECTS_ALL_B | |
| AP_INVOICES_ALL | AP_INVOICES_ALL | same name |
| GL_JE_HEADERS | GL_JE_HEADERS | same name |
| HR_ALL_ORGANIZATION_UNITS | HR_ALL_ORGANIZATION_UNITS_F | date-effective |
| PER_ALL_PEOPLE_F | PER_ALL_PEOPLE_F | same name |
| FND_LOOKUP_VALUES | FND_LOOKUP_VALUES_VL | _VL translated view |

Add custom mappings:

```bash
oracle-fusion-schema mapping add MY_EBS_TABLE MY_FUSION_TABLE
oracle-fusion-schema mapping list
```

## Data Source Rules

BIP reports need the correct JDBC data source. The `datasource` command
determines it from the table prefix:

| Data Source | Prefixes |
|-------------|----------|
| ApplicationDB_HCM | PAY_, PER_, HR_, BEN_, HWM_, HRC_, HRI_, ANC_, CMP_ |
| ApplicationDB_FSCM | GL_, AP_, AR_, PO_, PON_, POZ_, INV_, EGP_, FA_, XLA_, CST_, RCV_, ICX_, ASO_, PJF_, PJC_, PJB_, PJS_, PJT_, PJO_, PJR_, PJE_, PJL_, GMS_ |
| ApplicationDB_CRM | ZMM_, ZCA_, ZSO_, MOO_, MKL_, MKT_, HBY_, CN_ |

If a report joins tables from different data sources, the BIP data model
needs separate data sets (one per data source) linked by a shared key.

## Architecture

```
oracle-fusion-schema/
├── main.go                          # Entry point
├── cmd/                             # Cobra CLI commands
│   ├── root.go                      # --db, --json global flags
│   ├── sync.go                      # Download and index docs
│   ├── describe.go                  # Full table schema
│   ├── search.go                    # Full-text search
│   ├── columns.go                   # Column lookup across tables
│   ├── tables.go                    # List tables by domain
│   ├── domains.go                   # List domains
│   ├── datasource.go               # BIP data source identification
│   ├── mapping_cmd.go              # EBS-to-Fusion mapping
│   ├── export.go                    # JSON/CSV export
│   └── version.go                   # Version info
├── internal/
│   ├── model/model.go              # Data types
│   ├── db/db.go                     # SQLite storage (pure Go, no CGO)
│   ├── scraper/scraper.go          # Oracle docs scraper
│   └── mapping/mapping.go          # EBS mapping rules
├── TOOL_USE.md                      # Agent tool use reference
└── README.md                        # This file
```

**Dependencies:**
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — Pure Go SQLite (no CGO required)
- [spf13/cobra](https://github.com/spf13/cobra) — CLI framework
- [golang.org/x/net/html](https://pkg.go.dev/golang.org/x/net/html) — HTML parser
- [olekukonko/tablewriter](https://github.com/olekukonko/tablewriter) — Terminal table formatting

## For AI Agents

See [`TOOL_USE.md`](TOOL_USE.md) for a complete agent-oriented reference
including JSON response shapes, BIP workflow patterns, join-path discovery
techniques, and integration guidance.

A Claude Code skill is available at
[`skills/oracle-fusion-schema/SKILL.md`](skills/oracle-fusion-schema/SKILL.md)
— drop it into `~/.claude/skills/oracle-fusion-schema/` on any machine with
the CLI installed.

## License

Internal use.
