package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/birendrakm0508-sudo/oracle-fusion-schema/internal/scraper"
)

var (
	syncDomain  string
	syncWorkers int
	syncForce   bool
	syncQuiet   bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Download and index Oracle Fusion schema documentation",
	Long: `Scrape all Tables and Views documentation from docs.oracle.com and store
in a local SQLite database for offline access.

This fetches the TOC page for each domain, discovers all table/view pages,
then scrapes each page for column definitions, primary keys, indexes, and
foreign key relationships.

First sync may take 15-30 minutes depending on network speed.`,
	Example: `  oracle-fusion-schema sync
  oracle-fusion-schema sync --domain hcm
  oracle-fusion-schema sync --workers 10
  oracle-fusion-schema sync --force`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().StringVar(&syncDomain, "domain", "", "Sync specific domain only (e.g., hcm, financials)")
	syncCmd.Flags().IntVar(&syncWorkers, "workers", 5, "Number of parallel fetchers")
	syncCmd.Flags().BoolVar(&syncForce, "force", false, "Re-sync even if data exists")
	syncCmd.Flags().BoolVar(&syncQuiet, "quiet", false, "Suppress progress output")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer store.Close()

	domains := scraper.KnownDomains()

	// Filter by domain if specified
	if syncDomain != "" {
		var filtered []struct {
			d    interface{}
			name string
		}
		_ = filtered
		var matchedDomains []struct{ d interface{} }
		_ = matchedDomains

		upper := strings.ToUpper(syncDomain)
		var found bool
		for i, d := range domains {
			if strings.EqualFold(d.Code, syncDomain) || strings.EqualFold(d.Name, syncDomain) || strings.Contains(strings.ToUpper(d.Name), upper) {
				domains = domains[i : i+1]
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown domain: %s (use 'oracle-fusion-schema domains' to list)", syncDomain)
		}
	}

	if !syncQuiet {
		fmt.Printf("Syncing %d domain(s) to %s\n\n", len(domains), store.Path())
	}

	totalTables := 0
	totalErrors := 0

	for _, domain := range domains {
		// Register domain in DB
		if err := store.UpsertDomain(domain); err != nil {
			return fmt.Errorf("register domain %s: %w", domain.Code, err)
		}

		if !syncQuiet {
			fmt.Printf("[%s] %s — fetching TOC...\n", domain.Code, domain.Name)
		}

		// Fetch TOC
		entries, err := scraper.FetchTOC(domain)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "  ERROR: %v\n", err)
			totalErrors++
			continue
		}

		if !syncQuiet {
			fmt.Printf("[%s] Found %d tables/views\n", domain.Code, len(entries))
		}

		if len(entries) == 0 {
			continue
		}

		// Check if we already have data
		if !syncForce {
			existing, _ := store.DomainTableCount(domain.Code)
			if existing > 0 {
				if !syncQuiet {
					fmt.Printf("[%s] Already have %d tables cached (use --force to re-sync)\n\n", domain.Code, existing)
				}
				continue
			}
		}

		// Clear existing data if force re-sync
		if syncForce {
			store.ClearDomain(domain.Code)
		}

		// Scrape all tables concurrently
		progress := func(domainCode string, current, total int, tableName string) {
			if !syncQuiet {
				pct := float64(current) / float64(total) * 100
				fmt.Printf("\r[%s] %d/%d (%.0f%%) %s", domainCode, current, total, pct, padRight(tableName, 40))
			}
		}

		tables, errs := scraper.ScrapeTablesConcurrently(entries, domain, syncWorkers, progress)

		if !syncQuiet {
			fmt.Println()
		}

		// Store scraped tables
		stored := 0
		for _, t := range tables {
			if err := store.InsertTable(*t); err != nil {
				if !syncQuiet {
					fmt.Fprintf(cmd.ErrOrStderr(), "  store error for %s: %v\n", t.Name, err)
				}
				totalErrors++
			} else {
				stored++
			}
		}

		store.MarkDomainSynced(domain.Code)
		totalTables += stored

		if !syncQuiet {
			fmt.Printf("[%s] Stored %d tables/views", domain.Code, stored)
			if len(errs) > 0 {
				fmt.Printf(" (%d errors)", len(errs))
			}
			fmt.Println()
			fmt.Println()
		}
		totalErrors += len(errs)
	}

	if !syncQuiet {
		fmt.Printf("Sync complete: %d tables/views indexed", totalTables)
		if totalErrors > 0 {
			fmt.Printf(" (%d errors)", totalErrors)
		}
		fmt.Println()
	}

	return nil
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}
