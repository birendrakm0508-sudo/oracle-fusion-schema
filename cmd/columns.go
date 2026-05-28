package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	colExact  bool
	colDomain string
	colLimit  int
)

var columnsCmd = &cobra.Command{
	Use:   "columns <pattern>",
	Short: "Search columns across all tables",
	Long: `Quick column lookup across all tables in the cache.
Useful for finding which tables contain a specific column.`,
	Example: `  oracle-fusion-schema columns ORGANIZATION_ID
  oracle-fusion-schema columns PAYROLL --domain hcm
  oracle-fusion-schema columns INVOICE_NUM --exact`,
	Args: cobra.ExactArgs(1),
	RunE: runColumns,
}

func init() {
	columnsCmd.Flags().BoolVar(&colExact, "exact", false, "Exact match (default is pattern match)")
	columnsCmd.Flags().StringVar(&colDomain, "domain", "", "Filter by domain")
	columnsCmd.Flags().IntVar(&colLimit, "limit", 50, "Maximum results")
	rootCmd.AddCommand(columnsCmd)
}

func runColumns(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	if err := requireSynced(store); err != nil {
		return err
	}

	pattern := args[0]
	domain := ""
	if colDomain != "" {
		domain = resolveDomainCode(colDomain)
	}

	results, err := store.SearchColumns(pattern, domain, colExact, colLimit)
	if err != nil {
		return err
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	if len(results) == 0 {
		fmt.Printf("No columns matching %q\n", pattern)
		return nil
	}

	tbl := tablewriter.NewWriter(os.Stdout)
	tbl.SetHeader([]string{"Column", "Table", "Domain", "Type", "Description"})
	tbl.SetBorder(false)
	tbl.SetColumnSeparator("  ")
	tbl.SetAutoWrapText(false)

	for _, r := range results {
		desc := r.Description
		if len(desc) > 45 {
			desc = desc[:42] + "..."
		}
		tbl.Append([]string{r.ColumnName, r.TableName, r.Domain, r.DataType, desc})
	}

	tbl.Render()
	fmt.Printf("\n%d results for %q\n", len(results), strings.ToUpper(pattern))
	return nil
}
