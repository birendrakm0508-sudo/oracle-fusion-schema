package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	exportFormat string
	exportDomain string
	exportOutput string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export schema data in various formats",
	Long: `Export cached schema data to JSON, CSV, or SQL format.
Useful for feeding into other tools or for backup.`,
	Example: `  oracle-fusion-schema export --format json --output schema.json
  oracle-fusion-schema export --format csv --domain hcm --output hcm-tables.csv
  oracle-fusion-schema export --format json`,
	RunE: runExport,
}

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "json", "Output format: json, csv")
	exportCmd.Flags().StringVar(&exportDomain, "domain", "", "Filter by domain")
	exportCmd.Flags().StringVar(&exportOutput, "output", "", "Output file path (default: stdout)")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	if err := requireSynced(store); err != nil {
		return err
	}

	domain := ""
	if exportDomain != "" {
		domain = resolveDomainCode(exportDomain)
	}

	tables, err := store.ListTables(domain, "", "")
	if err != nil {
		return err
	}

	// Determine output writer
	var out *os.File
	if exportOutput != "" {
		out, err = os.Create(exportOutput)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer out.Close()
	} else {
		out = os.Stdout
	}

	switch exportFormat {
	case "json":
		// For JSON, include full details
		type exportTable struct {
			Name        string `json:"name"`
			Domain      string `json:"domain"`
			Module      string `json:"module"`
			Type        string `json:"type"`
			Description string `json:"description"`
		}
		var export []exportTable
		for _, t := range tables {
			export = append(export, exportTable{
				Name:        t.Name,
				Domain:      t.Domain,
				Module:      t.Module,
				Type:        t.Type,
				Description: t.Description,
			})
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(export)

	case "csv":
		w := csv.NewWriter(out)
		defer w.Flush()

		w.Write([]string{"Name", "Domain", "Module", "Type", "Description"})
		for _, t := range tables {
			w.Write([]string{t.Name, t.Domain, t.Module, t.Type, t.Description})
		}
		return w.Error()

	default:
		return fmt.Errorf("unsupported format: %s (use json or csv)", exportFormat)
	}
}
