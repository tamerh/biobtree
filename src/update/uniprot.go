package update

import (
	"biobtree/pbuf"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/pquerna/ffjson/ffjson"

	xmlparser "github.com/tamerh/xml-stream-parser"
)

// uniprotStopWords contains common terms in protein names that are too generic for search
// These are filtered when creating tokenized text search entries
var uniprotStopWords = map[string]bool{
	// Very high frequency generic terms
	"protein": true, "subunit": true, "ribosomal": true,
	"large": true, "small": true, "factor": true,
	"uncharacterized": true, "probable": true, "putative": true,
	"chain": true, "homolog": true, "hypothetical": true,

	// Roman numerals
	"ii": true, "iii": true, "iv": true, "vi": true, "vii": true, "viii": true,

	// Location/compartment
	"mitochondrial": true, "chloroplastic": true, "membrane": true,
	"nuclear": true, "transmembrane": true, "cytoplasmic": true,
	"periplasmic": true, "extracellular": true, "cytosolic": true,

	// Functional descriptors (enzyme classes)
	"oxidoreductase": true, "transferase": true, "hydrolase": true,
	"lyase": true, "isomerase": true, "ligase": true, "synthase": true,
	"synthetase": true, "polymerase": true, "protease": true,
	"peptidase": true, "phosphatase": true, "kinase": true,
	"dehydrogenase": true, "reductase": true, "oxidase": true,
	"decarboxylase": true, "deaminase": true, "translocase": true,
	"helicase": true, "endonuclease": true, "exonuclease": true,
	"methyltransferase": true, "acetyltransferase": true,
	"aminotransferase": true, "carboxylase": true, "monooxygenase": true,
	"dioxygenase": true, "hydroxylase": true, "oxygenase": true,

	// Type descriptors
	"type": true, "family": true, "member": true, "class": true,
	"domain": true, "containing": true, "dependent": true, "binding": true,
	"regulatory": true, "catalytic": true, "structural": true,
	"like": true, "related": true, "associated": true,

	// Process words
	"biosynthesis": true, "repair": true, "import": true, "export": true,
	"transport": true, "assembly": true, "replication": true,
	"maturation": true, "biogenesis": true, "modification": true,
	"division": true, "elongation": true, "initiation": true,
	"termination": true, "translation": true, "transcription": true,

	// Common biological terms
	"complex": true, "system": true, "component": true, "channel": true,
	"carrier": true, "center": true, "junction": true, "receptor": true,
	"transporter": true, "regulator": true, "activator": true,
	"inhibitor": true, "repressor": true, "chaperone": true,
	"chaperonin": true, "accessory": true, "resistance": true,

	// Common short words
	"and": true, "of": true, "the": true, "with": true, "for": true,
	"to": true, "in": true, "by": true, "or": true, "at": true,
}

// tokenizeProteinName splits a protein name into significant tokens for search indexing
// It filters out stop words and keeps alphanumeric identifiers like P53, IL6, BCL2
var tokenSplitRegex = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func tokenizeProteinName(name string) []string {
	tokens := tokenSplitRegex.Split(name, -1)
	var significant []string

	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if len(token) < 2 {
			continue // Skip single characters
		}

		lower := strings.ToLower(token)

		// Always keep alphanumeric tokens (like P53, IL6, BCL2, CD4)
		if hasLetterAndDigit(token) {
			significant = append(significant, token)
			continue
		}

		// Skip stop words
		if uniprotStopWords[lower] {
			continue
		}

		// Keep tokens with minimum length of 4 (filters short generic words)
		if len(token) >= 4 {
			significant = append(significant, token)
		}
	}

	return significant
}

// hasLetterAndDigit checks if a string contains both letters and digits
// Used to identify protein/gene identifiers like P53, IL6, BCL2
func hasLetterAndDigit(s string) bool {
	hasLetter := false
	hasDigit := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
		if hasLetter && hasDigit {
			return true
		}
	}
	return false
}

// uniprotSpeciesPriority maps taxonomy IDs to sort priority for UniProt entries
// Lower number = higher priority (appears first in search results)
var uniprotSpeciesPriority = map[string]string{
	"9606":  "01", // Human
	"10090": "02", // Mouse
	"10116": "03", // Rat
	"7955":  "04", // Zebrafish
	"7227":  "05", // Fruit fly
	"6239":  "06", // C. elegans
	"4932":  "07", // Yeast
	"3702":  "08", // Arabidopsis
}

// getUniprotSearchPriority computes combined priority for protein name search indexing
// Combines species priority (2 digits) + inverted xref count (6 digits)
// Example: Human protein with 100 xrefs → "01" + "999900" = "01999900"
// This ensures: 1) Human proteins first, 2) Within species, more-connected proteins first
func getUniprotSearchPriority(taxID string, xrefCount int) string {
	// Get species priority (01-08 for model organisms, 99 for others)
	speciesPriority := "99"
	if p, ok := uniprotSpeciesPriority[taxID]; ok {
		speciesPriority = p
	}

	// Invert xref count so higher counts sort first (lower number = higher priority)
	// Cap at 999999 to fit in 6 digits
	invertedCount := 999999 - xrefCount
	if invertedCount < 0 {
		invertedCount = 0
	}

	// Format: SS + NNNNNN (8 characters total)
	return fmt.Sprintf("%s%06d", speciesPriority, invertedCount)
}

type uniprot struct {
	source      string
	sourceID    string
	d           *DataUpdate
	trembl      bool
	featureID   string
	ensemblID   string
	ensmeblRefs map[string][]string
}

// check provides context-aware error checking for uniprot processor
func (u *uniprot) check(err error, operation string) {
	checkWithContext(err, u.source, operation)
}

func (u *uniprot) processDbReference(entryid string, r *xmlparser.XMLElement) {

	for _, v := range r.Childs["dbReference"] {

		switch v.Attrs["type"] {

		case "EMBL":
			emblID := strings.Split(v.Attrs["id"], ".")[0]
			u.d.addXref(entryid, u.sourceID, emblID, v.Attrs["type"], false)
			for _, z := range v.Childs["property"] {
				if _, ok := z.Attrs["type"]; ok && z.Attrs["type"] == "protein sequence ID" {
					// Link UniProt entry to the protein sequence ID (key is UniProt ID)
					targetEmblID := strings.Split(z.Attrs["value"], ".")[0]
					u.d.addXref(entryid, u.sourceID, targetEmblID, z.Attrs["type"], false)
				} else if _, ok := z.Attrs["type"]; ok && z.Attrs["type"] == "molecule type" {
					attr := pbuf.EnaAttr{}
					attr.Type = strings.ToLower(z.Attrs["value"])
					b, _ := ffjson.Marshal(attr)
					u.d.addProp3(v.Attrs["id"], config.Dataconf[v.Attrs["type"]]["id"], b)
				}
			}
		case "RefSeq":
			refseqID := strings.Split(v.Attrs["id"], ".")[0]
			u.d.addXref(entryid, u.sourceID, refseqID, v.Attrs["type"], false)
			for _, z := range v.Childs["property"] {
				if _, ok := z.Attrs["type"]; ok && z.Attrs["type"] == "nucleotide sequence ID" {
					// Link UniProt entry to the nucleotide sequence ID (key is UniProt ID)
					targetRefseqID := strings.Split(z.Attrs["value"], ".")[0]
					u.d.addXref(entryid, u.sourceID, targetRefseqID, z.Attrs["type"], false)
				}
			}
		case "PDB":
			u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)

		case "DrugBank":
			u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
			for _, z := range v.Childs["property"] {
				switch z.Attrs["type"] {
				case "generic name":
					attr := pbuf.DrugbankAttr{}
					attr.Name = strings.ToLower(z.Attrs["value"])
					b, _ := ffjson.Marshal(attr)
					u.d.addProp3(v.Attrs["id"], config.Dataconf[v.Attrs["type"]]["id"], b)
				}
			}
		case "Ensembl", "EnsemblPlants", "EnsemblBacteria", "EnsemblProtists", "EnsemblMetazoa", "EnsemblFungi":
			// for ensembl it is indexed for swissprot only for now. if ensembl data indexed connection will come from there.
			if !u.trembl {
				for _, z := range v.Childs["property"] {
					if z.Attrs["type"] == "gene ID" {
						// Strip version suffix (e.g., ENSG00000171862.16 -> ENSG00000171862)
						// to match ensembl entries which are stored without version
						geneID := strings.Split(z.Attrs["value"], ".")[0]
						u.ensmeblRefs[geneID] = append(u.ensmeblRefs[geneID], v.Attrs["id"])

					}
				}
			}

		case "Orphanet":
			u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
		case "Reactome":
			u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
		case "GO":
			// Capture this GO annotation's ECO evidence code so it can be attached
			// per-annotation to the uniprot->go edge (not only as an entry-level
			// uniprot->eco edge). Keep the eco edge too so the eco dataset stays
			// reachable ("which evidence codes does this protein use").
			ecoCode := ""
			for _, z := range v.Childs["property"] {
				if z.Attrs["type"] == "evidence" && strings.HasPrefix(z.Attrs["value"], "ECO:") {
					if ecoCode == "" {
						ecoCode = z.Attrs["value"]
					}
					u.d.addXref(entryid, u.sourceID, z.Attrs["value"], "eco", false)
				}
			}
			// Use bucketed xref for GO (has bucket config); evidence empty -> plain bucketed.
			u.d.addXrefBucketedWithEvidence(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false, ecoCode)
		case "BindingDB":
			// Skip - BindingDB creates proper bidirectional xrefs with numeric IDs
			// UniProt's BindingDB dbReferences incorrectly use UniProt accessions as IDs
		case "CTD":
			// Skip - UniProt's CTD dbReferences use gene IDs (e.g., "22", "29")
			// but our CTD dataset uses MeSH chemical IDs (e.g., "D000082")
			// The CTD parser creates proper xrefs from chemicals to genes
		case "SIGNOR":
			// Skip - UniProt's SIGNOR dbReferences use UniProt accessions as IDs
			// but SIGNOR uses interaction IDs (SIGNOR-XXXXXX). The SIGNOR parser creates proper xrefs
		case "DrugCentral":
			// Skip - UniProt's DrugCentral dbReferences incorrectly use UniProt accessions as IDs
			// DrugCentral uses numeric IDs internally
		case "MeSH", "MESH", "mesh":
			// Validate MeSH ID format: should be letter followed by digits (e.g., D000082, C012345)
			meshID := v.Attrs["id"]
			if len(meshID) >= 2 && (meshID[0] == 'D' || meshID[0] == 'C') && meshID[1] >= '0' && meshID[1] <= '9' {
				u.d.addXref(entryid, u.sourceID, meshID, v.Attrs["type"], false)
			}
			// Skip malformed MeSH IDs (e.g., just "2" instead of "D000002")
		case "GuideToPHARMACOLOGY":
			// Map to gtopdb dataset (UniProt uses full name, we use abbreviation)
			u.d.addXref(entryid, u.sourceID, v.Attrs["id"], "gtopdb", false)
		default:
			u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
		}
	}

	if !u.trembl {

		for k, v := range u.ensmeblRefs {
			u.d.addXref(entryid, u.sourceID, k, "ensembl", false)
			for _, t := range v {
				// Link UniProt entry to transcript (key is UniProt ID)
				u.d.addXref(entryid, u.sourceID, t, "transcript", false)
			}
		}

		for k := range u.ensmeblRefs {
			delete(u.ensmeblRefs, k)
		}

	}
}

func (u *uniprot) processSequence(entryid string, r *xmlparser.XMLElement, attr *pbuf.UniprotAttr) {

	if r.Childs["sequence"] != nil {
		seq := r.Childs["sequence"][0]

		attr.Sequence = &pbuf.UniSequence{}
		seqq := strings.Replace(seq.InnerText, "\n", "", -1)
		// Note: sequence.Seq not populated to reduce payload (~50% reduction)
		// Users can get full sequences from UniProt API directly
		// attr.Sequence.Seq = seqq
		attr.Sequence.Length = int32(len(seqq))

		if _, ok := seq.Attrs["mass"]; ok {
			c, err := strconv.Atoi(seq.Attrs["mass"])
			if err == nil {
				attr.Sequence.Mass = int32(c)
			}
		}

	}

}

type evidence struct {
	typee    string
	source   string
	sourceID string
}

// featureLigand extracts the binding-site ligand name + ChEBI id from a feature's
// <ligand><name> / <dbReference type="ChEBI"> children. Returns empty strings if the
// feature has no <ligand> (Atlas issue #38: binding-site ligand was being dropped).
func featureLigand(f *xmlparser.XMLElement) (name, chebiID string) {
	ligs := f.Childs["ligand"]
	if len(ligs) == 0 {
		return
	}
	lig := ligs[0]
	if n := lig.Childs["name"]; len(n) > 0 {
		name = n[0].InnerText
	}
	for _, dbref := range lig.Childs["dbReference"] {
		if dbref.Attrs["type"] == "ChEBI" {
			chebiID = dbref.Attrs["id"]
			break
		}
	}
	return
}

func (u *uniprot) processFeatures(entryid string, r *xmlparser.XMLElement) {

	evidences := map[string]evidence{} // for now value is just the evidence id there is also reference to evidence
	for _, e := range r.Childs["evidence"] {
		if _, ok := e.Attrs["key"]; ok {
			if _, ok := e.Attrs["type"]; ok {

				ev := evidence{}
				ev.typee = e.Attrs["type"]
				if e.Childs["source"] != nil && e.Childs["source"][0].Childs["dbReference"] != nil {
					if _, ok := e.Childs["source"][0].Childs["dbReference"][0].Attrs["type"]; ok {
						if _, ok := e.Childs["source"][0].Childs["dbReference"][0].Attrs["id"]; ok {
							ev.source = strings.ToLower(e.Childs["source"][0].Childs["dbReference"][0].Attrs["type"])
							ev.sourceID = e.Childs["source"][0].Childs["dbReference"][0].Attrs["id"]
							if ev.source == "uniprotkb" { // this for consistency during query
								ev.source = "uniprot"
							}
						}
					}
				}

				evidences[e.Attrs["key"]] = ev
			}
		}
	}

	for index, f := range r.Childs["feature"] {

		feature := pbuf.UniprotFeatureAttr{}

		// feature id
		fentryid := entryid + "_f" + strconv.Itoa(index)

		if _, ok := f.Attrs["type"]; ok {
			feature.Type = f.Attrs["type"]
		}

		if _, ok := f.Attrs["description"]; ok {
			feature.Description = strings.ToLower(f.Attrs["description"])

			// add variants
			splitted := strings.Split(feature.Description, "dbsnp:")
			if len(splitted) == 2 {
				u.d.addXref(fentryid, u.featureID, splitted[1][:len(splitted[1])-1], "variantid", false)
			}
		}

		if _, ok := f.Attrs["id"]; ok {
			feature.Id = f.Attrs["id"]
		}

		if _, ok := f.Attrs["evidence"]; ok {
			evKeys := strings.Split(f.Attrs["evidence"], " ")
			for _, key := range evKeys {
				if _, ok := evidences[key]; ok {
					feature.Evidences = append(feature.Evidences, &pbuf.UniprotFeatureEvidence{Type: evidences[key].typee, Id: evidences[key].sourceID, Source: evidences[key].source})
					if len(evidences[key].source) > 0 && len(evidences[key].sourceID) > 0 {
						if _, ok := config.Dataconf[evidences[key].source]; ok {
							if evidences[key].source == "uniprot" {
								// "By similarity" evidence cites another (often cross-species)
								// UniProt accession. Keep the forward feature->uniprot provenance
								// edge but NOT the reverse: a reverse makes that protein appear
								// linked to every feature (across species) that cites it as
								// evidence, leaking ortholog features into `uniprot >> ufeature`.
								// The evidence is preserved in feature.Evidences regardless.
								u.d.addXref2(fentryid, u.featureID, evidences[key].sourceID, evidences[key].source)
							} else {
								u.d.addXref(fentryid, u.featureID, evidences[key].sourceID, evidences[key].source, false)
							}
						}
					}
				}
			}
		}

		if f.Childs["original"] != nil {
			if f.Childs["variation"] != nil {
				feature.Original = f.Childs["original"][0].InnerText
				feature.Variation = f.Childs["variation"][0].InnerText
			}
		}

		if f.Childs["location"] != nil {
			loc := f.Childs["location"][0]

			if loc.Childs["begin"] != nil && loc.Childs["end"] != nil {

				uniloc := pbuf.UniLocation{}
				if _, ok := loc.Childs["begin"][0].Attrs["position"]; ok {

					c, err := strconv.Atoi(loc.Childs["begin"][0].Attrs["position"])
					if err == nil {
						uniloc.Begin = int32(c)
					}

				}

				if _, ok := loc.Childs["end"][0].Attrs["position"]; ok {

					c, err := strconv.Atoi(loc.Childs["end"][0].Attrs["position"])
					if err == nil {
						uniloc.End = int32(c)
					}

				}
				feature.Location = &uniloc

			} else if loc.Childs["position"] != nil {

				if _, ok := loc.Childs["position"][0].Attrs["position"]; ok {

					uniloc := pbuf.UniLocation{}

					c, err := strconv.Atoi(loc.Childs["position"][0].Attrs["position"])
					if err == nil { // same for begin and end
						uniloc.Begin = int32(c)
						uniloc.End = int32(c)
					}
					feature.Location = &uniloc

				}

			}

		}

		// Binding-site (and similar) features store their ligand in a
		// <ligand><name> child element, NOT in the description attribute, so the
		// description came out empty (Atlas issue #38). Project the ligand name +
		// ChEBI id, and fold the name into description when description is empty so
		// the existing id|type|description|... projection shows the role.
		if name, chebiID := featureLigand(&f); name != "" {
			feature.Ligand = name
			feature.LigandId = chebiID
			if feature.Description == "" {
				feature.Description = strings.ToLower(name)
			}
		}

		// feature xref
		u.d.addXref(entryid, u.sourceID, fentryid, "ufeature", false)

		// feature props
		b, _ := ffjson.Marshal(feature)
		u.d.addProp3(fentryid, u.featureID, b)

	}

}

func (u *uniprot) update(taxoids []int) {

	// Test mode: get limit and open ID log file
	testLimit := config.GetTestLimit("uniprot")
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, "uniprot_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var dataPath string

	taxofilter := true
	taxoidMap := map[int]bool{}
	if len(taxoids) == 0 {
		taxofilter = false
	} else {
		for _, taxo := range taxoids {
			taxoidMap[taxo] = true
		}
	}

	if u.trembl {
		dataPath = config.Dataconf[u.source]["pathTrembl"]
	} else {
		dataPath = config.Dataconf[u.source]["path"]
		u.ensmeblRefs = map[string][]string{}
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(u.source, u.d.uniprotFtp, u.d.uniprotFtpPath, dataPath)
	check(err)

	fr := config.Dataconf[u.source]["id"]
	fr2 := config.Dataconf["ufeature"]["id"]
	if len(fr) <= 0 || len(fr2) <= 0 { // todo these shoud check in the conf
		panic("Uniprot or ufeature id is missing")
	}
	u.sourceID = fr
	u.featureID = fr2
	u.ensemblID = config.Dataconf["ensembl"]["id"]

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer u.d.wg.Done()

	if client != nil {
		defer client.Quit()
	}

	// Comment subtrees are parsed (not skipped): they carry the CC narrative blocks
	// (FUNCTION, SUBCELLULAR LOCATION, DISEASE, ...) and the named isoforms under
	// "alternative products". Benchmarked at ~0% parse-time cost vs skipping.
	p := xmlparser.NewXMLParser(br, "entry")

	var total uint64
	var v, x, z xmlparser.XMLElement
	var entryid string
	var previous int64

	//index := 0

uniloop:
	for r := range streamChecked(p, "uniprot") {

		elapsed := int64(time.Since(u.d.start).Seconds())
		if elapsed > previous+u.d.progInterval {
			kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
			previous = elapsed
			u.d.progChan <- &progressInfo{dataset: u.source, currentKBPerSec: kbytesPerSecond}
		}

		if r.Childs["accession"] == nil {
			log.Println("uniprot entry skipped no accession", r)
			continue
		}

		entryid = r.Childs["accession"][0].InnerText

		// Track taxonomy ID and xref count for priority-based text search indexing
		var entryTaxID string
		// Count dbReferences for priority calculation (approximate xref count)
		entryXrefCount := len(r.Childs["dbReference"])

		for _, v = range r.Childs["organism"] {
			for _, z = range v.Childs["dbReference"] {

				taxoid, err := strconv.Atoi(z.Attrs["id"])
				if err != nil {
					continue
				}
				if taxofilter {
					if _, ok := taxoidMap[taxoid]; !ok {
						continue uniloop
					}
				}

				// Capture taxonomy ID for protein name priority sorting
				entryTaxID = z.Attrs["id"]

				// Use bucketed xref for taxonomy (has bucket config)
				u.d.addXrefBucketed(entryid, fr, z.Attrs["id"], z.Attrs["type"], false)

				for _, x := range z.Childs["property"] {
					// Taxonomy property xref - use uniprot's fr to write to uniprot/forward/
					u.d.addXrefBucketed(z.Attrs["id"], fr, x.Attrs["value"], x.Attrs["type"], false)
				}

			}
		}

		// Compute combined priority: species (2 digits) + inverted xref count (6 digits)
		// Human proteins with more xrefs appear first in search results
		entryPriority := getUniprotSearchPriority(entryTaxID, entryXrefCount)

		attr := pbuf.UniprotAttr{}

		// Note: attr.Reviewed field exists but is not set - we only use SwissProt (reviewed)
		// so this field would always be true and is unnecessary. Leaving it unset (false)
		// means it won't be serialized in JSON output due to omitempty behavior.

		for i := 1; i < len(r.Childs["accession"]); i++ {
			v = r.Childs["accession"][i]
			u.d.addXref(v.InnerText, textLinkID, entryid, u.source, true)
			// Note: accessions are searchable via text index above, no need to store in attr
			// attr.Accessions = append(attr.Accessions, v.InnerText)
		}

		for _, v = range r.Childs["name"] {
			u.d.addXref(v.InnerText, textLinkID, entryid, u.source, true)
		}

		/** test purpose
		if index < 4 {
			u.d.addXref("tpi1", textLinkID, entryid, u.source, true)
			index++
		}
		**/

		/** disabled because gene come from either hgnc or ensembl and uniprot name already contains the name like vav_human
		if r.Childs["gene"] != nil && len(r.Childs["gene"]) > 0 {
			x = r.Childs["gene"][0]
			for _, z = range x.Childs["name"] {
				if _, ok := z.Attrs["type"]; ok && z.Attrs["type"] == "primary" {
					u.d.addXref(z.InnerText, textLinkID, entryid, u.source, true)
					attr.Genes = append(attr.Genes, z.InnerText)
				}
			}
		}**/

		if r.Childs["protein"] != nil {

			x = r.Childs["protein"][0]

			for _, v = range x.Childs["recommendedName"] {
				for _, z = range v.Childs["fullName"] {
					attr.Names = append(attr.Names, z.InnerText)
					// Enable protein name search for SwissProt only (not TrEMBL)
					// Searching "Insulin" will find P01308
					if !u.trembl {
						// Index full protein name with priority (species + xref count)
						u.d.addTextLinkWithPriority(z.InnerText, textLinkID, entryid, u.source, entryPriority)
						// Also index significant tokens from protein name for partial search
						// e.g., "Hemoglobin subunit beta" -> "Hemoglobin", "beta"
						// Uses combined priority (species + xref count) so human proteins
						// with more cross-references appear first in search results
						for _, token := range tokenizeProteinName(z.InnerText) {
							u.d.addTextLinkWithPriority(token, textLinkID, entryid, u.source, entryPriority)
						}
					}
				}
			}

			for _, v = range x.Childs["alternativeName"] {
				for _, z = range v.Childs["fullName"] {
					attr.AlternativeNames = append(attr.AlternativeNames, z.InnerText)
				}
			}

			for _, v = range x.Childs["submittedName"] {
				for _, z = range v.Childs["fullName"] {
					attr.SubmittedNames = append(attr.SubmittedNames, z.InnerText)
				}
			}

		}

		// CC comment narrative blocks + named isoforms (alternative products).
		// Keys are lowercased with spaces->underscores (e.g. "subcellular_location").
		// Curated disease MIM ids are also linked to the mim dataset.
		addComment := func(key, val string) {
			if val == "" {
				return
			}
			if attr.Comments == nil {
				attr.Comments = map[string]string{}
			}
			if existing := attr.Comments[key]; existing != "" {
				attr.Comments[key] = existing + " " + val
			} else {
				attr.Comments[key] = val
			}
		}
		diseaseMimSeen := map[string]bool{}
		for _, c := range r.Childs["comment"] {
			switch c.Attrs["type"] {
			case "alternative products":
				for _, iso := range c.Childs["isoform"] {
					ui := &pbuf.UniIsoform{}
					if ids := iso.Childs["id"]; len(ids) > 0 {
						ui.Id = ids[0].InnerText
					}
					for _, nm := range iso.Childs["name"] {
						ui.Names = append(ui.Names, nm.InnerText)
					}
					if seqs := iso.Childs["sequence"]; len(seqs) > 0 && seqs[0].Attrs["type"] == "displayed" {
						ui.IsCanonical = true
					}
					if ui.Id != "" {
						attr.Isoforms = append(attr.Isoforms, ui)
					}
				}
			case "subcellular location":
				var locs []string
				for _, sl := range c.Childs["subcellularLocation"] {
					for _, loc := range sl.Childs["location"] {
						locs = append(locs, loc.InnerText)
					}
				}
				addComment("subcellular_location", strings.Join(locs, ". "))
			case "disease":
				var parts []string
				for _, dis := range c.Childs["disease"] {
					name := ""
					if n := dis.Childs["name"]; len(n) > 0 {
						name = n[0].InnerText
					}
					acr := ""
					if a := dis.Childs["acronym"]; len(a) > 0 {
						acr = a[0].InnerText
					}
					for _, ref := range dis.Childs["dbReference"] {
						if ref.Attrs["type"] != "MIM" {
							continue
						}
						mimID := ref.Attrs["id"]
						if mimID == "" {
							continue
						}
						if !diseaseMimSeen[mimID] {
							diseaseMimSeen[mimID] = true
							u.d.addXref(entryid, fr, mimID, "mim", false)
						}
						if name != "" {
							if acr != "" {
								parts = append(parts, fmt.Sprintf("%s (%s) [MIM:%s]", name, acr, mimID))
							} else {
								parts = append(parts, fmt.Sprintf("%s [MIM:%s]", name, mimID))
							}
						}
					}
					if desc := dis.Childs["description"]; len(desc) > 0 {
						parts = append(parts, desc[0].InnerText)
					}
				}
				for _, t := range c.Childs["text"] {
					parts = append(parts, t.InnerText)
				}
				addComment("disease", strings.Join(parts, " "))
			default:
				// Narrative types carrying a <text> child (function, subunit, ptm,
				// induction, domain, tissue specificity, miscellaneous, ...).
				for _, t := range c.Childs["text"] {
					addComment(strings.ToLower(strings.ReplaceAll(c.Attrs["type"], " ", "_")), t.InnerText)
				}
			}
		}

		u.processDbReference(entryid, r)

		// todo maybe  more info can be added for the literatuere for later searches e.g scope,title, interaction etc
		for _, v = range r.Childs["reference"] {
			for _, z = range v.Childs["citation"] {
				for _, x = range z.Childs["dbReference"] {
					u.d.addXref(entryid, fr, x.Attrs["id"], x.Attrs["type"], false)
				}
			}
		}

		u.processFeatures(entryid, r)

		u.processSequence(entryid, r, &attr)

		b, _ := ffjson.Marshal(attr)

		// Use bucketed properties for uniprot (has bucket config)
		u.d.addProp3Bucketed(entryid, fr, b)

		// Test mode: log ID
		if idLogFile != nil {
			logProcessedID(idLogFile, entryid)
		}

		total++

		// Test mode: check if limit reached
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(total)) {
			u.d.progChan <- &progressInfo{dataset: u.source, done: true}
			break
		}

	}

	u.d.progChan <- &progressInfo{dataset: u.source, done: true}

	atomic.AddUint64(&u.d.totalParsedEntry, total)
}
