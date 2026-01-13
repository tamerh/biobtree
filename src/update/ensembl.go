package update

import (
	"biobtree/pbuf"
	"bufio"
	json "encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
	"github.com/tamerh/jsparser"
)

type ensembl struct {
	source               string
	ftpAddress           string
	branch               pbuf.Ensemblbranch
	d                    *DataUpdate
	pauseDurationSeconds int
	// selected genomes paths and taxids
	taxids          map[string]int
	orthologGenomes map[string]int
	gff3Paths       map[string][]string
	jsonPaths       map[string][]string
	biomartPaths    []string
	// HGNC data for human genes (taxid 9606)
	hgncLookup map[string]*pbuf.HgncAttr // Key: Ensembl Gene ID, Value: HGNC attributes
}

// check provides context-aware error checking for ensembl processor
func (e *ensembl) check(err error, operation string) {
	checkWithContext(err, e.source, operation)
}

// loadHGNCData loads HGNC nomenclature data from remote source and builds lookup map
// This is called when processing human genes (taxid 9606) to enrich Ensembl entries
func (e *ensembl) loadHGNCData() {
	log.Println("Ensembl: Loading HGNC data for human genes...")

	// Get HGNC configuration
	hgncPath, ok := config.Dataconf["hgnc"]["path"]
	if !ok {
		log.Println("Ensembl: HGNC path not configured, skipping HGNC data loading")
		return
	}

	// Determine source type and open reader
	var br *bufio.Reader
	var httpResp *http.Response
	var localFile *os.File

	if config.Dataconf["hgnc"]["useLocalFile"] == "yes" {
		file, err := os.Open(filepath.FromSlash(hgncPath))
		if err != nil {
			log.Printf("Ensembl: Warning - could not open local HGNC file: %v, skipping", err)
			return
		}
		br = bufio.NewReaderSize(file, fileBufSize)
		localFile = file
		defer localFile.Close()
	} else if strings.HasPrefix(hgncPath, "http://") || strings.HasPrefix(hgncPath, "https://") {
		// HTTP(S) download
		resp, err := http.Get(hgncPath)
		if err != nil {
			log.Printf("Ensembl: Warning - could not download HGNC data: %v, skipping", err)
			return
		}
		br = bufio.NewReaderSize(resp.Body, fileBufSize)
		httpResp = resp
		defer httpResp.Body.Close()
	} else {
		// FTP fallback
		// Path is now a full URL
		br2, _, ftpFile, client, localFile2, _, err := getDataReaderNew("hgnc", "", "", hgncPath)
		if err != nil {
			log.Printf("Ensembl: Warning - could not get HGNC data via FTP: %v, skipping", err)
			return
		}
		br = br2
		if ftpFile != nil {
			defer ftpFile.Close()
		}
		if localFile2 != nil {
			defer localFile2.Close()
		}
		if client != nil {
			defer client.Quit()
		}
	}

	// Parse HGNC JSON
	p := jsparser.NewJSONParser(br, "docs")
	e.hgncLookup = make(map[string]*pbuf.HgncAttr)

	var processedCount int
	for j := range p.Stream() {
		// Extract Ensembl gene ID (this is the key for lookup)
		var ensemblGeneID string
		if j.ObjectVals["ensembl_gene_id"] != nil {
			switch t := j.ObjectVals["ensembl_gene_id"].(type) {
			case string:
				ensemblGeneID = t
			case (*jsparser.JSON):
				if len(t.ArrayVals) > 0 {
					ensemblGeneID = t.ArrayVals[0].(string) // Take first if multiple
				}
			}
		}

		if ensemblGeneID == "" {
			continue // Skip entries without Ensembl mapping
		}

		// Build HgncAttr
		attr := &pbuf.HgncAttr{}

		// Extract HGNC ID
		if j.ObjectVals["hgnc_id"] != nil {
			if hgncID, ok := j.ObjectVals["hgnc_id"].(string); ok {
				// Store it in a field we can use for cross-referencing
				// We'll add hgnc_id to symbols for searchability
				attr.Symbols = append(attr.Symbols, hgncID)
			}
		}

		// Extract symbols
		if j.ObjectVals["symbol"] != nil {
			switch t := j.ObjectVals["symbol"].(type) {
			case string:
				attr.Symbols = append(attr.Symbols, t)
			case (*jsparser.JSON):
				for _, v := range t.ArrayVals {
					attr.Symbols = append(attr.Symbols, v.(string))
				}
			}
		}

		// Extract aliases
		if j.ObjectVals["alias_symbol"] != nil {
			switch t := j.ObjectVals["alias_symbol"].(type) {
			case string:
				attr.Aliases = append(attr.Aliases, t)
			case (*jsparser.JSON):
				for _, v := range t.ArrayVals {
					attr.Aliases = append(attr.Aliases, v.(string))
				}
			}
		}

		// Extract previous symbols
		if j.ObjectVals["prev_symbol"] != nil {
			switch t := j.ObjectVals["prev_symbol"].(type) {
			case string:
				attr.PrevSymbols = append(attr.PrevSymbols, t)
			case (*jsparser.JSON):
				for _, v := range t.ArrayVals {
					attr.PrevSymbols = append(attr.PrevSymbols, v.(string))
				}
			}
		}

		// Extract names
		if j.ObjectVals["name"] != nil {
			switch t := j.ObjectVals["name"].(type) {
			case string:
				attr.Names = append(attr.Names, t)
			case (*jsparser.JSON):
				for _, v := range t.ArrayVals {
					attr.Names = append(attr.Names, v.(string))
				}
			}
		}

		// Extract previous names
		if j.ObjectVals["prev_name"] != nil {
			switch t := j.ObjectVals["prev_name"].(type) {
			case string:
				attr.PrevNames = append(attr.PrevNames, t)
			case (*jsparser.JSON):
				for _, v := range t.ArrayVals {
					attr.PrevNames = append(attr.PrevNames, v.(string))
				}
			}
		}

		// Extract locus group
		if j.ObjectVals["locus_group"] != nil {
			if locusGroup, ok := j.ObjectVals["locus_group"].(string); ok {
				attr.LocusGroup = locusGroup
			}
		}

		// Extract locus type
		if j.ObjectVals["locus_type"] != nil {
			if locusType, ok := j.ObjectVals["locus_type"].(string); ok {
				attr.LocusType = locusType
			}
		}

		// Extract location (cytogenetic)
		if j.ObjectVals["location"] != nil {
			if location, ok := j.ObjectVals["location"].(string); ok {
				attr.Location = location
			}
		}

		// Extract status
		if j.ObjectVals["status"] != nil {
			if status, ok := j.ObjectVals["status"].(string); ok {
				attr.Status = status
			}
		}

		// Extract gene groups
		if j.ObjectVals["gene_group"] != nil {
			switch t := j.ObjectVals["gene_group"].(type) {
			case string:
				attr.GeneGroups = append(attr.GeneGroups, t)
			case (*jsparser.JSON):
				for _, v := range t.ArrayVals {
					attr.GeneGroups = append(attr.GeneGroups, v.(string))
				}
			}
		}

		// Store in lookup map
		e.hgncLookup[ensemblGeneID] = attr
		processedCount++
	}

	log.Printf("Ensembl: Loaded HGNC data for %d genes", processedCount)
}

// ensembls runs one by one from one place.
func (d *DataUpdate) updateEnsembls(ensembls map[string]ensembl) {

	for _, ensembl := range ensembls {
		ensembl.update()
	}

}

func (e *ensembl) selectGenomes() bool {

	//set files
	taxids := map[string]int{}
	orthologGenomes := map[string]int{}
	gff3FilePaths := map[string][]string{}
	jsonFilePaths := map[string][]string{}
	var biomartFilePaths []string

	allGenomes := false
	if len(e.d.selectedGenomes) == 1 && e.d.selectedGenomes[0] == "all" {
		e.d.selectedGenomes = nil
		e.d.selectedGenomesPattern = nil
		e.d.selectedTaxids = nil
		allGenomes = true
	}

	// first retrieve the path
	ensemblPaths := e.getEnsemblPaths()

	// set orthologGenomes
	if e.d.orthologsActive && !e.d.orthologsAllActive {

		if len(e.d.orthologsIDs) > 0 {
			skippedOrthTaxids := map[int]bool{}
			if len(e.d.selectedGenomes) > 0 { // if this selected only these genomes for ortholog. e.g mouse
				for _, selectedGenome := range e.d.selectedGenomes {
					if _, ok := ensemblPaths.Taxids[selectedGenome]; ok {
						selectedTax := ensemblPaths.Taxids[selectedGenome]
						if _, ok := e.d.orthologsIDs[selectedTax]; ok {
							orthologGenomes[selectedGenome] = selectedTax
							skippedOrthTaxids[selectedTax] = true
						}
					}
				}
			}
			for tax := range e.d.orthologsIDs {
				if _, ok := ensemblPaths.TaxidsRev[tax]; ok {
					for _, genome := range ensemblPaths.TaxidsRev[tax] {
						if _, ok := skippedOrthTaxids[tax]; !ok {
							orthologGenomes[genome] = tax
						}
					}
				}
			}
		} else {
			orthologGenomes = taxids
		}

	}

	if allGenomes { // if all selected

		//gff3
		for sp, v := range ensemblPaths.Gff3s {
			for _, vv := range v {
				if _, ok := gff3FilePaths[sp]; !ok {
					var paths []string
					paths = append(paths, vv)
					gff3FilePaths[sp] = paths
				} else {
					paths := gff3FilePaths[sp]
					paths = append(paths, vv)
					gff3FilePaths[sp] = paths
				}
			}
		}

		//taxids
		taxids = ensemblPaths.Taxids

		//jsons
		for sp, v := range ensemblPaths.Jsons {
			for _, vv := range v {
				if _, ok := jsonFilePaths[sp]; !ok {
					var paths []string
					paths = append(paths, vv)
					jsonFilePaths[sp] = paths
				} else {
					paths := jsonFilePaths[sp]
					paths = append(paths, vv)
					jsonFilePaths[sp] = paths
				}
			}
		}

		//biomarts
		for _, v := range ensemblPaths.Biomarts {
			for _, vv := range v {
				biomartFilePaths = append(biomartFilePaths, vv)
			}
		}

	} else {

		if len(e.d.selectedGenomesPattern) > 0 { // if pattern selected

			e.d.selectedGenomes = nil

			for _, pattern := range e.d.selectedGenomesPattern {

				// set gff3 and selected genomes for use in common biomart func below
				for sp, v := range ensemblPaths.Gff3s {
					if strings.Contains(strings.ToUpper(sp), strings.ToUpper(pattern)) {
						for _, vv := range v {
							if _, ok := gff3FilePaths[sp]; !ok {
								var paths []string
								paths = append(paths, vv)
								gff3FilePaths[sp] = paths
								// set taxid
								if _, ok := ensemblPaths.Taxids[sp]; ok {
									taxids[sp] = ensemblPaths.Taxids[sp]
								}
							} else {
								paths := gff3FilePaths[sp]
								paths = append(paths, vv)
								gff3FilePaths[sp] = paths
							}
						}
						e.d.selectedGenomes = append(e.d.selectedGenomes, sp)
					}
				}
				// set jsons
				for sp, v := range ensemblPaths.Jsons {
					if strings.Contains(strings.ToUpper(sp), strings.ToUpper(pattern)) {
						for _, vv := range v {
							if _, ok := jsonFilePaths[sp]; !ok {
								var paths []string
								paths = append(paths, vv)
								jsonFilePaths[sp] = paths
							} else {
								paths := jsonFilePaths[sp]
								paths = append(paths, vv)
								jsonFilePaths[sp] = paths
							}
						}
					}
				}

			}

			e.writeSelectedGenomes()

		} else {

			hasTaxids := false
			if len(e.d.selectedTaxids) > 0 {

				e.d.selectedGenomes = nil
				hasTaxids = true

				for _, tax := range e.d.selectedTaxids {

					if _, ok := ensemblPaths.TaxidsRev[tax]; ok {

						for _, genome := range ensemblPaths.TaxidsRev[tax] {
							e.d.selectedGenomes = append(e.d.selectedGenomes, genome)
						}

					}

				}

			}

			for _, sp := range e.d.selectedGenomes {

				if _, ok := ensemblPaths.Jsons[sp]; ok {
					jsonFilePaths[sp] = ensemblPaths.Jsons[sp]
					gff3FilePaths[sp] = ensemblPaths.Gff3s[sp]
					// set taxid
					if _, ok := ensemblPaths.Taxids[sp]; ok {
						taxids[sp] = ensemblPaths.Taxids[sp]
					}
				}

			}

			if hasTaxids {
				e.writeSelectedGenomes()
			}

		}

		// biomart
		var biomartSpeciesName string // this is just the shorcut name of species in biomart folder e.g homo_sapiens-> hsapiens
		for _, sp := range e.d.selectedGenomes {

			splitted := strings.Split(sp, "_")
			if len(splitted) > 1 {
				biomartSpeciesName = splitted[0][:1] + splitted[len(splitted)-1]
			} else {
				log.Fatal("Unrecognized species name pattern->" + sp)
			}

			for _, vv := range ensemblPaths.Biomarts[biomartSpeciesName] {
				biomartFilePaths = append(biomartFilePaths, vv)
			}
		}

	}

	// set results
	e.orthologGenomes = orthologGenomes
	e.taxids = taxids
	e.gff3Paths = gff3FilePaths
	e.jsonPaths = jsonFilePaths
	e.biomartPaths = biomartFilePaths

	// this shows that we found genomes or not.
	return len(gff3FilePaths) > 0

}

func (e *ensembl) update() {

	defer e.d.wg.Done()

	sourceMap := map[string]string{"ensembl_havana": "eh", "ensembl": "e", "havana": "h"}

	ensemblTranscriptID := config.Dataconf["transcript"]["id"]
	ensemblProteinID := config.Dataconf["cds"]["id"]
	orthologID := config.Dataconf["ortholog"]["id"]
	paralogID := config.Dataconf["paralog"]["id"]
	exonsID := config.Dataconf["exon"]["id"]

	//set pause setting
	e.pauseDurationSeconds = 2 // default
	if _, ok := config.Appconf["ensemblPauseDuration"]; ok {
		var err error
		e.pauseDurationSeconds, err = strconv.Atoi(config.Appconf["ensemblPauseDuration"])
		if err != nil {
			log.Fatal("Invalid ensemblPauseDuration definition")
		}
	}

	var total uint64
	var previous int64
	var start time.Time
	fr := config.Dataconf["ensembl"]["id"]

	// if local file just ignore ftp jsons
	if config.Dataconf["ensembl"]["useLocalFile"] == "yes" {
		e.jsonPaths = nil
		e.gff3Paths = map[string][]string{}
		e.biomartPaths = nil
		e.gff3Paths["local"] = []string{config.Dataconf["ensembl"]["path"]}
	}

	totalRead := 0
	previous = 0
	start = time.Now()

	// Test mode: get limit and open ID log file
	testLimit := config.GetTestLimit(e.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, e.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// Track processed genes per genome in test mode
	processedGenesPerGenome := make(map[string]int)

	// Check if we're processing human genes (taxid 9606) and load HGNC data
	hasHumanGenome := false
	for genome := range e.gff3Paths {
		if taxid, ok := e.taxids[genome]; ok && taxid == 9606 {
			hasHumanGenome = true
			break
		}
	}
	if hasHumanGenome {
		e.loadHGNCData()
	}

	for genome, paths := range e.gff3Paths {
		for _, path := range paths {

			// Test mode: skip remaining files if this genome has reached its limit
			if config.IsTestMode() && shouldStopProcessing(testLimit, processedGenesPerGenome[genome]) {
				break // Skip remaining files for this genome
			}

			br, _, ftpFile, client, localFile, _, err := getDataReaderNew("ensembl", e.ftpAddress, "", path)
			if err != nil {
				log.Printf("Warning: Failed to retrieve GFF3 file for genome %s at path %s: %v - skipping", genome, path, err)
				continue
			}

			scanner := bufio.NewScanner(br)

			var currTranscript *pbuf.EnsemblAttr
			var currTranscriptID string
			var currGeneID string

		scanLoop:
			for scanner.Scan() {

				l := scanner.Text()
				totalRead += len(l)

				elapsed := int64(time.Since(start).Seconds())
				if elapsed > previous+e.d.progInterval {
					kbytesPerSecond := int64(totalRead) / elapsed / 1024
					previous = elapsed
					e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: kbytesPerSecond}
				}

				if l[0] == '#' {
					continue
				}

				fields := strings.Split(string(l), tab)
				if len(fields) != 9 {
					log.Printf("Invalid line in gff3 has skipped %v\n", string(l))
					continue
				}

				if fields[1] == "." {
					continue // lines without source not used e.g biological_region
				}

				// SeqRegion = fields[0]
				// Source = fields[1]
				// Type = fields[2]
				// Start, _ = strconv.Atoi(fields[3])
				// End, _ = strconv.Atoi(fields[4])
				// Score, _ = strconv.ParseFloat(fields[5], 64)
				// Strand = fields[6][0] // one byte char: +, -, ., or ?
				// Phase, _ = strconv.Atoi(fields[7])
				// Attrs = map[string]string{}

				attrsMap := map[string]string{}
				var eqIndex int
				attrs := fields[8]

				for i := strings.Index(attrs, ";"); i > 0; i = strings.Index(attrs, ";") {
					eqIndex = strings.Index(attrs[:i], "=")
					attrsMap[attrs[:i][:eqIndex]] = attrs[:i][eqIndex+1:]
					attrs = attrs[i+1:]
				}

				eqIndex = strings.Index(attrs, "=")
				attrsMap[attrs[:eqIndex]] = attrs[eqIndex+1:]

				if _, ok := attrsMap["ID"]; ok {
					idAttr := strings.SplitN(attrsMap["ID"], ":", 2)
					if len(idAttr) != 2 {
						continue // this is not truely right but it will panic anyway
					}
					switch idAttr[0] {
					case "gene":
						// Test mode: exit early if this genome has reached its limit
						if config.IsTestMode() && shouldStopProcessing(testLimit, processedGenesPerGenome[genome]) {
							break scanLoop
						}

						attr := pbuf.EnsemblAttr{}

						currGeneID = idAttr[1]

						// Test mode: track and log gene ID for this genome
						if idLogFile != nil {
							logProcessedID(idLogFile, genome+":"+currGeneID)
						}
						processedGenesPerGenome[genome]++
						// Check again after incrementing - if we've hit the limit, break immediately
						if config.IsTestMode() && shouldStopProcessing(testLimit, processedGenesPerGenome[genome]) {
							break scanLoop
						}

						attr.Branch = e.branch

						if _, ok := attrsMap["Name"]; ok {
							attr.Name = attrsMap["Name"]
							e.d.addXref(attrsMap["Name"], textLinkID, currGeneID, "ensembl", true)
						}

						if _, ok := attrsMap["description"]; ok {
							attr.Description = attrsMap["description"]
						}

						if _, ok := attrsMap["biotype"]; ok {
							attr.Biotype = attrsMap["biotype"]
						}

						if _, ok := e.taxids[genome]; ok {
							e.d.addXref(currGeneID, fr, strconv.Itoa(e.taxids[genome]), "taxonomy", false)
						}

						attr.Genome = genome

						if fields[6] != "." {
							attr.Strand = fields[6]
						}

						attr.SeqRegion = fields[0]

						c, err := strconv.Atoi(fields[3])
						if err == nil {
							attr.Start = int32(c)
						}

						c, err = strconv.Atoi(fields[4])
						if err == nil {
							attr.End = int32(c)
						}

						// Embed HGNC data if available for this gene
						if hgncData, ok := e.hgncLookup[currGeneID]; ok {
							attr.Hgnc = hgncData

							// Make HGNC symbols searchable (resolving to Ensembl entry)
							for _, symbol := range hgncData.Symbols {
								if symbol != "" {
									e.d.addXref(symbol, textLinkID, currGeneID, "ensembl", true)
								}
							}

							// Make HGNC aliases searchable
							for _, alias := range hgncData.Aliases {
								if alias != "" {
									e.d.addXref(alias, textLinkID, currGeneID, "ensembl", true)
								}
							}

							// Make HGNC previous symbols searchable
							for _, prevSymbol := range hgncData.PrevSymbols {
								if prevSymbol != "" {
									e.d.addXref(prevSymbol, textLinkID, currGeneID, "ensembl", true)
								}
							}
						}

						b, _ := ffjson.Marshal(attr)
						e.d.addProp3(idAttr[1], fr, b)

					case "transcript":
						// first write current transcript
						if currTranscript != nil {
							b, _ := ffjson.Marshal(currTranscript)
							e.d.addProp3(currTranscriptID, ensemblTranscriptID, b)
						}

						currTranscript = &pbuf.EnsemblAttr{}

						if _, ok := sourceMap[fields[1]]; ok {
							currTranscript.Source = sourceMap[fields[1]]
						}

						currTranscriptID = idAttr[1]
						e.d.addXref(currGeneID, fr, idAttr[1], "transcript", false)

						// if _, ok := attrsMap["Name"]; ok {
						// 	currTranscript.Name = attrsMap["Name"]
						// }

						if _, ok := attrsMap["biotype"]; ok {
							currTranscript.Biotype = attrsMap["biotype"]
						}

						if fields[6] != "." {
							currTranscript.Strand = fields[6]
						}

						currTranscript.SeqRegion = fields[0]

						c, err := strconv.Atoi(fields[3])
						if err == nil {
							currTranscript.Start = int32(c)
						}

						c, err = strconv.Atoi(fields[4])
						if err == nil {
							currTranscript.End = int32(c)
						}

						if _, ok := attrsMap["ccdsid"]; ok {

							ccdsid := strings.Split(attrsMap["ccdsid"], ".")[0]
							e.d.addXref(currTranscriptID, ensemblTranscriptID, ccdsid, "CCDS", false)
							e.d.addXref(currGeneID, fr, ccdsid, "CCDS", false)

						}

					case "CDS":

						attr := pbuf.EnsemblAttr{}

						if fields[6] != "." {
							attr.Strand = fields[6]
						}

						attr.SeqRegion = fields[0]

						c, err := strconv.Atoi(fields[3])
						if err == nil {
							attr.Start = int32(c)
						}

						c, err = strconv.Atoi(fields[4])
						if err == nil {
							attr.End = int32(c)
						}

						if fields[7] != "." {
							c, err = strconv.Atoi(fields[7])
							if err == nil {
								attr.Frame = int32(c)
							}
						}

						b, _ := ffjson.Marshal(attr)
						e.d.addProp3(idAttr[1], ensemblProteinID, b)

						e.d.addXref(currTranscriptID, ensemblTranscriptID, idAttr[1], "eprotein", false)

					}
				} else if fields[2] == "exon" {

					if _, ok := attrsMap["Name"]; ok {

						e.d.addXref(currTranscriptID, ensemblTranscriptID, attrsMap["Name"], "exon", false)

						attr := pbuf.EnsemblAttr{}

						if fields[6] != "." {
							attr.Strand = fields[6]
						}

						attr.SeqRegion = fields[0]

						c, err := strconv.Atoi(fields[3])
						if err == nil {
							attr.Start = int32(c)
						}

						c, err = strconv.Atoi(fields[4])
						if err == nil {
							attr.End = int32(c)
						}

						b, _ := ffjson.Marshal(attr)
						e.d.addProp3(attrsMap["Name"], exonsID, b)

					}

				} else if fields[2] == "five_prime_UTR" {

					c, err := strconv.Atoi(fields[3])
					if err == nil {
						currTranscript.Utr5Start = int32(c)
					}

					c, err = strconv.Atoi(fields[4])
					if err == nil {
						currTranscript.Utr5End = int32(c)
					}

				} else if fields[2] == "three_prime_UTR" {

					c, err := strconv.Atoi(fields[3])
					if err == nil {
						currTranscript.Utr3Start = int32(c)
					}

					c, err = strconv.Atoi(fields[4])
					if err == nil {
						currTranscript.Utr3End = int32(c)
					}

				}

			}

			if ftpFile != nil {
				ftpFile.Close()
			}
			if localFile != nil {
				localFile.Close()
			}

			if client != nil {
				client.Quit()
			}

			// Skip sleep in test mode after reaching limit
			if !config.IsTestMode() || !shouldStopProcessing(testLimit, processedGenesPerGenome[genome]) {
				time.Sleep(time.Duration(e.pauseDurationSeconds) * time.Second) // for not to kicked out from ensembl ftp
			}

		}
	}

	if e.d.orthologsActive {

		// Test mode: skip JSON/xref processing in test mode
		if config.IsTestMode() {
		} else {

		for _, paths := range e.jsonPaths {

			for _, path := range paths {

				previous = 0
				start = time.Now()

				br, _, ftpFile, client, localFile, _, err := getDataReaderNew("ensembl", e.ftpAddress, "", path)
				if err != nil {
					log.Printf("Warning: Failed to retrieve JSON file at path %s: %v - skipping", path, err)
					continue
				}

				p := jsparser.NewJSONParser(br, "genes").SkipProps([]string{"lineage", "evidence", "coord_system", "sifts", "xrefs", "gene_tree_id", "orthology_type", "exons"})

				for j := range p.Stream() {

					if j.ObjectVals["id"] != nil {

						elapsed := int64(time.Since(start).Seconds())
						if elapsed > previous+e.d.progInterval {
							kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
							previous = elapsed
							e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: kbytesPerSecond}
						}

						entryid := j.ObjectVals["id"].(string)

						if j.ObjectVals["homologues"] != nil {
							for _, val := range j.ObjectVals["homologues"].(*jsparser.JSON).ArrayVals {
								if val.(*jsparser.JSON).ObjectVals["stable_id"] != nil {
									stableID := val.(*jsparser.JSON).ObjectVals["stable_id"].(string)
									if val.(*jsparser.JSON).ObjectVals["genome"] != nil && j.ObjectVals["genome"] != nil && val.(*jsparser.JSON).ObjectVals["genome"].(string) == j.ObjectVals["genome"].(string) {
										e.d.addXref2(entryid, fr, stableID, "paralog")
										e.d.addXref2(stableID, paralogID, stableID, "ensembl")
									} else {
										if e.d.orthologsAllActive {
											e.d.addXref2(entryid, fr, stableID, "ortholog")
											e.d.addXref2(stableID, orthologID, stableID, "ensembl")
										} else if e.d.orthologsActive && val.(*jsparser.JSON).ObjectVals["genome"] != nil {
											if _, ok := e.orthologGenomes[val.(*jsparser.JSON).ObjectVals["genome"].(string)]; ok {
												e.d.addXref2(entryid, fr, stableID, "ortholog")
												e.d.addXref2(stableID, orthologID, stableID, "ensembl")
											}
										}
									}
								}
							}
						}

						// maybe these values from configuration
						e.xref(j, entryid, fr, "RefSeq_peptide", "RefSeq")
						e.xref(j, entryid, fr, "EntrezGene", "GeneID")
						e.xref(j, entryid, fr, "Reactome", "Reactome")
						e.xref(j, entryid, fr, "Uniprot/SPTREMBL", "uniprot")
						e.xref(j, entryid, fr, "KEGG_Enzyme", "KEGG")
						e.xref(j, entryid, fr, "CDD", "CDD")
						e.xref(j, entryid, fr, "RefSeq_mRNA", "RefSeq")
						e.xref(j, entryid, fr, "CCDS", "CCDS")
						e.xref(j, entryid, fr, "Uniprot/SWISSPROT", "uniprot")
						e.xref(j, entryid, fr, "UCSC", "UCSC")
						e.xref(j, entryid, fr, "RefSeq_ncRNA_predicted", "RefSeq")
						e.xrefGO(j, entryid, fr) // go terms are also under xrefs with source information.
						// e.xref(j, entryid, fr, "HGNC", "hgnc")

						if e.d.orthologsAllActive {
							e.xref(j, entryid, fr, "Interpro", "interpro")
							e.xref(j, entryid, fr, "HPA", "HPA")
							e.xref(j, entryid, fr, "ArrayExpress", "ExpressionAtlas")
							e.xref(j, entryid, fr, "GENE3D", "CATHGENE3D")
							e.xref(j, entryid, fr, "MIM_GENE", "MIM")
							e.xref(j, entryid, fr, "PANTHER", "PANTHER")
							e.xref(j, entryid, fr, "RNAcentral", "RNAcentral")
							e.xref(j, entryid, fr, "protein_id", "EMBL")
							e.xref(j, entryid, fr, "EMBL", "EMBL")
							e.xref(j, entryid, fr, "TIGRfam", "TIGRFAMs")
							e.xref(j, entryid, fr, "ChEMBL", "ChEMBL")
							e.xref(j, entryid, fr, "UniParc", "uniparc")
							e.xref(j, entryid, fr, "PDB", "PDB")
							e.xref(j, entryid, fr, "SuperFamily", "SUPFAM")
							e.xref(j, entryid, fr, "Prosite_profiles", "PROSITE")
							e.xref(j, entryid, fr, "Pfam", "Pfam")
							e.xref(j, entryid, fr, "Prosite_patterns", "PROSITE")
							e.xref(j, entryid, fr, "HAMAP", "HAMAP")
						}

						if j.ObjectVals["transcripts"] != nil {
							for _, val := range j.ObjectVals["transcripts"].(*jsparser.JSON).ArrayVals {
								tentryid := val.(*jsparser.JSON).ObjectVals["id"].(string)

								if val.(*jsparser.JSON).ObjectVals["translations"] != nil {
									for _, eprotein := range val.(*jsparser.JSON).ObjectVals["translations"].(*jsparser.JSON).ArrayVals {
										e.xref(eprotein.(*jsparser.JSON), eprotein.(*jsparser.JSON).ObjectVals["id"].(string), ensemblProteinID, "Uniprot/SWISSPROT", "uniprot")
										e.xref(eprotein.(*jsparser.JSON), eprotein.(*jsparser.JSON).ObjectVals["id"].(string), ensemblProteinID, "Uniprot/SPTREMBL", "uniprot")
									}
								}

								e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "RefSeq_peptide", "RefSeq")
								e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "EntrezGene", "GeneID")
								e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "Reactome", "Reactome")
								e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "Uniprot/SPTREMBL", "uniprot")
								e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "KEGG_Enzyme", "KEGG")
								e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "CDD", "CDD")
								e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "RefSeq_mRNA", "RefSeq")
								e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "CCDS", "CCDS")
								e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "Uniprot/SWISSPROT", "uniprot")
								e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "UCSC", "UCSC")
								e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "Uniprot_gn", "uniprot")
								e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "RefSeq_ncRNA_predicted", "RefSeq")
								// e.xref(val, tentryid, ensemblTranscriptID, "HGNC", "hgnc")
								e.xrefGO(val.(*jsparser.JSON), tentryid, ensemblTranscriptID)
								if e.d.orthologsAllActive {
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "Interpro", "interpro")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "HPA", "HPA")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "ArrayExpress", "ExpressionAtlas")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "GENE3D", "CATHGENE3D")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "MIM_GENE", "MIM")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "PANTHER", "PANTHER")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "RNAcentral", "RNAcentral")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "protein_id", "EMBL")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "EMBL", "EMBL")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "TIGRfam", "TIGRFAMs")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "ChEMBL", "ChEMBL")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "UniParc", "uniparc")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "PDB", "PDB")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "SuperFamily", "SUPFAM")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "Prosite_profiles", "PROSITE")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "Pfam", "Pfam")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "Prosite_patterns", "PROSITE")
									e.xref(val.(*jsparser.JSON), tentryid, ensemblTranscriptID, "HAMAP", "HAMAP")
								}

							}
						}
					}
					total++
				}

				if ftpFile != nil {
					ftpFile.Close()
				}
				if localFile != nil {
					localFile.Close()
				}

				if client != nil {
					client.Quit()
				}

				time.Sleep(time.Duration(e.pauseDurationSeconds) * time.Second) // for not to kicked out from ensembl ftp
			}

		}
		} // End of else block for test mode skip
	}

	previous = 0
	totalRead = 0
	start = time.Now()
	// probset biomart

	// Test mode: skip BioMart processing in test mode
	if config.IsTestMode() {
	} else {

	for _, path := range e.biomartPaths {
		// first get the probset machine name
		f := strings.Split(path, "/")
		probsetMachine := strings.Split(f[len(f)-1], "__")[1][4:]
		probsetConf := config.Dataconf[probsetMachine]

		if probsetConf != nil {
			fr2 := config.Dataconf[probsetMachine]["id"]
			br2, _, ftpFile2, client, localFile2, _, err := getDataReaderNew(probsetMachine, e.ftpAddress, "", path)
			if err != nil {
				log.Printf("Warning: Failed to retrieve biomart file at path %s: %v - skipping", path, err)
				continue
			}

			scanner := bufio.NewScanner(br2)
			for scanner.Scan() {

				elapsed := int64(time.Since(start).Seconds())
				if elapsed > previous+e.d.progInterval {
					kbytesPerSecond := int64(totalRead) / elapsed / 1024
					previous = elapsed
					e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: kbytesPerSecond}
				}
				s := scanner.Text()
				t := strings.Split(s, "\t")
				if len(t) == 3 && t[2] != "\\N" && t[1] != "\\N" {
					e.d.addXref(t[2], fr2, t[1], "transcript", false)
				}
				totalRead = totalRead + len(s) + 1
			}
			if ftpFile2 != nil {
				ftpFile2.Close()
			}
			if localFile2 != nil {
				localFile2.Close()
			}

			if client != nil {
				client.Quit()
			}

		} else {
			log.Println("Warn: new prob mapping found. It must be defined in configuration", probsetMachine)
		}
		time.Sleep(time.Duration(e.pauseDurationSeconds) * time.Second) // for not to kicked out from ensembl ftp
	}
	} // End of else block for test mode skip

	e.d.progChan <- &progressInfo{dataset: e.source, done: true}
	atomic.AddUint64(&e.d.totalParsedEntry, total)
}

func (e *ensembl) xref(j *jsparser.JSON, entryid, from, propName, dbid string) {

	if j.ObjectVals[propName] != nil {
		for _, val := range j.ObjectVals[propName].(*jsparser.JSON).ArrayVals {
			e.d.addXref(entryid, from, val.(string), dbid, false)
		}
	}
}

func (e *ensembl) xrefGO(j *jsparser.JSON, entryid, from string) {

	if j.ObjectVals["GO"] != nil {
		for _, val := range j.ObjectVals["GO"].(*jsparser.JSON).ArrayVals {
			if _, ok := val.(*jsparser.JSON).ObjectVals["term"]; ok {
				e.d.addXref(entryid, from, val.(*jsparser.JSON).ObjectVals["term"].(string), "GO", false)
			}
		}
	}

}

//xrefProps := []string{"name", "description", "start", "end", "biotype", "genome", "strand", "seq_region_name"}
func (e *ensembl) xrefProp(j *jsparser.JSON, entryid, from string) {

	attr := pbuf.EnsemblAttr{}

	attr.Branch = e.branch

	if j.ObjectVals["name"] != nil {
		attr.Name = j.ObjectVals["name"].(string)
	}

	if j.ObjectVals["description"] != nil {
		attr.Description = j.ObjectVals["description"].(string)
	}

	if j.ObjectVals["biotype"] != nil {
		attr.Biotype = j.ObjectVals["biotype"].(string)
	}

	if j.ObjectVals["genome"] != nil {
		attr.Genome = j.ObjectVals["genome"].(string)
	}

	if j.ObjectVals["strand"] != nil {
		attr.Strand = j.ObjectVals["strand"].(string)
	}

	if j.ObjectVals["seq_region_name"] != nil {
		attr.SeqRegion = j.ObjectVals["seq_region_name"].(string)
	}

	if j.ObjectVals["start"] != nil {
		c, err := strconv.Atoi(j.ObjectVals["start"].(string))
		if err == nil {
			attr.Start = int32(c)
		}
	}

	if j.ObjectVals["end"] != nil {
		c, err := strconv.Atoi(j.ObjectVals["end"].(string))
		if err == nil {
			attr.End = int32(c)
		}
	}

	b, _ := ffjson.Marshal(attr)
	e.d.addProp3(entryid, from, b)

}

func (e *ensembl) writeSelectedGenomes() {

	if len(e.d.selectedGenomes) == 0 {
		return
	}

	selected := map[string]string{} // just show in file better

	for _, sp := range e.d.selectedGenomes {
		selected[sp] = ""
	}
	data, err := json.Marshal(selected)
	check(err)

	ioutil.WriteFile(filepath.FromSlash(config.Appconf["rootDir"]+"/genomes_"+e.source+".json"), data, 0770)

	log.Println("For reference 'genomes_" + e.source + ".json' file created with selected genome list")

}
