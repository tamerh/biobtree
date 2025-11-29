package update

import (
	"biobtree/pbuf"
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type antibody struct {
	source string
	d      *DataUpdate
	idMap  map[string]string // Track IDs and their sources for conflict detection
}

// Helper for context-aware error checking
func (a *antibody) check(err error, operation string) {
	checkWithContext(err, a.source, operation)
}

// Main update entry point - processes all antibody sources
func (a *antibody) update() {
	defer a.d.wg.Done()

	log.Println("Antibody: Starting unified antibody data processing...")
	startTime := time.Now()

	// Initialize ID conflict detection map
	a.idMap = make(map[string]string)

	// Test mode support
	testLimit := config.GetTestLimit(a.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, a.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("Antibody: [TEST MODE] Processing up to %d entries per source", testLimit)
	}

	// Process all antibody sources
	a.parseTheraSAbDab(testLimit, idLogFile)
	a.parseSAbDab(testLimit, idLogFile)
	a.parseIMGTGene(testLimit, idLogFile)
	a.parseIMGTLigm(testLimit, idLogFile)

	log.Printf("Antibody: Processing complete (%.2fs)", time.Since(startTime).Seconds())
}

// parseTheraSAbDab processes the TheraSAbDab CSV file (therapeutic antibodies)
func (a *antibody) parseTheraSAbDab(testLimit int, idLogFile *os.File) {
	// Build file path from therasabdab_path config
	filePath := config.Dataconf[a.source]["therasabdab_path"]
	if filePath == "" {
		log.Println("Antibody: TheraSAbDab path not configured, skipping...")
		return
	}
	log.Printf("Antibody (TheraSAbDab): Downloading from %s", filePath)

	// Open CSV file
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(a.source, a.d.ebiFtp, a.d.ebiFtpPath, filePath)
	a.check(err, "opening TheraSAbDab CSV file")
	defer closeReaders(gz, ftpFile, client, localFile)

	sourceID := config.Dataconf[a.source]["id"]
	textLinkID := "0" // Text search link ID

	// Create CSV reader
	csvReader := csv.NewReader(br)
	csvReader.Comma = ','
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1 // Allow variable number of fields

	log.Println("Antibody (TheraSAbDab): Reading CSV file...")

	// Read header
	header, err := csvReader.Read()
	a.check(err, "reading TheraSAbDab header")

	// Strip UTF-8 BOM from first column if present
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\ufeff")
	}

	// Map column names to indices
	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(name)] = i
	}

	log.Printf("Antibody (TheraSAbDab): Found %d columns in header", len(colMap))

	// Save each therapeutic antibody entry
	var savedAntibodies int
	var skippedNoINN int
	var skippedDuplicates int
	var totalRowsRead int

	for {
		row, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Antibody (TheraSAbDab): Warning - error reading row %d: %v", totalRowsRead+2, err)
			continue
		}

		totalRowsRead++

		// Test mode limit check
		if config.IsTestMode() && savedAntibodies >= testLimit {
			log.Printf("Antibody (TheraSAbDab): [TEST MODE] Reached limit of %d entries", testLimit)
			break
		}

		// Progress logging
		if totalRowsRead%100 == 0 {
			log.Printf("Antibody (TheraSAbDab): Processed %d rows, saved %d antibodies...", totalRowsRead, savedAntibodies)
		}

		// Extract INN name (primary key) - column is "Therapeutic"
		innName := strings.TrimSpace(getColumnValue(row, colMap, "Therapeutic"))
		if innName == "" {
			skippedNoINN++
			continue
		}

		// ID conflict detection - silently skip duplicates
		if _, exists := a.idMap[innName]; exists {
			skippedDuplicates++
			continue
		}
		a.idMap[innName] = "therasabdab"

		// Extract all fields with correct column names
		format := strings.TrimSpace(getColumnValue(row, colMap, "Format"))
		isotype := strings.TrimSpace(getColumnValue(row, colMap, "CH1 Isotype"))
		lightChain := strings.TrimSpace(getColumnValue(row, colMap, "VD LC"))
		clinicalStage := strings.TrimSpace(getColumnValue(row, colMap, "Highest_Clin_Trial"))
		status := strings.TrimSpace(getColumnValue(row, colMap, "Est. Status"))

		// Sequences - single sequences per antibody
		heavyChainSeq := extractSequences(getColumnValue(row, colMap, "HeavySequence"))
		lightChainSeq := extractSequences(getColumnValue(row, colMap, "LightSequence"))

		// Targets and indications
		targets := extractList(getColumnValue(row, colMap, "Target"))
		// Combine different condition columns for indications
		conditionsApproved := getColumnValue(row, colMap, "Conditions Approved")
		conditionsActive := getColumnValue(row, colMap, "Conditions Active")
		conditionsDiscontinued := getColumnValue(row, colMap, "Conditions Discontinued")
		allConditions := strings.Join([]string{conditionsApproved, conditionsActive, conditionsDiscontinued}, ";")
		indications := extractList(allConditions)

		// PDB IDs - may not be in this CSV, extract from SAbDab column if available
		pdbIDs := extractList(getColumnValue(row, colMap, "SAbDab"))

		// Create protobuf entry with unified schema
		entry := &pbuf.AntibodyAttr{
			Source:         "therasabdab",
			AntibodyType:   "therapeutic",
			InnName:        innName,
			Format:         format,
			Isotype:        isotype,
			LightChain:     lightChain,
			ClinicalStage:  clinicalStage,
			Status:         status,
			HeavyChainSeq:  heavyChainSeq,
			LightChainSeq:  lightChainSeq,
			Targets:        targets,
			Indications:    indications,
			PdbIds:         pdbIDs,
		}

		// Marshal to JSON
		marshaled, err := ffjson.Marshal(entry)
		a.check(err, fmt.Sprintf("marshaling antibody %s", innName))

		// Save primary entry
		a.d.addProp3(innName, sourceID, marshaled)

		// Add text search for INN name
		a.d.addXref(innName, textLinkID, innName, a.source, true)

		// Create cross-references to target genes via gene symbol lookup
		// This looks up each gene symbol in the database and creates xrefs to Ensembl gene entries
		for _, target := range targets {
			if target != "" {
				// Look up gene symbol to find Ensembl gene entries
				a.d.addXrefViaKeyword(target, "ensembl", innName, a.source, sourceID, false)
			}
		}

		// Create cross-references to PDB structures
		for _, pdbID := range pdbIDs {
			if pdbID != "" {
				a.d.addXref(innName, config.Dataconf[a.source]["id"], pdbID, "pdb", false)
				// Bidirectional
				a.d.addXref(pdbID, config.Dataconf["pdb"]["id"], innName, a.source, false)
			}
		}

		// Log ID if in test mode
		if idLogFile != nil {
			fmt.Fprintln(idLogFile, innName)
		}

		savedAntibodies++
	}

	log.Printf("Antibody (TheraSAbDab): Summary - Total rows: %d, Saved: %d, Skipped (no INN): %d, Skipped (duplicates): %d",
		totalRowsRead, savedAntibodies, skippedNoINN, skippedDuplicates)
}

// parseSAbDab processes the SAbDab TSV file (antibody structures)
func (a *antibody) parseSAbDab(testLimit int, idLogFile *os.File) {
	// Build file path from sabdab_path config
	filePath := config.Dataconf[a.source]["sabdab_path"]
	if filePath == "" {
		log.Println("Antibody: SAbDab path not configured, skipping...")
		return
	}
	log.Printf("Antibody (SAbDab): Downloading from %s", filePath)

	// Open TSV file
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(a.source, a.d.ebiFtp, a.d.ebiFtpPath, filePath)
	a.check(err, "opening SAbDab TSV file")
	defer closeReaders(gz, ftpFile, client, localFile)

	sourceID := config.Dataconf[a.source]["id"]
	textLinkID := "0" // Text search link ID

	// Create TSV reader (tab-delimited)
	tsvReader := csv.NewReader(br)
	tsvReader.Comma = '\t'
	tsvReader.LazyQuotes = true
	tsvReader.FieldsPerRecord = -1 // Allow variable number of fields

	log.Println("Antibody (SAbDab): Reading TSV file...")

	// Read header
	header, err := tsvReader.Read()
	a.check(err, "reading SAbDab header")

	// Map column names to indices
	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(name)] = i
	}

	log.Printf("Antibody (SAbDab): Found %d columns in header", len(colMap))

	// Save each antibody structure entry
	var savedStructures int
	var skippedNoPDB int
	var skippedDuplicates int
	var totalRowsRead int

	for {
		row, err := tsvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Antibody (SAbDab): Warning - error reading row %d: %v", totalRowsRead+2, err)
			continue
		}

		totalRowsRead++

		// Test mode limit check
		if config.IsTestMode() && savedStructures >= testLimit {
			log.Printf("Antibody (SAbDab): [TEST MODE] Reached limit of %d entries", testLimit)
			break
		}

		// Progress logging
		if totalRowsRead%1000 == 0 {
			log.Printf("Antibody (SAbDab): Processed %d rows, saved %d structures...", totalRowsRead, savedStructures)
		}

		// Extract PDB ID (primary key) + chains to make unique ID
		pdbID := strings.TrimSpace(getColumnValue(row, colMap, "pdb"))
		hChain := strings.TrimSpace(getColumnValue(row, colMap, "Hchain"))
		lChain := strings.TrimSpace(getColumnValue(row, colMap, "Lchain"))

		if pdbID == "" {
			skippedNoPDB++
			continue
		}

		// Create composite ID: pdb_Hchain_Lchain
		compositeID := fmt.Sprintf("%s_%s_%s", pdbID, hChain, lChain)

		// ID conflict detection - silently skip duplicates
		if _, exists := a.idMap[compositeID]; exists {
			skippedDuplicates++
			continue
		}
		a.idMap[compositeID] = "sabdab"

		// Extract all fields
		resolution := strings.TrimSpace(getColumnValue(row, colMap, "resolution"))
		method := strings.TrimSpace(getColumnValue(row, colMap, "method"))
		organism := strings.TrimSpace(getColumnValue(row, colMap, "organism"))
		heavySubclass := strings.TrimSpace(getColumnValue(row, colMap, "heavy_subclass"))
		lightCtype := strings.TrimSpace(getColumnValue(row, colMap, "light_ctype"))
		antigenName := strings.TrimSpace(getColumnValue(row, colMap, "antigen_name"))

		// Create protobuf entry with unified schema
		entry := &pbuf.AntibodyAttr{
			Source:       "sabdab",
			AntibodyType: "structure",
			PdbId:        compositeID, // Use composite ID as primary
			Resolution:   resolution,
			Method:       method,
			Organism:     organism,
			Format:       heavySubclass, // Use heavy_subclass for format
			LightChain:   lightCtype,    // Kappa or Lambda
			Targets:      []string{antigenName},
			PdbIds:       []string{pdbID}, // Original PDB ID for cross-reference
		}

		// Marshal to JSON
		marshaled, err := ffjson.Marshal(entry)
		a.check(err, fmt.Sprintf("marshaling antibody structure %s", compositeID))

		// Save primary entry
		a.d.addProp3(compositeID, sourceID, marshaled)

		// Add text search for PDB ID
		a.d.addXref(compositeID, textLinkID, pdbID, a.source, true)

		// Create bidirectional cross-reference to PDB database
		if pdbID != "" {
			a.d.addXref(compositeID, config.Dataconf[a.source]["id"], pdbID, "pdb", false)
			// Bidirectional
			a.d.addXref(pdbID, config.Dataconf["pdb"]["id"], compositeID, a.source, false)
		}

		// Log ID if in test mode
		if idLogFile != nil {
			fmt.Fprintln(idLogFile, compositeID)
		}

		savedStructures++
	}

	log.Printf("Antibody (SAbDab): Summary - Total rows: %d, Saved: %d, Skipped (no PDB): %d, Skipped (duplicates): %d",
		totalRowsRead, savedStructures, skippedNoPDB, skippedDuplicates)
}

// parseIMGTGene processes the IMGT/GENE-DB FASTA file (germline gene alleles)
func (a *antibody) parseIMGTGene(testLimit int, idLogFile *os.File) {
	// Build file path from imgt_gene_path config
	filePath := config.Dataconf[a.source]["imgt_gene_path"]
	if filePath == "" {
		log.Println("Antibody: IMGT/GENE-DB path not configured, skipping...")
		return
	}
	log.Printf("Antibody (IMGT/GENE-DB): Downloading from %s", filePath)

	// Open FASTA file
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(a.source, a.d.ebiFtp, a.d.ebiFtpPath, filePath)
	a.check(err, "opening IMGT/GENE-DB FASTA file")
	defer closeReaders(gz, ftpFile, client, localFile)

	sourceID := config.Dataconf[a.source]["id"]
	textLinkID := "0" // Text search link ID

	log.Println("Antibody (IMGT/GENE-DB): Reading FASTA file...")

	// Save each germline gene entry
	var savedGenes int
	var skippedNoName int
	var totalEntriesRead int
	var currentHeader string
	var currentSequence strings.Builder

	// Track different skip reasons
	var skippedDuplicates int

	// Process FASTA format line by line
	scanner := bufio.NewScanner(br)
	for scanner.Scan() {
		line := scanner.Text()

		// FASTA header line
		if strings.HasPrefix(line, ">") {
			// Process previous entry if exists
			if currentHeader != "" {
				// Test mode limit check
				if config.IsTestMode() && savedGenes >= testLimit {
					log.Printf("Antibody (IMGT/GENE-DB): [TEST MODE] Reached limit of %d entries", testLimit)
					break
				}

				// Parse and save the gene entry
				result := a.processIMGTGeneEntry(currentHeader, currentSequence.String(), sourceID, textLinkID, idLogFile)
				if result == 1 {
					savedGenes++
				} else if result == 0 {
					skippedNoName++
				} else if result == -1 {
					skippedDuplicates++
				}

				totalEntriesRead++

				// Progress logging
				if totalEntriesRead%1000 == 0 {
					log.Printf("Antibody (IMGT/GENE-DB): Processed %d entries, saved %d genes...", totalEntriesRead, savedGenes)
				}

				// Reset for next entry
				currentSequence.Reset()
			}

			// Store new header (remove '>')
			currentHeader = strings.TrimPrefix(line, ">")
		} else {
			// Sequence line - append to current sequence
			currentSequence.WriteString(strings.TrimSpace(line))
		}
	}

	// Process last entry
	if currentHeader != "" && (savedGenes < testLimit || !config.IsTestMode()) {
		result := a.processIMGTGeneEntry(currentHeader, currentSequence.String(), sourceID, textLinkID, idLogFile)
		if result == 1 {
			savedGenes++
		} else if result == 0 {
			skippedNoName++
		} else if result == -1 {
			skippedDuplicates++
		}
		totalEntriesRead++
	}

	a.check(scanner.Err(), "reading IMGT/GENE-DB FASTA file")

	log.Printf("Antibody (IMGT/GENE-DB): Summary - Total entries: %d, Saved: %d, Skipped (no name): %d, Skipped (duplicates): %d",
		totalEntriesRead, savedGenes, skippedNoName, skippedDuplicates)
}

// processIMGTGeneEntry processes a single IMGT/GENE-DB FASTA entry
// Returns: 1 = saved, 0 = skipped (no name/invalid), -1 = skipped (duplicate)
func (a *antibody) processIMGTGeneEntry(header, sequence, sourceID, textLinkID string, idLogFile *os.File) int {
	// Parse FASTA header - 15 pipe-delimited fields
	// Example: M99641|IGHV1-18*01|Homo sapiens|F|V-REGION|...
	fields := strings.Split(header, "|")
	if len(fields) < 4 {
		log.Printf("Antibody (IMGT/GENE-DB): Warning - malformed header: %s", header)
		return 0
	}

	// Extract key fields
	accession := strings.TrimSpace(fields[0])
	geneName := strings.TrimSpace(fields[1]) // Gene+allele (e.g., "IGHV1-18*01")
	organism := strings.TrimSpace(fields[2])
	functionality := strings.TrimSpace(fields[3])

	// Extract region if available (field 5)
	region := ""
	if len(fields) >= 5 {
		region = strings.TrimSpace(fields[4])
	}

	if geneName == "" {
		return 0
	}

	// Create composite ID: gene+allele_region (e.g., "IGHV1-18*01_V-REGION")
	// This prevents duplicates when the same allele has multiple regions
	compositeID := geneName
	if region != "" && region != "NA" && region != "-" {
		compositeID = geneName + "_" + region
	}

	// ID conflict detection - silently skip duplicates
	if existingSource, exists := a.idMap[compositeID]; exists {
		_ = existingSource // Avoid unused variable warning
		return -1           // Return -1 for duplicate
	}
	a.idMap[compositeID] = "imgt_gene"

	// Extract pure gene name and allele from full name (e.g., "IGHV1-18*01")
	pureGeneName := geneName
	allele := ""
	if idx := strings.LastIndex(geneName, "*"); idx != -1 {
		pureGeneName = geneName[:idx] // e.g., "IGHA" or "IGHV1-18"
		allele = geneName[idx:]        // e.g., "*01"
	}

	// Create protobuf entry with unified schema
	entry := &pbuf.AntibodyAttr{
		Source:        "imgt_gene",
		AntibodyType:  "germline",
		GeneName:      pureGeneName,  // Pure gene name (e.g., "IGHA") for filtering
		Format:        region,         // Store region info (e.g., "V-REGION", "CH1")
		Organism:      organism,
		Functionality: functionality,
		Allele:        allele,         // e.g., "*01"
		HeavyChainSeq: []string{sequence}, // Store the nucleotide sequence
	}

	// Marshal to JSON
	marshaled, err := ffjson.Marshal(entry)
	a.check(err, fmt.Sprintf("marshaling germline gene %s", compositeID))

	// Save primary entry with composite ID
	a.d.addProp3(compositeID, sourceID, marshaled)

	// Add text search for composite ID (e.g., "IGHA*01_V-REGION")
	a.d.addXref(compositeID, textLinkID, compositeID, a.source, true)

	// Add text search for gene+allele (e.g., "IGHA*01") for easier searching
	if compositeID != geneName {
		a.d.addXref(compositeID, textLinkID, geneName, a.source, true)
	}

	// Add text search for pure gene name (e.g., "IGHA") for broad searching
	if pureGeneName != geneName {
		a.d.addXref(compositeID, textLinkID, pureGeneName, a.source, true)
	}

	// Add cross-reference to IMGT/LIGM-DB accession if available
	if accession != "" && accession != "NA" && accession != "-" {
		a.d.addXref(compositeID, config.Dataconf[a.source]["id"], accession, "imgt_ligm", false)
	}

	// Log ID if in test mode
	if idLogFile != nil {
		fmt.Fprintln(idLogFile, compositeID)
	}

	return 1 // Return 1 for success
}

// parseIMGTLigm processes the IMGT/LIGM-DB FASTA file (antibody and TCR sequences)
func (a *antibody) parseIMGTLigm(testLimit int, idLogFile *os.File) {
	// Build file path from imgt_ligm_path config
	filePath := config.Dataconf[a.source]["imgt_ligm_path"]
	if filePath == "" {
		log.Println("Antibody: IMGT/LIGM-DB path not configured, skipping...")
		return
	}
	log.Printf("Antibody (IMGT/LIGM-DB): Downloading from %s", filePath)

	// Open compressed FASTA file
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(a.source, a.d.ebiFtp, a.d.ebiFtpPath, filePath)
	a.check(err, "opening IMGT/LIGM-DB FASTA file")
	defer closeReaders(gz, ftpFile, client, localFile)

	sourceID := config.Dataconf[a.source]["id"]
	textLinkID := "0" // Text search link ID

	log.Println("Antibody (IMGT/LIGM-DB): Reading compressed FASTA file...")

	// Save each antibody sequence entry
	var savedSequences int
	var skippedNoAccession int
	var skippedDuplicates int
	var totalEntriesRead int
	var currentHeader string
	var currentSequence strings.Builder

	// Process FASTA format line by line
	scanner := bufio.NewScanner(br)
	for scanner.Scan() {
		line := scanner.Text()

		// FASTA header line
		if strings.HasPrefix(line, ">") {
			// Process previous entry if exists
			if currentHeader != "" {
				// Test mode limit check
				if config.IsTestMode() && savedSequences >= testLimit {
					log.Printf("Antibody (IMGT/LIGM-DB): [TEST MODE] Reached limit of %d entries", testLimit)
					break
				}

				// Parse and save the sequence entry
				result := a.processIMGTLigmEntry(currentHeader, currentSequence.String(), sourceID, textLinkID, idLogFile)
				if result == 1 {
					savedSequences++
				} else if result == 0 {
					skippedNoAccession++
				} else if result == -1 {
					skippedDuplicates++
				}

				totalEntriesRead++

				// Progress logging
				if totalEntriesRead%5000 == 0 {
					log.Printf("Antibody (IMGT/LIGM-DB): Processed %d entries, saved %d sequences...", totalEntriesRead, savedSequences)
				}

				// Reset for next entry
				currentSequence.Reset()
			}

			// Store new header (remove '>')
			currentHeader = strings.TrimPrefix(line, ">")
		} else {
			// Sequence line - append to current sequence
			currentSequence.WriteString(strings.TrimSpace(line))
		}
	}

	// Process last entry
	if currentHeader != "" && (savedSequences < testLimit || !config.IsTestMode()) {
		result := a.processIMGTLigmEntry(currentHeader, currentSequence.String(), sourceID, textLinkID, idLogFile)
		if result == 1 {
			savedSequences++
		} else if result == 0 {
			skippedNoAccession++
		} else if result == -1 {
			skippedDuplicates++
		}
		totalEntriesRead++
	}

	a.check(scanner.Err(), "reading IMGT/LIGM-DB FASTA file")

	log.Printf("Antibody (IMGT/LIGM-DB): Summary - Total entries: %d, Saved: %d, Skipped (no accession): %d, Skipped (duplicates): %d",
		totalEntriesRead, savedSequences, skippedNoAccession, skippedDuplicates)
}

// processIMGTLigmEntry processes a single IMGT/LIGM-DB FASTA entry
// Returns: 1 = saved, 0 = skipped (no accession/invalid), -1 = skipped (duplicate)
func (a *antibody) processIMGTLigmEntry(header, sequence, sourceID, textLinkID string, idLogFile *os.File) int {
	// Parse FASTA header - simple format: ACCESSION|Description
	// Example: A00673|Artificial sequence for plasmid pSV-V-NP gamma-SNase
	parts := strings.SplitN(header, "|", 2)
	if len(parts) < 1 {
		log.Printf("Antibody (IMGT/LIGM-DB): Warning - malformed header: %s", header)
		return 0
	}

	// Extract accession (primary ID)
	accession := strings.TrimSpace(parts[0])
	if accession == "" {
		return 0
	}

	// Extract description
	description := ""
	if len(parts) == 2 {
		description = strings.TrimSpace(parts[1])
	}

	// ID conflict detection - silently skip duplicates
	if _, exists := a.idMap[accession]; exists {
		return -1 // Return -1 for duplicate
	}
	a.idMap[accession] = "imgt_ligm"

	// Try to extract organism from description (e.g., "H.sapiens", "M.musculus")
	organism := ""
	descLower := strings.ToLower(description)
	if strings.Contains(descLower, "h.sapiens") || strings.Contains(descLower, "homo sapiens") {
		organism = "Homo sapiens"
	} else if strings.Contains(descLower, "m.musculus") || strings.Contains(descLower, "mus musculus") {
		organism = "Mus musculus"
	} else if strings.Contains(descLower, "rattus") {
		organism = "Rattus norvegicus"
	}
	// Can extract more species if needed

	// Create protobuf entry with unified schema
	entry := &pbuf.AntibodyAttr{
		Source:       "imgt_ligm",
		AntibodyType: "sequence",
		SequenceId:   accession,
		Organism:     organism,
		HeavyChainSeq: []string{sequence}, // Store the nucleotide sequence
	}

	// Marshal to JSON
	marshaled, err := ffjson.Marshal(entry)
	a.check(err, fmt.Sprintf("marshaling antibody sequence %s", accession))

	// Save primary entry
	a.d.addProp3(accession, sourceID, marshaled)

	// Add text search for accession
	a.d.addXref(accession, textLinkID, accession, a.source, true)

	// Add text search for description if available
	if description != "" {
		a.d.addXref(accession, textLinkID, description, a.source, true)
	}

	// Log ID if in test mode
	if idLogFile != nil {
		fmt.Fprintln(idLogFile, accession)
	}

	return 1 // Return 1 for success
}

// Helper function to safely get column value
func getColumnValue(row []string, colMap map[string]int, columnName string) string {
	if idx, ok := colMap[columnName]; ok && idx < len(row) {
		return row[idx]
	}
	return ""
}

// Helper function to extract sequences from comma/semicolon separated list
func extractSequences(s string) []string {
	if s == "" {
		return []string{}
	}
	// Split by common separators
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '|'
	})

	result := []string{}
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" && trimmed != "NA" && trimmed != "N/A" && trimmed != "-" {
			result = append(result, trimmed)
		}
	}
	return result
}

// Helper function to extract list items from comma/semicolon separated string
func extractList(s string) []string {
	if s == "" {
		return []string{}
	}
	// Split by common separators
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '|'
	})

	result := []string{}
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" && trimmed != "NA" && trimmed != "N/A" && trimmed != "-" {
			result = append(result, trimmed)
		}
	}
	return result
}
