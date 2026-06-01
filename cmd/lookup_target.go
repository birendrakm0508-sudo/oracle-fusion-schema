package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/birendrakm0508-sudo/oracle-fusion-schema/internal/lookup"
)

var lookupStrict bool

var lookupTargetCmd = &cobra.Command{
	Use:   "lookup-target <TABLE.COLUMN>",
	Short: "Resolve a column to its canonical lookup table chain",
	Long: `Given a fully-qualified TABLE.COLUMN reference, resolve it to the canonical
lookup table chain (_B / _TL / _VL) that translates the column's coded values
into human-readable names.

Resolution cascade:
  1. FK metadata (from Oracle docs) — confidence: high
  2. Naming conventions (*_TYPE_ID, *_CODE, *_STATUS) — confidence: medium
  3. Static map of top-20 cross-module FK targets — confidence: medium

Derives entirely from data the CLI already indexes — no new data ingestion required.`,
	Example: `  oracle-fusion-schema lookup-target INV_RESERVATIONS.SUPPLY_SOURCE_TYPE_ID
  oracle-fusion-schema lookup-target AP_INVOICES_ALL.VENDOR_ID --json
  oracle-fusion-schema lookup-target EGP_SYSTEM_ITEMS_B.ITEM_TYPE --json
  oracle-fusion-schema lookup-target HZ_PARTIES.PARTY_TYPE --strict`,
	Args: cobra.ExactArgs(1),
	RunE: runLookupTarget,
}

func init() {
	lookupTargetCmd.Flags().BoolVar(&lookupStrict, "strict", false, "Fail with non-zero exit if no target found")
	rootCmd.AddCommand(lookupTargetCmd)
}

func runLookupTarget(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	if err := requireSynced(store); err != nil {
		return err
	}

	// Parse TABLE.COLUMN
	input := args[0]
	parts := strings.SplitN(input, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid input %q: expected TABLE.COLUMN format (e.g., INV_RESERVATIONS.SUPPLY_SOURCE_TYPE_ID)", input)
	}
	tableName := strings.ToUpper(parts[0])
	columnName := strings.ToUpper(parts[1])

	result, err := lookup.Resolve(store, tableName, columnName)
	if err != nil {
		return err
	}

	// Handle --strict with no target found
	if lookupStrict && result.Target == nil {
		if jsonOut {
			errResult := lookup.ErrorResult{
				Error:   "no_target_found",
				Source:  result.Source,
				Message: "No FK metadata and no naming-convention match. Inspect 'describe' output and verify conventions.",
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(errResult)
		} else {
			fmt.Fprintf(os.Stderr, "No target found for %s.%s\n", tableName, columnName)
			fmt.Fprintf(os.Stderr, "No FK metadata and no naming-convention match.\n")
			fmt.Fprintf(os.Stderr, "Try: oracle-fusion-schema describe %s\n", tableName)
		}
		os.Exit(1)
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Human-readable output
	fmt.Printf("Source:     %s.%s (%s)\n", result.Source.Table, result.Source.Column, result.Source.DataType)
	fmt.Println()

	if result.Target == nil {
		fmt.Println("Target:    (none found)")
		fmt.Printf("Method:    %s\n", result.Resolution.Method)
		fmt.Printf("Note:      %s\n", result.Resolution.Note)
		return nil
	}

	fmt.Printf("Target:    %s\n", result.Target.BaseTable)
	if result.Target.TLTable != nil {
		fmt.Printf("  _TL:     %s\n", *result.Target.TLTable)
	}
	if result.Target.VLView != nil {
		fmt.Printf("  _VL:     %s\n", *result.Target.VLView)
	}
	fmt.Printf("Join:      %s = %s\n", result.Source.Column, result.Target.JoinColumn)
	if result.Target.NameColumn != "" {
		fmt.Printf("Name col:  %s\n", result.Target.NameColumn)
	}
	if result.Target.DescriptionColumn != "" {
		fmt.Printf("Desc col:  %s\n", result.Target.DescriptionColumn)
	}
	fmt.Printf("Data src:  %s\n", result.Target.DataSource)
	if result.Target.LookupType != "" {
		fmt.Printf("Lookup:    %s\n", result.Target.LookupType)
	}
	fmt.Println()
	fmt.Printf("Method:    %s (confidence: %s)\n", result.Resolution.Method, result.Resolution.Confidence)
	if result.Resolution.Note != "" {
		fmt.Printf("Note:      %s\n", result.Resolution.Note)
	}

	if result.SampleJoinSQL != "" {
		fmt.Println()
		fmt.Println("Sample JOIN SQL:")
		for _, line := range strings.Split(result.SampleJoinSQL, "\n") {
			fmt.Printf("  %s\n", line)
		}
	}

	return nil
}
