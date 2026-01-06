package update

import (
	"biobtree/pbuf"
	"database/sql"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

type msigdb struct {
	source string
	d      *DataUpdate
}

// check provides context-aware error checking for msigdb processor
func (m *msigdb) check(err error, operation string) {
	checkWithContext(err, m.source, operation)
}

func (m *msigdb) update() {
	defer m.d.wg.Done()

	log.Printf("[%s] Starting MSigDB gene set integration...", m.source)
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(m.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, m.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("[%s] [TEST MODE] Processing up to %d gene sets", m.source, testLimit)
	}

	// Get database path
	dbPath := config.Dataconf[m.source]["path"]
	if dbPath == "" {
		log.Fatalf("[%s] Database path not configured", m.source)
	}

	log.Printf("[%s] Opening SQLite database: %s", m.source, dbPath)

	// Open SQLite database
	db, err := sql.Open("sqlite", dbPath)
	m.check(err, "opening SQLite database")
	defer db.Close()

	// Verify connection
	err = db.Ping()
	m.check(err, "pinging SQLite database")

	// Get total count for progress reporting
	var totalCount int
	err = db.QueryRow("SELECT COUNT(*) FROM gene_set").Scan(&totalCount)
	m.check(err, "counting gene sets")
	log.Printf("[%s] Found %d gene sets to process", m.source, totalCount)

	// Process gene sets
	m.processGeneSets(db, testLimit, idLogFile)

	log.Printf("[%s] Data processing complete (total: %.2fs)", m.source, time.Since(startTime).Seconds())
	m.d.progChan <- &progressInfo{dataset: m.source, done: true}
}

func (m *msigdb) processGeneSets(db *sql.DB, testLimit int, idLogFile *os.File) {
	fr := config.Dataconf[m.source]["id"]

	// Query to get gene set details with publication info
	query := `
		SELECT
			gs.id,
			gs.standard_name,
			gs.collection_name,
			gs.license_code,
			COALESCE(gsd.systematic_name, '') as systematic_name,
			COALESCE(gsd.description_brief, '') as description,
			COALESCE(gsd.exact_source, '') as exact_source,
			COALESCE(gsd.external_details_URL, '') as external_url,
			COALESCE(gsd.source_species_code, '') as source_species,
			COALESCE(gsd.contributor, '') as contributor,
			COALESCE(gsd.contrib_organization, '') as contributor_org,
			COALESCE(p.PMID, '') as pmid,
			COALESCE(p.DOI, '') as doi
		FROM gene_set gs
		LEFT JOIN gene_set_details gsd ON gs.id = gsd.gene_set_id
		LEFT JOIN publication p ON gsd.publication_id = p.id
		ORDER BY gs.id
	`

	rows, err := db.Query(query)
	m.check(err, "querying gene sets")
	defer rows.Close()

	processedCount := uint64(0)
	skippedCount := uint64(0)

	for rows.Next() {
		var gsID int
		var standardName, collectionName, licenseCode string
		var systematicName, description, exactSource, externalURL, sourceSpecies string
		var contributor, contributorOrg, pmid, doi string

		err := rows.Scan(
			&gsID, &standardName, &collectionName, &licenseCode,
			&systematicName, &description, &exactSource, &externalURL, &sourceSpecies,
			&contributor, &contributorOrg, &pmid, &doi,
		)
		if err != nil {
			log.Printf("[%s] Warning: error scanning row: %v", m.source, err)
			skippedCount++
			continue
		}

		// Skip entries without systematic_name (the primary ID)
		if systematicName == "" {
			skippedCount++
			continue
		}

		// Get gene symbols for this gene set
		geneSymbols := m.getGeneSymbols(db, gsID)

		// Get Entrez Gene IDs for cross-referencing
		entrezIDs := m.getEntrezIDs(db, gsID)

		// Get external terms (GO, HPO)
		goTerms, hpoTerms := m.getExternalTerms(db, gsID)

		// Parse collection into main collection and sub-collection
		collection, subCollection := parseCollection(collectionName)

		// Build protobuf attribute
		attr := &pbuf.MsigdbAttr{
			SystematicName: systematicName,
			StandardName:   standardName,
			Collection:     collection,
			SubCollection:  subCollection,
			Description:    description,
			ExactSource:    exactSource,
			GeneSymbols:    geneSymbols,
			GeneCount:      int32(len(geneSymbols)),
			Pmid:           pmid,
			Doi:            doi,
			ExternalUrl:    externalURL,
			GoTerms:        goTerms,
			HpoTerms:       hpoTerms,
			SourceSpecies:  sourceSpecies,
			Contributor:    contributor,
			ContributorOrg: contributorOrg,
		}

		// Marshal and save
		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("[%s] Warning: error marshaling attributes for %s: %v", m.source, systematicName, err)
			skippedCount++
			continue
		}
		m.d.addProp3(systematicName, fr, attrBytes)

		// Create cross-references
		m.createReferences(systematicName, standardName, collectionName, geneSymbols, entrezIDs, pmid, goTerms, hpoTerms)

		// Log ID in test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, systematicName)
		}

		processedCount++

		// Progress logging every 5000 entries
		if processedCount%5000 == 0 {
			log.Printf("[%s] Processed %d gene sets...", m.source, processedCount)
		}

		// Test mode limit
		if testLimit > 0 && int(processedCount) >= testLimit {
			log.Printf("[%s] [TEST MODE] Reached limit of %d gene sets", m.source, testLimit)
			break
		}
	}

	err = rows.Err()
	m.check(err, "iterating gene set rows")

	log.Printf("[%s] Successfully processed %d gene sets (skipped %d)", m.source, processedCount, skippedCount)
	atomic.AddUint64(&m.d.totalParsedEntry, processedCount)
}

func (m *msigdb) getGeneSymbols(db *sql.DB, geneSetID int) []string {
	query := `
		SELECT gsym.symbol
		FROM gene_set_gene_symbol gsgs
		JOIN gene_symbol gsym ON gsgs.gene_symbol_id = gsym.id
		WHERE gsgs.gene_set_id = ?
		ORDER BY gsym.symbol
	`

	rows, err := db.Query(query, geneSetID)
	if err != nil {
		log.Printf("[%s] Warning: error querying gene symbols for gene set %d: %v", m.source, geneSetID, err)
		return nil
	}
	defer rows.Close()

	var symbols []string
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err == nil && symbol != "" {
			symbols = append(symbols, symbol)
		}
	}

	return symbols
}

func (m *msigdb) getExternalTerms(db *sql.DB, geneSetID int) (goTerms, hpoTerms []string) {
	query := `
		SELECT et.term
		FROM external_term_filtered_by_similarity etf
		JOIN external_term et ON etf.term = et.term
		WHERE etf.gene_set_id = ?
	`

	rows, err := db.Query(query, geneSetID)
	if err != nil {
		// This table might not always exist or have data
		return nil, nil
	}
	defer rows.Close()

	for rows.Next() {
		var term string
		if err := rows.Scan(&term); err == nil && term != "" {
			if strings.HasPrefix(term, "GO:") {
				goTerms = append(goTerms, term)
			} else if strings.HasPrefix(term, "HP:") {
				hpoTerms = append(hpoTerms, term)
			}
		}
	}

	return goTerms, hpoTerms
}

func (m *msigdb) createReferences(systematicName, standardName, collectionName string,
	geneSymbols, entrezIDs []string, pmid string, goTerms, hpoTerms []string) {

	// 1. Text search: systematic name (M5890) → MSigDB
	m.d.addXref(systematicName, textLinkID, systematicName, m.source, true)

	// 2. Text search: standard name (HALLMARK_APOPTOSIS) → MSigDB
	if standardName != "" {
		m.d.addXref(standardName, textLinkID, systematicName, m.source, true)
	}

	// 3. Cross-reference to HGNC gene symbols
	// This enables: BRCA1 >> msigdb to find gene sets containing BRCA1
	if hgncID, ok := config.Dataconf["hgnc"]; ok {
		for _, symbol := range geneSymbols {
			// Gene symbol → MSigDB gene set
			m.d.addXref(symbol, hgncID["id"], systematicName, m.source, false)
		}
	}

	// 4. Cross-reference to Entrez Gene IDs
	// This enables: 672 >>entrez>>msigdb to find gene sets containing BRCA1 (Entrez:672)
	if entrezID, ok := config.Dataconf["entrez"]; ok {
		for _, ncbiID := range entrezIDs {
			m.d.addXref(ncbiID, entrezID["id"], systematicName, m.source, false)
		}
	}

	// 5. Cross-reference to PubMed
	if pmid != "" && pmid != "0" {
		if pubmedID, ok := config.Dataconf["pubmed"]; ok {
			m.d.addXref(pmid, pubmedID["id"], systematicName, m.source, false)
		}
	}

	// 6. Cross-reference to GO terms
	if goID, ok := config.Dataconf["go"]; ok {
		for _, goTerm := range goTerms {
			m.d.addXref(goTerm, goID["id"], systematicName, m.source, false)
		}
	}

	// 7. Cross-reference to HPO terms
	if hpoID, ok := config.Dataconf["hpo"]; ok {
		for _, hpoTerm := range hpoTerms {
			m.d.addXref(hpoTerm, hpoID["id"], systematicName, m.source, false)
		}
	}

	// 8. Cross-reference to Reactome (for C2:CP:REACTOME collection)
	// Note: Reactome pathway IDs (R-HSA-*) are not directly available in the SQLite
	// The link between MSigDB and Reactome is established through gene membership
	// Skip direct Reactome xref for now - users can use gene-based queries
}

// parseCollection splits collection name into main and sub collection
// e.g., "C2:CGP" → "C2", "CGP"
// e.g., "C5:GO:BP" → "C5:GO", "BP"
// e.g., "H" → "H", ""
func parseCollection(collectionName string) (string, string) {
	if collectionName == "" {
		return "", ""
	}

	// For simple collections like "H"
	if !strings.Contains(collectionName, ":") {
		return collectionName, ""
	}

	// For hierarchical collections like "C2:CGP" or "C5:GO:BP"
	parts := strings.Split(collectionName, ":")
	if len(parts) == 2 {
		return parts[0], parts[1]
	} else if len(parts) >= 3 {
		// For "C5:GO:BP" return "C5:GO" and "BP"
		return parts[0] + ":" + parts[1], parts[len(parts)-1]
	}

	return collectionName, ""
}

// Helper to get Entrez gene IDs for cross-referencing
func (m *msigdb) getEntrezIDs(db *sql.DB, geneSetID int) []string {
	query := `
		SELECT DISTINCT gsym.NCBI_id
		FROM gene_set_gene_symbol gsgs
		JOIN gene_symbol gsym ON gsgs.gene_symbol_id = gsym.id
		WHERE gsgs.gene_set_id = ? AND gsym.NCBI_id IS NOT NULL AND gsym.NCBI_id != ''
	`

	rows, err := db.Query(query, geneSetID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var ncbiID string
		if err := rows.Scan(&ncbiID); err == nil && ncbiID != "" {
			// NCBI_id is stored as integer string
			if _, err := strconv.Atoi(ncbiID); err == nil {
				ids = append(ids, ncbiID)
			}
		}
	}

	return ids
}
