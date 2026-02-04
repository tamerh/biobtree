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

	// In test mode, ensure we include at least one GO, HPO, and Hallmark entry
	priorityIDs := []int{}
	if testLimit > 0 {
		priorityIDs = m.getPriorityTestGeneSetIDs(db)
	}

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
	processedIDs := make(map[int]bool)

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

		if m.processGeneSet(db, fr, gsID, standardName, collectionName, licenseCode,
			systematicName, description, exactSource, externalURL, sourceSpecies,
			contributor, contributorOrg, pmid, doi, idLogFile) {
			processedCount++
			processedIDs[gsID] = true
		} else {
			skippedCount++
			continue
		}

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

	// In test mode, ensure priority IDs are included even if beyond the test limit
	if testLimit > 0 && len(priorityIDs) > 0 {
		added := m.processSpecificGeneSets(db, fr, priorityIDs, processedIDs, idLogFile)
		if added > 0 {
			log.Printf("[%s] [TEST MODE] Added %d priority gene sets (GO/HPO/H) beyond test limit", m.source, added)
			processedCount += uint64(added)
		}
	}

	log.Printf("[%s] Successfully processed %d gene sets (skipped %d)", m.source, processedCount, skippedCount)
	atomic.AddUint64(&m.d.totalParsedEntry, processedCount)
}

func (m *msigdb) processGeneSet(db *sql.DB, fr string, gsID int, standardName, collectionName, licenseCode,
	systematicName, description, exactSource, externalURL, sourceSpecies, contributor, contributorOrg, pmid, doi string,
	idLogFile *os.File) bool {

	// Skip entries without systematic_name (the primary ID)
	if systematicName == "" {
		return false
	}

	// Get gene symbols for this gene set
	geneSymbols := m.getGeneSymbols(db, gsID)

	// Get Entrez Gene IDs for cross-referencing
	entrezIDs := m.getEntrezIDs(db, gsID)

	// Get external terms (GO, HPO) from filtered similarity table
	goTerms, hpoTerms := m.getExternalTerms(db, gsID)

	// Also capture GO/HPO terms directly from exact_source when present
	// MSigDB stores authoritative ontology links here for C5 gene sets
	addExternalTermsFromExactSource(exactSource, &goTerms, &hpoTerms)

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
		return false
	}
	m.d.addProp3(systematicName, fr, attrBytes)

	// Create cross-references
	m.createReferences(systematicName, standardName, collectionName, geneSymbols, entrezIDs, pmid, goTerms, hpoTerms)

	// Log ID in test mode
	if idLogFile != nil {
		logProcessedID(idLogFile, systematicName)
	}

	return true
}

func (m *msigdb) processSpecificGeneSets(db *sql.DB, fr string, ids []int, processedIDs map[int]bool, idLogFile *os.File) int {
	if len(ids) == 0 {
		return 0
	}

	placeholders := strings.Repeat("?,", len(ids))
	placeholders = strings.TrimSuffix(placeholders, ",")

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
		WHERE gs.id IN (` + placeholders + `)
	`

	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("[%s] Warning: error querying priority gene sets: %v", m.source, err)
		return 0
	}
	defer rows.Close()

	added := 0
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
			continue
		}

		if processedIDs[gsID] {
			continue
		}

		if m.processGeneSet(db, fr, gsID, standardName, collectionName, licenseCode,
			systematicName, description, exactSource, externalURL, sourceSpecies,
			contributor, contributorOrg, pmid, doi, idLogFile) {
			processedIDs[gsID] = true
			added++
		}
	}

	return added
}

func (m *msigdb) getPriorityTestGeneSetIDs(db *sql.DB) []int {
	ids := make([]int, 0, 6)

	// Hallmark (H) collection
	if id := m.querySingleInt(db, "SELECT MIN(id) FROM gene_set WHERE collection_name='H'"); id > 0 {
		ids = append(ids, id)
	}

	// GO exact_source
	if id := m.querySingleInt(db, "SELECT MIN(gene_set_id) FROM gene_set_details WHERE exact_source LIKE 'GO:%'"); id > 0 {
		ids = append(ids, id)
	}

	// HPO exact_source
	if id := m.querySingleInt(db, "SELECT MIN(gene_set_id) FROM gene_set_details WHERE exact_source LIKE 'HP:%'"); id > 0 {
		ids = append(ids, id)
	}

	return ids
}

func (m *msigdb) querySingleInt(db *sql.DB, query string) int {
	var id sql.NullInt64
	if err := db.QueryRow(query).Scan(&id); err != nil {
		return 0
	}
	if !id.Valid {
		return 0
	}
	return int(id.Int64)
}

// addExternalTermsFromExactSource extracts GO/HPO terms from exact_source
// and appends them to the provided slices if not already present.
func addExternalTermsFromExactSource(exactSource string, goTerms, hpoTerms *[]string) {
	if exactSource == "" {
		return
	}

	parts := splitExternalTermCandidates(exactSource)
	for _, term := range parts {
		if strings.HasPrefix(term, "GO:") {
			appendUniqueTerm(goTerms, term)
		} else if strings.HasPrefix(term, "HP:") {
			appendUniqueTerm(hpoTerms, term)
		}
	}
}

// splitExternalTermCandidates splits a string on common delimiters
// and trims punctuation around terms.
func splitExternalTermCandidates(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	// Normalize common separators to spaces
	replacer := strings.NewReplacer(",", " ", ";", " ", "|", " ")
	s = replacer.Replace(s)

	fields := strings.Fields(s)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		t := strings.Trim(f, " \t\r\n\"'()[]{}")
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// appendUnique appends term to list if not already present.
func appendUniqueTerm(list *[]string, term string) {
	for _, existing := range *list {
		if existing == term {
			return
		}
	}
	*list = append(*list, term)
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
	// This enables: BRCA1 >> hgnc >> msigdb to find gene sets containing BRCA1
	// Requires lookup database to resolve gene symbols to proper HGNC IDs (e.g., BRCA1 → HGNC:1100)
	// Without lookup, HGNC xrefs are skipped to avoid creating spurious entries
	if _, ok := config.Dataconf["hgnc"]; ok && m.d.hasLookupDB {
		for _, symbol := range geneSymbols {
			m.d.addXrefViaKeyword(symbol, "hgnc", systematicName, m.source, config.Dataconf[m.source]["id"], false)
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

	// 6. Cross-reference to GO terms (bidirectional)
	// Forward: GO → MSigDB (enables >>go>>msigdb queries)
	// Reverse: MSigDB → GO (enables >>msigdb>>go queries)
	if goID, ok := config.Dataconf["go"]; ok {
		msigdbID := config.Dataconf[m.source]["id"]
		for _, goTerm := range goTerms {
			// Forward: GO term → MSigDB gene set
			m.d.addXref(goTerm, goID["id"], systematicName, m.source, false)
			// Reverse: MSigDB gene set → GO term
			m.d.addXref(systematicName, msigdbID, goTerm, "go", false)
		}
	}

	// 7. Cross-reference to HPO terms (bidirectional)
	// Forward: HPO → MSigDB (enables >>hpo>>msigdb queries)
	// Reverse: MSigDB → HPO (enables >>msigdb>>hpo queries)
	if hpoID, ok := config.Dataconf["hpo"]; ok {
		msigdbID := config.Dataconf[m.source]["id"]
		for _, hpoTerm := range hpoTerms {
			// Forward: HPO term → MSigDB gene set
			m.d.addXref(hpoTerm, hpoID["id"], systematicName, m.source, false)
			// Reverse: MSigDB gene set → HPO term
			m.d.addXref(systematicName, msigdbID, hpoTerm, "hpo", false)
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
