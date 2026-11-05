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
	Genes       []string `json:"genes"`
	MappedGenes int      `json:"mapped_genes"`
	Collection  string   `json:"collection"`
	Background  int      `json:"background"`
}

type EnrichmentTerm struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Collection string  `json:"collection"`
	Overlap    int     `json:"overlap"`
	SetSize    int     `json:"set_size"`
	Fold       float64 `json:"fold"`
	P          float64 `json:"p"`
	FDR        float64 `json:"fdr"`
}

type EnrichmentResult struct {
	Query        EnrichmentQuery  `json:"query"`
	Results      []EnrichmentTerm `json:"results"`
	TestedSets   int              `json:"tested_sets"`
	NotFound     []string         `json:"not_found,omitempty"`
	PagesFetched int              `json:"pages_fetched,omitempty"`
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
	sets := make(map[string]*enrichSet)
	mappedGenes := make(map[string]bool)
	notFound := make(map[string]bool)

	page := ""
	pages := 0
	for {
		res, err := s.MapFilterLite(ids, ">>hgnc>>msigdb", page)
		if err != nil {
			return nil, err
		}

		for _, m := range res.Mappings {
			gene := m.Source
			if i := strings.Index(gene, "|"); i >= 0 {
				gene = gene[:i]
			}
			gene = strings.TrimSpace(gene)
			if gene == "" {
				gene = strings.TrimSpace(m.Input)
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

		// Only the first page's not_found is authoritative; later pages repeat
		// the input terms there even after mapping.
		if pages == 0 {
			for _, nf := range res.NotFound {
				notFound[nf] = true
			}
		}

		page = res.Pagination.NextToken
		pages++
		if page == "" || pages >= 100 {
			break
		}
	}

	result := &EnrichmentResult{
		Query: EnrichmentQuery{
			Genes:       sortedKeys(mappedGenes),
			MappedGenes: len(mappedGenes),
			Collection:  collection,
			Background:  msigdbBackground,
		},
		Results:      []EnrichmentTerm{},
		PagesFetched: pages,
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
		p := hypergeomSF(k, msigdbBackground, K, n)
		fold := (float64(k) / float64(n)) / (float64(K) / float64(msigdbBackground))
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

// normalizeEnrichCollection returns the base collection and optional name
// prefix. The map response stores all GO sets as "C5:GO"; a request for
// "C5:GO:BP" is narrowed with the GOBP_ name prefix.
func normalizeEnrichCollection(collection string) (string, string) {
	collection = strings.TrimSpace(collection)
	if collection == "" {
		collection = "C5:GO"
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
	return collection, ""
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
