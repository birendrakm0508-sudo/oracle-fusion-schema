package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/birendrakm0508-sudo/oracle-fusion-schema/internal/lookup"
	"github.com/birendrakm0508-sudo/oracle-fusion-schema/internal/mapping"
)

var (
	descColumnsOnly bool
	descCompact     bool
)

var describeCmd = &cobra.Command{
	Use:   "describe <table-name>",
	Short: "Show full schema for a table or view",
	Long: `Display complete schema documentation for a specific Oracle Fusion table or view,
including columns, data types, primary keys, indexes, and foreign key relationships.

If an EBS table name is provided, automatically maps to the Fusion equivalent.`,
	Example: `  oracle-fusion-schema describe PER_ALL_PEOPLE_F
  oracle-fusion-schema describe GL_JE_HEADERS --columns-only
  oracle-fusion-schema describe MTL_SYSTEM_ITEMS_B  # auto-maps to EGP_SYSTEM_ITEMS_B
  oracle-fusion-schema describe AP_INVOICES_ALL --json`,
	Args: cobra.ExactArgs(1),
	RunE: runDescribe,
}

func init() {
	describeCmd.Flags().BoolVar(&descColumnsOnly, "columns-only", false, "Show only column definitions")
	describeCmd.Flags().BoolVar(&descCompact, "compact", false, "Compact output format")
	rootCmd.AddCommand(describeCmd)
}

func runDescribe(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	if err := requireSynced(store); err != nil {
		return err
	}

	tableName := strings.ToUpper(args[0])

	// Check for EBS mapping
	ebsMap := mapping.LookupEBS(tableName)
	if ebsMap != nil && ebsMap.EBSName != ebsMap.FusionName {
		fmt.Fprintf(os.Stderr, "Note: %s is an EBS name. Mapped to Fusion: %s\n", ebsMap.EBSName, ebsMap.FusionName)
		if ebsMap.Notes != "" {
			fmt.Fprintf(os.Stderr, "      %s\n", ebsMap.Notes)
		}
		fmt.Fprintln(os.Stderr)
		tableName = ebsMap.FusionName
	}

	t, err := store.GetTable(tableName)
	if err != nil {
		return fmt.Errorf("lookup table: %w", err)
	}
	if t != nil && strings.Contains(strings.ToUpper(t.Type), "VIEW") {
		// View pages publish column names but no types. Enrich the scraped names
		// with type/description from the underlying _B / _TL tables (falling back
		// to _B+_TL synthesis if the view has no scraped columns).
		cols, source := lookup.ResolveViewColumns(store, t)
		t.Columns = cols
		t.ColumnSource = source
	}
	if t == nil {
		// Try case-insensitive partial match
		results, _ := store.Search(tableName, "", "table", 5)
		if len(results) > 0 {
			fmt.Fprintf(os.Stderr, "Table %q not found. Did you mean:\n", tableName)
			for _, r := range results {
				fmt.Fprintf(os.Stderr, "  - %s (%s)\n", r.TableName, r.Domain)
			}
			return fmt.Errorf("table not found")
		}
		return fmt.Errorf("table %q not found in cache. Run 'oracle-fusion-schema sync' to update", tableName)
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(t)
	}

	// Header
	if !descCompact {
		ds := mapping.IdentifyDataSource(t.Name)
		fmt.Printf("Table:       %s\n", t.Name)
		fmt.Printf("Type:        %s\n", t.Type)
		fmt.Printf("Domain:      %s (%s)\n", t.Domain, domainName(t.Domain))
		fmt.Printf("Module:      %s\n", t.Module)
		fmt.Printf("Data Source: %s\n", ds.DataSource)
		if t.ColumnSource != "" {
			fmt.Printf("Col Source:  %s\n", t.ColumnSource)
		}
		if t.Description != "" {
			fmt.Printf("Description: %s\n", t.Description)
		}
		fmt.Printf("Doc URL:     %s\n", t.DocURL)
		fmt.Println()
	}

	// Columns
	if len(t.Columns) > 0 {
		if !descCompact {
			fmt.Printf("Columns (%d):\n", len(t.Columns))
		}
		tbl := tablewriter.NewWriter(os.Stdout)
		tbl.SetHeader([]string{"#", "Column", "Type", "Len", "Null", "Description"})
		tbl.SetBorder(false)
		tbl.SetColumnSeparator("  ")
		tbl.SetAutoWrapText(false)
		tbl.SetColWidth(60)

		for _, col := range t.Columns {
			nullable := "Y"
			if !col.Nullable {
				nullable = "N"
			}
			desc := col.Description
			if descCompact && len(desc) > 50 {
				desc = desc[:47] + "..."
			}
			tbl.Append([]string{
				fmt.Sprintf("%d", col.Position),
				col.Name,
				col.DataType,
				col.Length,
				nullable,
				desc,
			})
		}
		tbl.Render()
	}

	if descColumnsOnly {
		return nil
	}

	// Primary Key
	if t.PrimaryKey != nil && t.PrimaryKey.Name != "" {
		fmt.Printf("\nPrimary Key: %s\n", t.PrimaryKey.Name)
		if len(t.PrimaryKey.Columns) > 0 {
			fmt.Printf("  Columns: %s\n", strings.Join(t.PrimaryKey.Columns, ", "))
		}
	}

	// Indexes
	if len(t.Indexes) > 0 {
		fmt.Printf("\nIndexes (%d):\n", len(t.Indexes))
		tbl := tablewriter.NewWriter(os.Stdout)
		tbl.SetHeader([]string{"Name", "Unique", "Columns"})
		tbl.SetBorder(false)
		tbl.SetColumnSeparator("  ")
		for _, idx := range t.Indexes {
			unique := "N"
			if idx.Unique {
				unique = "Y"
			}
			tbl.Append([]string{idx.Name, unique, strings.Join(idx.Columns, ", ")})
		}
		tbl.Render()
	}

	// Foreign Keys
	if len(t.ForeignKeys) > 0 {
		fmt.Printf("\nForeign Keys (%d):\n", len(t.ForeignKeys))
		tbl := tablewriter.NewWriter(os.Stdout)
		tbl.SetHeader([]string{"Referencing Table", "Column", "Relationship"})
		tbl.SetBorder(false)
		tbl.SetColumnSeparator("  ")
		for _, fk := range t.ForeignKeys {
			tbl.Append([]string{fk.ReferencingTable, fk.FKColumn, fk.Relationship})
		}
		tbl.Render()
	}

	// EBS mapping note
	reverseMap := mapping.LookupFusion(t.Name)
	if len(reverseMap) > 0 {
		fmt.Printf("\nEBS Equivalents:\n")
		for _, m := range reverseMap {
			note := ""
			if m.Notes != "" {
				note = " (" + m.Notes + ")"
			}
			fmt.Printf("  %s -> %s%s\n", m.EBSName, m.FusionName, note)
		}
	}

	return nil
}

func domainName(code string) string {
	names := map[string]string{
		"OEDMF": "Financials",
		"OEDSC": "SCM",
		"OEDMH": "HCM",
		"OEDMP": "Procurement",
		"OEDMS": "Sales/CX",
		"OEDMA": "Common",
		"OEDPP": "Project Management",
	}
	if n, ok := names[code]; ok {
		return n
	}
	return code
}
