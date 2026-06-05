// Package scraper fetches and parses Oracle Fusion Tables and Views documentation.
package scraper

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"github.com/birendrakm0508-sudo/oracle-fusion-schema/internal/model"
)

// KnownDomains returns all Oracle Fusion Cloud documentation domains.
func KnownDomains() []model.Domain {
	return []model.Domain{
		{Code: "OEDMF", Name: "Financials", BaseURL: "https://docs.oracle.com/en/cloud/saas/financials/25c/oedmf/", TOCURL: "https://docs.oracle.com/en/cloud/saas/financials/25c/oedmf/toc.htm"},
		{Code: "OEDSC", Name: "SCM", BaseURL: "https://docs.oracle.com/en/cloud/saas/supply-chain-and-manufacturing/26b/oedsc/", TOCURL: "https://docs.oracle.com/en/cloud/saas/supply-chain-and-manufacturing/26b/oedsc/toc.htm"},
		{Code: "OEDMH", Name: "HCM", BaseURL: "https://docs.oracle.com/en/cloud/saas/human-resources/oedmh/", TOCURL: "https://docs.oracle.com/en/cloud/saas/human-resources/oedmh/toc.htm"},
		{Code: "OEDMP", Name: "Procurement", BaseURL: "https://docs.oracle.com/en/cloud/saas/procurement/26b/oedmp/", TOCURL: "https://docs.oracle.com/en/cloud/saas/procurement/26b/oedmp/toc.htm"},
		{Code: "OEDMS", Name: "Sales/CX", BaseURL: "https://docs.oracle.com/en/cloud/saas/sales/oedms/", TOCURL: "https://docs.oracle.com/en/cloud/saas/sales/oedms/toc.htm"},
		{Code: "OEDMA", Name: "Common", BaseURL: "https://docs.oracle.com/en/cloud/saas/applications-common/26b/oedma/", TOCURL: "https://docs.oracle.com/en/cloud/saas/applications-common/26b/oedma/toc.htm"},
	}
}

// TOCEntry represents a table/view listed in the TOC page.
type TOCEntry struct {
	Name   string
	URL    string
	Module string
	Type   string // "Tables" or "Views"
}

// ProgressFunc is called with progress updates during scraping.
type ProgressFunc func(domain string, current, total int, tableName string)

// FetchTOC fetches and parses the table of contents page for a domain.
func FetchTOC(domain model.Domain) ([]TOCEntry, error) {
	resp, err := httpGet(domain.TOCURL)
	if err != nil {
		return nil, fmt.Errorf("fetch TOC for %s: %w", domain.Code, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("TOC for %s returned status %d", domain.Code, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read TOC body for %s: %w", domain.Code, err)
	}

	return parseTOC(string(body), domain.BaseURL)
}

// parseTOC extracts table entries from the TOC HTML.
func parseTOC(htmlContent, baseURL string) ([]TOCEntry, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	var entries []TOCEntry
	currentModule := ""
	currentType := "Tables"

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			// Detect module headings from multiple formats:
			// Format 1 (Sales/CX): <strong>Activity Stream for CRM - Tables</strong>
			// Format 2 (Procurement): <a>2</a> + nested section with module name
			// Format 3: <h2>Module Name</h2>

			if isHeading(n) {
				text := getTextContent(n)
				text = strings.TrimSpace(text)
				if text != "" {
					if strings.Contains(text, " - Tables") {
						currentModule = strings.TrimSuffix(text, " - Tables")
						currentType = "TABLE"
					} else if strings.Contains(text, " - Views") {
						currentModule = strings.TrimSuffix(text, " - Views")
						currentType = "VIEW"
					} else if !strings.Contains(text, "Tables and Views") && !strings.Contains(text, "Overview") {
						currentModule = text
					}
				}
			}

			// Detect <strong> elements used as section headings in TOC
			if n.Data == "strong" || n.Data == "b" {
				text := getTextContent(n)
				text = strings.TrimSpace(text)
				if text != "" {
					if strings.Contains(text, " - Tables") {
						currentModule = strings.TrimSuffix(text, " - Tables")
						currentType = "TABLE"
					} else if strings.Contains(text, " - Views") {
						currentModule = strings.TrimSuffix(text, " - Views")
						currentType = "VIEW"
					} else if strings.EqualFold(text, "Tables") {
						currentType = "TABLE"
					} else if strings.EqualFold(text, "Views") {
						currentType = "VIEW"
					} else if len(text) > 2 && !strings.Contains(text, "Tables and Views") &&
						!strings.Contains(text, "Overview") && !isTableLink(text) {
						// Module name without "- Tables/Views" suffix
						// Strip leading section numbers (e.g., "2 Purchasing" -> "Purchasing")
						cleaned := stripSectionNumber(text)
						if cleaned != "" {
							currentModule = cleaned
						}
					}
				}
			}

			// Detect section-level <a> links that serve as module headings
			// e.g., <a href="purchasing-1.html">2 Purchasing</a>
			if n.Data == "a" {
				href := getAttr(n, "href")
				text := getTextContent(n)
				text = strings.TrimSpace(text)

				// Section heading links (not table links) — e.g., "purchasing-1.html" with text "2"
				if href != "" && text != "" && !isTableLink(href) {
					// Check if parent <li> has a sibling <ul> with module name
					// This handles numbered section format
				}

				// Detect table links
				if href != "" && text != "" && isTableLink(href) {
					fullURL := resolveURL(baseURL, href)
					name := strings.TrimSpace(text)

					// Determine type from link context or name
					entryType := currentType
					if strings.HasSuffix(name, "_V") || strings.HasSuffix(name, "_VL") ||
						strings.Contains(name, "_V_") {
						entryType = "VIEW"
					}

					entries = append(entries, TOCEntry{
						Name:   name,
						URL:    fullURL,
						Module: currentModule,
						Type:   entryType,
					})
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)
	return entries, nil
}

// FetchTableDetail fetches and parses an individual table documentation page.
func FetchTableDetail(entry TOCEntry, domain model.Domain) (*model.Table, error) {
	// Strip the anchor from the URL for fetching
	url := entry.URL
	if idx := strings.Index(url, "#"); idx >= 0 {
		url = url[:idx]
	}

	resp, err := httpGet(url)
	if err != nil {
		return nil, fmt.Errorf("fetch table %s: %w", entry.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("table %s returned status %d", entry.Name, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body for %s: %w", entry.Name, err)
	}

	return parseTablePage(string(body), entry, domain)
}

// parseTablePage extracts table schema from the individual table HTML page.
func parseTablePage(htmlContent string, entry TOCEntry, domain model.Domain) (*model.Table, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("parse table HTML: %w", err)
	}

	t := &model.Table{
		Name:   entry.Name,
		Domain: domain.Code,
		Module: entry.Module,
		Type:   entry.Type,
		Schema: "FUSION",
		DocURL: entry.URL,
	}

	// Extract description - usually the first paragraph after the title
	t.Description = extractDescription(doc)

	// Extract columns from the main table. Base-table pages use a multi-column
	// table (Name, Datatype, ...); view pages publish a single-column "Name"
	// list, which extractColumns does not match — fall back to extractViewColumns.
	cols := extractColumns(doc)
	if len(cols) == 0 {
		cols = extractViewColumns(doc)
	}
	t.Columns = cols

	// Extract primary key
	t.PrimaryKey = extractPrimaryKey(doc)

	// Extract indexes
	t.Indexes = extractIndexes(doc)

	// Extract foreign keys
	t.ForeignKeys = extractForeignKeys(doc)

	return t, nil
}

// extractDescription gets the table description from the page.
func extractDescription(doc *html.Node) string {
	// Look for <p> tags after the first heading that contain the description
	var desc string
	var foundTitle bool
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if desc != "" {
			return
		}
		if n.Type == html.ElementNode {
			if isHeading(n) {
				foundTitle = true
				return
			}
			if foundTitle && n.Data == "p" {
				text := getTextContent(n)
				text = strings.TrimSpace(text)
				if text != "" && len(text) > 10 && !strings.HasPrefix(text, "Details") {
					desc = text
					return
				}
			}
			// Also check for description in div/section elements
			if foundTitle && (n.Data == "div" || n.Data == "section") {
				cls := getAttr(n, "class")
				if strings.Contains(cls, "description") || strings.Contains(cls, "desc") {
					text := getTextContent(n)
					if text = strings.TrimSpace(text); text != "" {
						desc = text
						return
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return desc
}

// extractColumns parses the column table from the HTML.
func extractColumns(doc *html.Node) []model.Column {
	tables := findHTMLTables(doc)

	for _, tbl := range tables {
		if len(tbl) < 2 { // Need header + at least one row
			continue
		}

		header := tbl[0]
		// Look for the columns table by checking header cells.
		// Oracle docs use headers: "Name", "Datatype", "Length", "Precision", "Not-null", "Comments", "Flexfield-mapping"
		// Some pages use: "Column Name", "Data Type", etc.
		headerStr := strings.ToUpper(strings.Join(header, " "))
		isColumnTable := (strings.Contains(headerStr, "COLUMN") && strings.Contains(headerStr, "DATA TYPE")) ||
			(strings.Contains(headerStr, "NAME") && strings.Contains(headerStr, "DATATYPE")) ||
			(strings.Contains(headerStr, "NAME") && strings.Contains(headerStr, "DATA TYPE")) ||
			(strings.Contains(headerStr, "NAME") && strings.Contains(headerStr, "LENGTH") && strings.Contains(headerStr, "NOT"))

		if isColumnTable {
			var columns []model.Column
			for i, row := range tbl[1:] {
				if len(row) < 2 {
					continue
				}
				col := model.Column{
					Position: i + 1,
				}

				// Map columns based on header position
				// Oracle uses "Name" for column name, "Datatype" for type, "Comments" for description
				for j, h := range header {
					if j >= len(row) {
						break
					}
					val := strings.TrimSpace(row[j])
					hu := strings.ToUpper(strings.TrimSpace(h))
					switch {
					case hu == "NAME" || hu == "COLUMN NAME" || hu == "COLUMN":
						col.Name = val
					case hu == "DATATYPE" || hu == "DATA TYPE" || hu == "TYPE":
						col.DataType = val
					case hu == "LENGTH":
						col.Length = val
					case hu == "PRECISION" || hu == "SCALE" || hu == "PRECISION/SCALE":
						col.Precision = val
					case hu == "NOT-NULL" || hu == "NOT NULL" || hu == "NULLABLE" || hu == "NULL":
						col.Nullable = !strings.EqualFold(val, "yes")
					case hu == "COMMENTS" || hu == "DESCRIPTION" || hu == "COMMENT":
						col.Description = val
					}
				}

				if col.Name != "" {
					columns = append(columns, col)
				}
			}
			return columns
		}
	}

	return nil
}

// extractPrimaryKey finds the primary key section.
//
// Oracle docs publish the PK in a two-column table under a "Primary Key" heading:
//
//	| Name                  | Columns             |
//	| IBY_PAYMENT_METHODS_B_PK | PAYMENT_METHOD_CODE |
//
// We detect that table by its header shape (exactly two columns: Name + Columns,
// and crucially NOT a Datatype column, which distinguishes it from the main
// columns table) plus a data row whose Name cell ends in "_PK" (every Fusion PK
// is named <TABLE>_PK). The Columns cell holds one or more PK column names.
func extractPrimaryKey(doc *html.Node) *model.PK {
	tables := findHTMLTables(doc)

	for _, tbl := range tables {
		if len(tbl) < 2 { // need header + at least one data row
			continue
		}
		header := tbl[0]
		if len(header) != 2 {
			continue
		}
		headerStr := strings.ToUpper(strings.Join(header, " "))
		isPKTable := strings.Contains(headerStr, "NAME") &&
			strings.Contains(headerStr, "COLUMN") &&
			!strings.Contains(headerStr, "DATATYPE") &&
			!strings.Contains(headerStr, "DATA TYPE") &&
			!strings.Contains(headerStr, "INDEX") &&
			!strings.Contains(headerStr, "UNIQUE")
		if !isPKTable {
			continue
		}

		for _, row := range tbl[1:] {
			if len(row) < 2 {
				continue
			}
			name := strings.TrimSpace(row[0])
			if !strings.HasSuffix(strings.ToUpper(name), "_PK") {
				continue
			}
			pk := &model.PK{Name: name}
			for _, col := range splitPKColumns(row[1]) {
				pk.Columns = append(pk.Columns, col)
			}
			if len(pk.Columns) > 0 {
				return pk
			}
		}
	}

	return nil
}

// splitPKColumns splits a primary-key Columns cell into individual column names.
// Handles single, comma-separated, and whitespace-separated (composite) keys.
func splitPKColumns(s string) []string {
	s = strings.ReplaceAll(s, ",", " ")
	return strings.Fields(s)
}

// extractViewColumns parses the column list from a view documentation page.
//
// Unlike base-table pages (which list Name + Datatype + ... per row),
// Oracle's _VL / _V view pages publish a single-column "Name" table whose one
// data cell contains every projected column name, whitespace-separated. The
// published list includes view-specific derived columns and omits the
// LANGUAGE / SOURCE_LANG columns that the view filters out. Types are not
// listed on the view page (they are enriched later from the _B / _TL tables).
func extractViewColumns(doc *html.Node) []model.Column {
	tables := findHTMLTables(doc)

	for _, tbl := range tables {
		if len(tbl) < 2 {
			continue
		}
		header := tbl[0]
		if len(header) != 1 || !strings.EqualFold(strings.TrimSpace(header[0]), "Name") {
			continue
		}

		var columns []model.Column
		pos := 0
		for _, row := range tbl[1:] {
			if len(row) == 0 {
				continue
			}
			for _, name := range strings.Fields(row[0]) {
				name = strings.TrimSpace(name)
				if name == "" {
					continue
				}
				pos++
				columns = append(columns, model.Column{Name: name, Position: pos})
			}
		}
		if len(columns) > 0 {
			return columns
		}
	}

	return nil
}

// extractIndexes finds index definitions in the page.
func extractIndexes(doc *html.Node) []model.Index {
	tables := findHTMLTables(doc)
	var indexes []model.Index

	for _, tbl := range tables {
		if len(tbl) < 2 {
			continue
		}
		header := tbl[0]
		headerStr := strings.ToUpper(strings.Join(header, " "))
		if strings.Contains(headerStr, "INDEX") && (strings.Contains(headerStr, "UNIQUE") || strings.Contains(headerStr, "COLUMN")) {
			for _, row := range tbl[1:] {
				if len(row) < 2 {
					continue
				}
				idx := model.Index{}
				for j, h := range header {
					if j >= len(row) {
						break
					}
					val := strings.TrimSpace(row[j])
					switch {
					case matchHeader(h, "index", "name"):
						idx.Name = val
					case matchHeader(h, "unique"):
						idx.Unique = strings.EqualFold(val, "unique") || strings.EqualFold(val, "yes")
					case matchHeader(h, "tablespace"):
						idx.Tablespace = val
					case matchHeader(h, "column"):
						// Columns may be comma-separated or space-separated
						cols := strings.Split(val, ",")
						for _, c := range cols {
							c = strings.TrimSpace(c)
							if c != "" {
								idx.Columns = append(idx.Columns, c)
							}
						}
					}
				}
				if idx.Name != "" {
					indexes = append(indexes, idx)
				}
			}
		}
	}

	return indexes
}

// extractForeignKeys finds FK relationships.
func extractForeignKeys(doc *html.Node) []model.FK {
	tables := findHTMLTables(doc)
	var fks []model.FK

	for _, tbl := range tables {
		if len(tbl) < 2 {
			continue
		}
		header := tbl[0]
		headerStr := strings.ToUpper(strings.Join(header, " "))
		if strings.Contains(headerStr, "FOREIGN") || strings.Contains(headerStr, "REFERENC") {
			for _, row := range tbl[1:] {
				if len(row) < 2 {
					continue
				}
				fk := model.FK{}
				for j, h := range header {
					if j >= len(row) {
						break
					}
					val := strings.TrimSpace(row[j])
					switch {
					case matchHeader(h, "referenc", "table"):
						fk.ReferencingTable = val
					case matchHeader(h, "column", "fk"):
						fk.FKColumn = val
					case matchHeader(h, "relation"):
						fk.Relationship = val
					}
				}
				if fk.ReferencingTable != "" {
					fks = append(fks, fk)
				}
			}
		}
	}

	return fks
}

// ScrapeTablesConcurrently scrapes multiple table pages in parallel.
func ScrapeTablesConcurrently(entries []TOCEntry, domain model.Domain, workers int, progress ProgressFunc) ([]*model.Table, []error) {
	if workers <= 0 {
		workers = 5
	}

	type result struct {
		table *model.Table
		err   error
		idx   int
	}

	jobs := make(chan int, len(entries))
	results := make(chan result, len(entries))

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				entry := entries[idx]
				tbl, err := FetchTableDetail(entry, domain)
				results <- result{table: tbl, err: err, idx: idx}
				// Be polite to Oracle's servers
				time.Sleep(200 * time.Millisecond)
			}
		}()
	}

	for i := range entries {
		jobs <- i
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	tables := make([]*model.Table, len(entries))
	var errs []error
	done := 0
	for r := range results {
		done++
		if r.err != nil {
			errs = append(errs, fmt.Errorf("%s: %v", entries[r.idx].Name, r.err))
		} else {
			tables[r.idx] = r.table
		}
		if progress != nil {
			name := entries[r.idx].Name
			progress(domain.Code, done, len(entries), name)
		}
	}

	// Filter nil entries
	var valid []*model.Table
	for _, t := range tables {
		if t != nil {
			valid = append(valid, t)
		}
	}

	return valid, errs
}

// --- HTML utility functions ---

func httpGet(url string) (*http.Response, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "oracle-fusion-schema/1.0 (CLI documentation indexer)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	return client.Do(req)
}

func isHeading(n *html.Node) bool {
	return n.Data == "h1" || n.Data == "h2" || n.Data == "h3" || n.Data == "h4" || n.Data == "h5"
}

func getTextContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(getTextContent(c))
	}
	return sb.String()
}

func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// isTableLink checks if a link href looks like a table documentation page.
// Pattern: {tablename}-{numericid}.html
func isTableLink(href string) bool {
	href = strings.ToLower(href)
	// Remove anchor
	if idx := strings.Index(href, "#"); idx >= 0 {
		href = href[:idx]
	}
	if !strings.HasSuffix(href, ".html") {
		return false
	}
	// Must contain a hyphen followed by digits before .html
	base := strings.TrimSuffix(href, ".html")
	lastHyphen := strings.LastIndex(base, "-")
	if lastHyphen < 0 {
		return false
	}
	numPart := base[lastHyphen+1:]
	for _, c := range numPart {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(numPart) > 0
}

func resolveURL(baseURL, href string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	// Relative URL
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return baseURL + href
}

// findHTMLTables extracts all HTML tables as [][]string (rows of cells).
func findHTMLTables(doc *html.Node) [][][]string {
	var tables [][][]string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			tbl := parseHTMLTable(n)
			if len(tbl) > 0 {
				tables = append(tables, tbl)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return tables
}

func parseHTMLTable(tableNode *html.Node) [][]string {
	var rows [][]string
	var walkRows func(*html.Node)
	walkRows = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			var cells []string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
					cells = append(cells, strings.TrimSpace(getTextContent(c)))
				}
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkRows(c)
		}
	}
	walkRows(tableNode)
	return rows
}

func matchHeader(header string, keywords ...string) bool {
	h := strings.ToUpper(strings.TrimSpace(header))
	for _, kw := range keywords {
		if strings.Contains(h, strings.ToUpper(kw)) {
			return true
		}
	}
	return false
}

// stripSectionNumber removes leading section numbers from headings.
// e.g., "2 Purchasing" -> "Purchasing", "3 Self Service Procurement" -> "Self Service Procurement"
func stripSectionNumber(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return s
	}
	// Strip leading digits and spaces
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9') {
		i++
	}
	if i > 0 && i < len(s) {
		return strings.TrimSpace(s[i:])
	}
	return s
}
