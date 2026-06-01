package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/birendrakm0508-sudo/oracle-fusion-schema/internal/mapping"
	"github.com/birendrakm0508-sudo/oracle-fusion-schema/internal/model"
)

var mappingCmd = &cobra.Command{
	Use:   "mapping",
	Short: "EBS-to-Fusion table name mappings",
	Long: `Manage and query EBS-to-Fusion table name mappings.
These mappings help developers migrating from Oracle E-Business Suite (EBS)
to Oracle Fusion Cloud by showing the equivalent table names.`,
	Example: `  oracle-fusion-schema mapping list
  oracle-fusion-schema mapping lookup MTL_SYSTEM_ITEMS_B
  oracle-fusion-schema mapping add MY_EBS_TABLE MY_FUSION_TABLE`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var mappingListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all EBS-to-Fusion table mappings",
	RunE:  runMappingList,
}

var mappingLookupCmd = &cobra.Command{
	Use:   "lookup <ebs-table-name>",
	Short: "Find the Fusion equivalent of an EBS table",
	Args:  cobra.ExactArgs(1),
	RunE:  runMappingLookup,
}

var mappingAddCmd = &cobra.Command{
	Use:   "add <ebs-name> <fusion-name>",
	Short: "Add a custom EBS-to-Fusion mapping",
	Args:  cobra.ExactArgs(2),
	RunE:  runMappingAdd,
}

func init() {
	mappingCmd.AddCommand(mappingListCmd)
	mappingCmd.AddCommand(mappingLookupCmd)
	mappingCmd.AddCommand(mappingAddCmd)
	rootCmd.AddCommand(mappingCmd)
}

func runMappingList(cmd *cobra.Command, args []string) error {
	builtins := mapping.GetBuiltinMappings()

	// Also load custom mappings from DB
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	custom, _ := store.GetCustomMappings()

	all := append(builtins, custom...)

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(all)
	}

	tbl := tablewriter.NewWriter(os.Stdout)
	tbl.SetHeader([]string{"EBS Table", "Fusion Table", "Module", "Notes"})
	tbl.SetBorder(false)
	tbl.SetColumnSeparator("  ")
	tbl.SetAutoWrapText(false)

	for _, m := range all {
		notes := m.Notes
		if len(notes) > 50 {
			notes = notes[:47] + "..."
		}
		tbl.Append([]string{m.EBSName, m.FusionName, m.Module, notes})
	}

	tbl.Render()
	fmt.Printf("\n%d mappings (%d built-in, %d custom)\n", len(all), len(builtins), len(custom))
	return nil
}

func runMappingLookup(cmd *cobra.Command, args []string) error {
	ebsName := strings.ToUpper(args[0])

	// Check built-in mappings
	m := mapping.LookupEBS(ebsName)

	// Check custom mappings in DB
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	if m == nil {
		// Try database
		custom, _ := store.GetCustomMappings()
		for _, cm := range custom {
			if cm.EBSName == ebsName {
				m = &cm
				break
			}
		}
	}

	if m == nil {
		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			return enc.Encode(map[string]string{"error": "no mapping found", "ebs_name": ebsName})
		}
		fmt.Printf("No mapping found for EBS table: %s\n", ebsName)
		fmt.Println("The table may have the same name in Fusion, or may not be in our mapping database.")
		fmt.Println("Use 'oracle-fusion-schema search' to find it in the schema cache.")
		return nil
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(m)
	}

	fmt.Printf("EBS:    %s\n", m.EBSName)
	fmt.Printf("Fusion: %s\n", m.FusionName)
	fmt.Printf("Module: %s\n", m.Module)
	if m.Notes != "" {
		fmt.Printf("Notes:  %s\n", m.Notes)
	}

	ds := mapping.IdentifyDataSource(m.FusionName)
	fmt.Printf("Data Source: %s\n", ds.DataSource)

	return nil
}

func runMappingAdd(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	m := model.EBSMapping{
		EBSName:    strings.ToUpper(args[0]),
		FusionName: strings.ToUpper(args[1]),
	}

	if err := store.UpsertEBSMapping(m, true); err != nil {
		return fmt.Errorf("save mapping: %w", err)
	}

	fmt.Printf("Added mapping: %s -> %s\n", m.EBSName, m.FusionName)
	return nil
}
