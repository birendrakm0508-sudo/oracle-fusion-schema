// Package cmd implements the oracle-fusion-schema CLI commands.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/birendrakm0508-sudo/oracle-fusion-schema/internal/db"
)

var (
	dbPath  string
	jsonOut bool
)

var rootCmd = &cobra.Command{
	Use:   "oracle-fusion-schema",
	Short: "Oracle Fusion Cloud schema documentation browser",
	Long: `oracle-fusion-schema indexes and queries Oracle Fusion Cloud's "Tables and Views"
documentation across all SaaS domains (Financials, SCM, HCM, Procurement, CX, Common).

Designed as a companion tool for AI agents building BI Publisher (BIP) reports.
Provides instant schema lookup, EBS-to-Fusion table name mapping, and data source
identification without requiring network access after initial sync.

Quick start:
  oracle-fusion-schema sync          # Download and index all documentation
  oracle-fusion-schema domains       # List available domains
  oracle-fusion-schema tables hcm    # List HCM tables
  oracle-fusion-schema describe PER_ALL_PEOPLE_F
  oracle-fusion-schema search payroll
  oracle-fusion-schema mapping lookup MTL_SYSTEM_ITEMS_B
  oracle-fusion-schema datasource GL_JE_HEADERS`,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "Database file path (default: ~/.oracle-fusion-schema/schema.db)")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "Output in JSON format")
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// openStore opens the database, creating it if necessary.
func openStore() (*db.Store, error) {
	path := dbPath
	if path == "" {
		path = db.DefaultDBPath()
	}
	return db.Open(path)
}

// requireSynced checks that the database has been synced at least once.
func requireSynced(store *db.Store) error {
	count, err := store.TotalTableCount()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("no schema data found. Run 'oracle-fusion-schema sync' first")
	}
	return nil
}
