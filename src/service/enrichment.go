package service

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// Gene-set over-representation over the MSigDB sets already indexed in
// biobtree. The MSigDB human gene universe is the background; set sizes are
// restricted to 5-500 so specific terms (e.g. a 10-gene GO set) are kept.
const (
	msigdbBackground = 43633
	minSetSize       = 5
	maxSetSize       = 500
)

// Number of sets in each collection that pass the 5-500 size filter. Used as
// the BH denominator; for unknown collections we fall back to the number of
// overlapping sets.
var collectionTotals = map[string]int{
	"C5:GO":          10127,
	"C5:GO:BP":       7276,
	"C5:GO:MF":       1825,
	"C5:GO:CC":       1026,
	"C2:CP:REACTOME": 1818,
	"C2:CGP":         3401,
	"C5:HPO":         5559,
	"H":              50,
}

type EnrichmentQuery struct {
	Genes       []string `json:"genes"`       // resolved HGNC ids used for the test
	InputGenes  []string `json:"input_genes"` // terms exactly as the caller passed them
	MappedGenes int      `json:"mapped_genes"`
	Collection  string   `json:"collection"`
	Background  int      `json:"background"`
}

type EnrichmentTerm struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Collection  string  `json:"collection"`
	GoID        string  `json:"go_id,omitempty"`        // canonical GO id when the set maps to one
	ExactSource string  `json:"exact_source,omitempty"` // source term from the MSigDB attribute
	Overlap     int     `json:"overlap"`
	SetSize     int     `json:"set_size"`
	Fold        float64 `json:"fold"`
	P           float64 `json:"p"`
	FDR         float64 `json:"fdr"`
}

type EnrichmentResult struct {
	Query        EnrichmentQuery  `json:"query"`
	Results      []EnrichmentTerm `json:"results"`
	TestedSets   int              `json:"tested_sets"`
	NotFound     []string         `json:"not_found,omitempty"`
	PagesFetched int              `json:"pages_fetched,omitempty"`
	// Truncated is true when the service stopped at the internal page limit
	// before consuming every input page. The result is usable but incomplete;
	// split a long gene list into smaller batches or raise the limit.
	Truncated bool `json:"truncated,omitempty"`
	PageLimit int  `json:"page_limit,omitempty"`
}

type enrichSet struct {
	ID         string
	Name       string
	Collection string
	SetSize    int
	Genes      map[string]bool
}

// Enrich computes over-representation for ids against MSigDB sets. It reuses
// the same >>hgnc>>msigdb map path as the REST map endpoint, following the
// lite pagination internally.
func (s *Service) Enrich(ids []string, collection string, limit int) (*EnrichmentResult, error) {
	if collection == "" {
		collection = "C5:GO"
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	baseCollection, namePrefix := normalizeEnrichCollection(collection)
	chain := ">>hgnc>>msigdb"
	background := msigdbBackground
	if strings.HasPrefix(baseCollection, "GOA") {
		chain = ">>hgnc>>goa"
		// GOA is a separate human annotation universe; use its own size.
		if idStr, ok := config.Dataconf["goa"]["id"]; ok {
			if id, err := strconv.Atoi(idStr); err == nil {
				if x, xerr := s.LookupByDataset("GOA:UNIVERSE", uint32(id)); xerr == nil && x != nil {
					if a := x.GetMsigdb(); a != nil && a.GeneCount > 0 {
						background = int(a.GeneCount)
					}
				}
			}
		}
	}
	sets := make(map[string]*enrichSet)
	mappedGenes := make(map[string]bool)
	mappedInputs := make(map[string]bool)

	// Resolve symbols to a single HGNC id before querying. The map path's text
	// search is deliberately broad ("AR" matches four genes), which inflates
	// both the overlap and the cohort. Exact official symbols (and explicit
	// HGNC ids) are unambiguous; anything else falls back to its original term.
	resolvedIDs, inputToResolved := s.resolveEnrichInputs(ids)

	page := ""
	pages := 0
	truncated := false
	pageLimit := 100
	for {
		res, err := s.MapFilterLite(resolvedIDs, chain, page)
		if err != nil {
			return nil, err
		}

		for _, m := range res.Mappings {
			if m.Input != "" {
				mappedInputs[strings.ToUpper(strings.TrimSpace(m.Input))] = true
			}
			gene := m.Source
			if i := strings.Index(gene, "|"); i >= 0 {
				gene = gene[:i]
			}
			gene = strings.TrimSpace(gene)
			if gene == "" {
				gene = strings.TrimSpace(m.Input)
			}
			if gene != "" {
				mappedGenes[gene] = true
			}

			for _, target := range m.Targets {
				id, name, coll, size, ok := parseMsigdbTarget(target)
				if !ok {
					continue
				}
				if baseCollection != "" && !strings.HasPrefix(coll, baseCollection) {
					continue
				}
				if namePrefix != "" && !strings.HasPrefix(name, namePrefix) {
					continue
				}
				if size < minSetSize || size > maxSetSize {
					continue
				}

				es := sets[id]
				if es == nil {
					es = &enrichSet{
						ID:         id,
						Name:       name,
						Collection: coll,
						SetSize:    size,
						Genes:      make(map[string]bool),
					}
					sets[id] = es
				}
				es.Genes[gene] = true
				mappedGenes[gene] = true
			}
		}

		page = res.Pagination.NextToken
		pages++
		if page == "" {
			break
		}
		if pages >= pageLimit {
			truncated = true
			break
		}
	}

	// Compute not_found from the resolved inputs, not from the map pages.
	// The service repeats not_found per page for long lists; deriving it from
	// which resolved ids actually appeared is authoritative.
	notFound := make(map[string]bool)
	for _, raw := range ids {
		key := strings.ToUpper(strings.TrimSpace(raw))
		if key == "" {
			continue
		}
		rid := inputToResolved[key]
		if mappedInputs[key] || (rid != "" && mappedGenes[rid]) {
			continue
		}
		// mappedGenes is keyed by the source id as returned by the map; accept
		// a case-insensitive lookup for the resolved id too.
		found := false
		for g := range mappedGenes {
			if strings.EqualFold(g, rid) {
				found = true
				break
			}
		}
		if !found {
			notFound[raw] = true
		}
	}

	result := &EnrichmentResult{
		Query: EnrichmentQuery{
			Genes:       sortedKeys(mappedGenes),
			InputGenes:  ids,
			MappedGenes: len(mappedGenes),
			Collection:  collection,
			Background:  background,
		},
		Results:      []EnrichmentTerm{},
		PagesFetched: pages,
		Truncated:    truncated,
	}
	if truncated {
		result.PageLimit = pageLimit
	}
	if len(notFound) > 0 {
		result.NotFound = sortedKeys(notFound)
	}

	n := len(mappedGenes)
	if n == 0 || len(sets) == 0 {
		return result, nil
	}

	terms := make([]EnrichmentTerm, 0, len(sets))
	for _, es := range sets {
		k := len(es.Genes)
		if k == 0 {
			continue
		}
		K := es.SetSize
		if k > K {
			// The set metadata says it has fewer genes than the input matched.
			// This is a data inconsistency (historically caused by ambiguous
			// symbol resolution); publishing p=0 here would be wrong.
			continue
		}
		p := hypergeomSF(k, background, K, n)
		fold := (float64(k) / float64(n)) / (float64(K) / float64(background))
		terms = append(terms, EnrichmentTerm{
			ID:         es.ID,
			Name:       es.Name,
			Collection: es.Collection,
			Overlap:    k,
			SetSize:    K,
			Fold:       fold,
			P:          p,
		})
	}
	result.TestedSets = len(terms)
	if len(terms) == 0 {
		return result, nil
	}

	// Canonical IDs make every identifier in an answer traceable to the tool
	// output instead of being recalled from memory: GO terms carry go_id,
	// MSigDB sets keep their exact source.
	s.attachEnrichTermIDs(terms, baseCollection)

	m := collectionTotals[collection]
	if m < len(terms) {
		m = len(terms)
	}
	pvals := make([]float64, len(terms))
	for i := range terms {
		pvals[i] = terms[i].P
	}
	qvals := bhFDR(pvals, m)
	for i := range terms {
		terms[i].FDR = qvals[i]
	}

	// Rank by significance (FDR) first, then fold. Fold alone promotes tiny
	// sets (e.g. 2/6) above a stronger 3/10 overlap; ORA convention is to lead
	// with the multiple-testing-corrected p-value while still reporting fold.
	sort.Slice(terms, func(i, j int) bool {
		if terms[i].FDR != terms[j].FDR {
			return terms[i].FDR < terms[j].FDR
		}
		if terms[i].Fold != terms[j].Fold {
			return terms[i].Fold > terms[j].Fold
		}
		return terms[i].Overlap > terms[j].Overlap
	})
	if len(terms) > limit {
		terms = terms[:limit]
	}
	result.Results = terms
	return result, nil
}

// resolveEnrichInputs maps caller terms to one HGNC id each. Explicit HGNC ids
// are kept; official gene symbols are resolved by exact symbol match. Anything
// else (Ensembl/RefSeq/alias) is passed through unchanged and left to the map
// path.
func (s *Service) resolveEnrichInputs(ids []string) ([]string, map[string]string) {
	resolved := make([]string, 0, len(ids))
	inputToResolved := make(map[string]string, len(ids))
	seen := make(map[string]bool)

	for _, raw := range ids {
		term := strings.TrimSpace(raw)
		if term == "" {
			continue
		}
		key := strings.ToUpper(term)
		rid := term
		if !strings.HasPrefix(key, "HGNC:") {
			if id := s.resolveExactHgncSymbol(term); id != "" {
				rid = id
			}
		}
		inputToResolved[key] = rid
		rkey := strings.ToUpper(rid)
		if !seen[rkey] {
			seen[rkey] = true
			resolved = append(resolved, rid)
		}
	}
	return resolved, inputToResolved
}

// resolveExactHgncSymbol returns the HGNC id whose official symbol equals the
// supplied term, or "" when there is no exact symbol match. Text search alone
// is too broad for this job (AR matches four genes).
func (s *Service) resolveExactHgncSymbol(symbol string) string {
	hgncID, ok := config.DataconfIDStringToInt["hgnc"]
	if !ok || hgncID == 0 {
		return ""
	}
	entries, err := s.LookupAllInDataset(symbol, hgncID)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		full, ferr := s.LookupFullEntry(e.Identifier, hgncID)
		if ferr != nil || full == nil {
			continue
		}
		a := full.GetHgnc()
		if a == nil {
			continue
		}
		for _, candidate := range a.Symbols {
			if strings.EqualFold(candidate, symbol) {
				return e.Identifier
			}
		}
	}
	return ""
}

// normalizeEnrichCollection returns the base collection and optional name
// prefix. The map response stores all GO sets as "C5:GO"; a request for
// "C5:GO:BP" is narrowed with the GOBP_ name prefix.
func normalizeEnrichCollection(collection string) (string, string) {
	collection = strings.TrimSpace(collection)
	if collection == "" {
		collection = "C5:GO"
	}

	// GO-native mode over GOA human annotations. "GO" means all namespaces;
	// "GO:BP/MF/CC" narrows to one.
	upper := strings.ToUpper(collection)
	if upper == "GO" || upper == "GOA" {
		return "GOA", ""
	}
	if strings.HasPrefix(upper, "GO:") {
		return "GOA:" + strings.TrimPrefix(upper, "GO:"), ""
	}
	if strings.HasPrefix(upper, "GOA:") {
		return upper, ""
	}

	if strings.HasPrefix(collection, "C5:GO:") {
		ns := strings.ToUpper(strings.TrimPrefix(collection, "C5:GO:"))
		switch ns {
		case "BP":
			return "C5:GO", "GOBP_"
		case "MF":
			return "C5:GO", "GOMF_"
		case "CC":
			return "C5:GO", "GOCC_"
		}
	}
	// C2:CP is a mixed collection (Reactome + PID + BioCarta + KEGG). The
	// documented sub-collection names select one contributor by name prefix.
	if strings.HasPrefix(collection, "C2:CP:") {
		ns := strings.ToUpper(strings.TrimPrefix(collection, "C2:CP:"))
		switch ns {
		case "REACTOME":
			return "C2:CP", "REACTOME_"
		case "PID":
			return "C2:CP", "PID_"
		case "BIOCARTA":
			return "C2:CP", "BIOCARTA_"
		case "KEGG":
			return "C2:CP", "KEGG_"
		}
	}
	return collection, ""
}

// attachEnrichTermIDs adds the canonical GO id / exact source to each result.
// GOA sets are natively GO terms; MSigDB sets are looked up once per set to
// read MsigdbAttr.go_terms and exact_source.
func (s *Service) attachEnrichTermIDs(terms []EnrichmentTerm, baseCollection string) {
	if strings.HasPrefix(baseCollection, "GOA") {
		for i := range terms {
			terms[i].GoID = terms[i].ID
		}
		return
	}

	idStr, ok := config.Dataconf["msigdb"]["id"]
	if !ok {
		return
	}
	msigdbID, err := strconv.Atoi(idStr)
	if err != nil {
		return
	}

	type termIDs struct {
		goID  string
		exact string
	}
	cache := make(map[string]termIDs, len(terms))
	for i := range terms {
		v, ok := cache[terms[i].ID]
		if !ok {
			x, xerr := s.LookupByDataset(terms[i].ID, uint32(msigdbID))
			if xerr == nil && x != nil {
				if a := x.GetMsigdb(); a != nil {
					if len(a.GoTerms) > 0 {
						v.goID = a.GoTerms[0]
					}
					v.exact = a.ExactSource
				}
			}
			cache[terms[i].ID] = v
		}
		terms[i].GoID = v.goID
		terms[i].ExactSource = v.exact
	}
}

// parseMsigdbTarget parses id|name|collection|description|size, honouring
// escaped pipes in the description.
func parseMsigdbTarget(target string) (id, name, collection string, size int, ok bool) {
	parts := splitEscapedPipe(target)
	if len(parts) < 5 {
		return "", "", "", 0, false
	}
	id = strings.TrimSpace(parts[0])
	name = strings.TrimSpace(parts[1])
	collection = strings.TrimSpace(parts[2])
	size, err := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
	if err != nil || id == "" || name == "" {
		return "", "", "", 0, false
	}
	return id, name, collection, size, true
}

func splitEscapedPipe(s string) []string {
	out := make([]string, 0, 5)
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == '|' {
			b.WriteByte('|')
			i++
			continue
		}
		if s[i] == '|' {
			out = append(out, b.String())
			b.Reset()
			continue
		}
		b.WriteByte(s[i])
	}
	out = append(out, b.String())
	return out
}

func hypergeomSF(k, N, K, n int) float64 {
	if k <= 0 {
		return 1.0
	}
	upper := K
	if n < upper {
		upper = n
	}
	if k > upper {
		return 0.0
	}

	logDenom := lgamma(N+1) - lgamma(n+1) - lgamma(N-n+1)
	p := 0.0
	for i := k; i <= upper; i++ {
		logP := lgamma(K+1) - lgamma(i+1) - lgamma(K-i+1) +
			lgamma(N-K+1) - lgamma(n-i+1) - lgamma(N-K-n+i+1) - logDenom
		p += math.Exp(logP)
	}
	if p > 1.0 {
		return 1.0
	}
	return p
}

func lgamma(x int) float64 {
	v, _ := math.Lgamma(float64(x))
	return v
}

// bhFDR returns Benjamini-Hochberg adjusted p-values using m tests.
func bhFDR(pvals []float64, m int) []float64 {
	n := len(pvals)
	if n == 0 {
		return nil
	}
	if m < n {
		m = n
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return pvals[order[i]] < pvals[order[j]] })

	q := make([]float64, n)
	prev := 1.0
	for rank := n; rank >= 1; rank-- {
		idx := order[rank-1]
		val := pvals[idx] * float64(m) / float64(rank)
		if val > prev {
			val = prev
		}
		q[idx] = val
		prev = val
	}
	return q
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
