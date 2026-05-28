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
	searchDomain string
	searchColumn bool
	searchDesc   bool
	searchLimit  int
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search across tables, columns, and descriptions",
	Long: `Full-text search across table names, column names, and descriptions.
Searches all domains by default; filter with --domain.`,
	Example: `  oracle-fusion-schema search payroll
  oracle-fusion-schema search ORGANIZATION --domain hcm
  oracle-fusion-schema search invoice --column
  oracle-fusion-schema search "cost allocation" --description`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().StringVar(&searchDomain, "domain", "", "Filter by domain")
	searchCmd.Flags().BoolVar(&searchColumn, "column", false, "Search column names only")
	searchCmd.Flags().BoolVar(&searchDesc, "description", false, "Search descriptions only")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 50, "Maximum results")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	if err := requireSynced(store); err != nil {
		return err
	}

	query := strings.Join(args, " ")
	domain := ""
	if searchDomain != "" {
		domain = resolveDomainCode(searchDomain)
	}

	searchType := ""
	if searchColumn {
		searchType = "column"
	} else if searchDesc {
		searchType = "description"
	}

	results, err := store.Search(query, domain, searchType, searchLimit)
	if err != nil {
		return err
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	if len(results) == 0 {
		fmt.Printf("No results for %q\n", query)
		return nil
	}

	tbl := tablewriter.NewWriter(os.Stdout)
	tbl.SetHeader([]string{"Table", "Domain", "Match Type", "Match", "Description"})
	tbl.SetBorder(false)
	tbl.SetColumnSeparator("  ")
	tbl.SetAutoWrapText(false)

	for _, r := range results {
		desc := r.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		match := r.MatchField
		if len(match) > 30 {
			match = match[:27] + "..."
		}
		tbl.Append([]string{r.TableName, r.Domain, r.MatchType, match, desc})
	}

	tbl.Render()
	fmt.Printf("\n%d results for %q\n", len(results), query)
	return nil
}
