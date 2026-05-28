package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/birenkumar/oracle-fusion-schema/internal/scraper"
)

var domainsCmd = &cobra.Command{
	Use:   "domains",
	Short: "List Oracle Fusion Cloud documentation domains",
	Example: `  oracle-fusion-schema domains
  oracle-fusion-schema domains --json`,
	RunE: runDomains,
}

func init() {
	rootCmd.AddCommand(domainsCmd)
}

func runDomains(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	// Ensure domains are registered
	for _, d := range scraper.KnownDomains() {
		store.UpsertDomain(d)
	}

	domains, err := store.ListDomains()
	if err != nil {
		return err
	}

	if jsonOut {
		type domainInfo struct {
			Code       string `json:"code"`
			Name       string `json:"name"`
			BaseURL    string `json:"base_url"`
			TableCount int    `json:"table_count"`
		}
		var info []domainInfo
		for _, d := range domains {
			count, _ := store.DomainTableCount(d.Code)
			info = append(info, domainInfo{
				Code:       d.Code,
				Name:       d.Name,
				BaseURL:    d.BaseURL,
				TableCount: count,
			})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Code", "Domain", "Tables", "Base URL"})
	table.SetBorder(false)
	table.SetColumnSeparator("  ")

	for _, d := range domains {
		count, _ := store.DomainTableCount(d.Code)
		countStr := "-"
		if count > 0 {
			countStr = fmt.Sprintf("%d", count)
		}
		table.Append([]string{d.Code, d.Name, countStr, d.BaseURL})
	}

	table.Render()
	return nil
}
