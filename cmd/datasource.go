package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/birendrakm0508-sudo/oracle-fusion-schema/internal/mapping"
)

var datasourceCmd = &cobra.Command{
	Use:   "datasource <table-name>",
	Short: "Identify the BIP data source for a table",
	Long: `Determine which BI Publisher (BIP) JDBC data source connection to use
for a given Oracle Fusion table. Returns one of:
  - ApplicationDB_HCM  (HCM, Payroll, Benefits, Talent)
  - ApplicationDB_FSCM (Financials, SCM, Procurement, Projects)
  - ApplicationDB_CRM  (CRM, Sales, Marketing)`,
	Example: `  oracle-fusion-schema datasource GL_JE_HEADERS
  oracle-fusion-schema datasource PER_ALL_PEOPLE_F
  oracle-fusion-schema datasource ZMM_ACTIVITIES
  oracle-fusion-schema datasource INV_ONHAND_QUANTITIES_DETAIL --json`,
	Args: cobra.ExactArgs(1),
	RunE: runDatasource,
}

func init() {
	rootCmd.AddCommand(datasourceCmd)
}

func runDatasource(cmd *cobra.Command, args []string) error {
	tableName := strings.ToUpper(args[0])

	// Check if it's an EBS name first
	ebsMap := mapping.LookupEBS(tableName)
	if ebsMap != nil && ebsMap.EBSName != ebsMap.FusionName {
		fmt.Fprintf(os.Stderr, "Note: %s is an EBS name -> Fusion: %s\n\n", ebsMap.EBSName, ebsMap.FusionName)
		tableName = ebsMap.FusionName
	}

	ds := mapping.IdentifyDataSource(tableName)

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(ds)
	}

	fmt.Printf("Table:       %s\n", ds.TableName)
	fmt.Printf("Data Source: %s\n", ds.DataSource)
	if ds.TablePrefix != "" {
		fmt.Printf("Prefix:      %s\n", ds.TablePrefix)
	}
	if ds.Notes != "" {
		fmt.Printf("Notes:       %s\n", ds.Notes)
	}

	return nil
}
