package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	tablesModule string
	tablesType   string
	tablesCount  bool
)

var tablesCmd = &cobra.Command{
	Use:   "tables [domain]",
	Short: "List tables and views for a domain",
	Long: `List all tables and views, optionally filtered by domain, module, or type.
If no domain is specified, lists tables across all domains.`,
	Example: `  oracle-fusion-schema tables
  oracle-fusion-schema tables hcm
  oracle-fusion-schema tables financials --module "General Ledger"
  oracle-fusion-schema tables --type view
  oracle-fusion-schema tables scm --count`,
	RunE: runTables,
}

func init() {
	tablesCmd.Flags().StringVar(&tablesModule, "module", "", "Filter by module name")
	tablesCmd.Flags().StringVar(&tablesType, "type", "", "Filter by type (table or view)")
	tablesCmd.Flags().BoolVar(&tablesCount, "count", false, "Show count only")
	rootCmd.AddCommand(tablesCmd)
}

func runTables(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	if err := requireSynced(store); err != nil {
		return err
	}

	domain := ""
	if len(args) > 0 {
		domain = resolveDomainCode(args[0])
	}

	tables, err := store.ListTables(domain, tablesModule, tablesType)
	if err != nil {
		return err
	}

	if tablesCount {
		if jsonOut {
			out := map[string]int{"count": len(tables)}
			enc := json.NewEncoder(os.Stdout)
			return enc.Encode(out)
		}
		fmt.Printf("%d tables/views\n", len(tables))
		return nil
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(tables)
	}

	if len(tables) == 0 {
		fmt.Println("No tables found matching criteria.")
		return nil
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Domain", "Module", "Type"})
	table.SetBorder(false)
	table.SetColumnSeparator("  ")
	table.SetAutoWrapText(false)

	for _, t := range tables {
		table.Append([]string{t.Name, t.Domain, truncate(t.Module, 30), t.Type})
	}

	table.Render()
	fmt.Printf("\n%d tables/views\n", len(tables))
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// resolveDomainCode converts common domain aliases to their code.
func resolveDomainCode(input string) string {
	aliases := map[string]string{
		"financials":    "OEDMF",
		"fin":           "OEDMF",
		"finance":       "OEDMF",
		"erp":           "OEDMF",
		"scm":           "OEDSC",
		"supply":        "OEDSC",
		"manufacturing": "OEDSC",
		"hcm":           "OEDMH",
		"hr":            "OEDMH",
		"human":         "OEDMH",
		"procurement":   "OEDMP",
		"proc":          "OEDMP",
		"purchasing":    "OEDMP",
		"sales":         "OEDMS",
		"cx":            "OEDMS",
		"crm":           "OEDMS",
		"common":        "OEDMA",
		"apps":          "OEDMA",
		"ppm":           "OEDPP",
		"projects":      "OEDPP",
		"project":       "OEDPP",
		"grants":        "OEDPP",
		// Direct codes
		"oedmf": "OEDMF",
		"oedsc": "OEDSC",
		"oedmh": "OEDMH",
		"oedmp": "OEDMP",
		"oedms": "OEDMS",
		"oedma": "OEDMA",
		"oedpp": "OEDPP",
	}

	if code, ok := aliases[input]; ok {
		return code
	}
	return input
}
