package update

// GoAttr is gene ontology attributes
type GoAttr struct {
	Type     string   `json:"type"`
	Label    string   `json:"label"`
	Synonyms []string `json:"synonyms"`
}

func (g *GoAttr) Reset() {
	g.Synonyms = nil
	g.Type = ""
	g.Label = ""
}

type HgncAttr struct {
	Symbol     []string `json:"symbols"`
	Name       []string `json:"names"`
	LocusGroup string   `json:"locus_group"`
	Location   string   `json:"location"`
}

func (h *HgncAttr) Reset() {
	h.Symbol = nil
	h.Location = ""
	h.LocusGroup = ""
	h.Name = nil
}

type InterproAtrr struct {
	ShortName    string   `json:"short_name"`
	Type         string   `json:"type"`
	ProteinCount int      `json:"protein_count"`
	Name         []string `json:"names"`
}

func (i *InterproAtrr) Reset() {
	i.ShortName = ""
	i.Type = ""
	i.ProteinCount = 0
	i.Name = nil
}

type EnsemblAttr struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Start         int    `json:"start"`
	End           int    `json:"end"`
	Biotype       string `json:"biotype"`
	Genome        string `json:"genome"`
	Strand        string `json:"strand"`
	SeqRegionName string `json:"seq_region_name"`
}

type TaxoAttr struct {
	Name              string `json:"name"`
	CommonName        string `json:"common_name"`
	Rank              int    `json:"rank"`
	TaxonomicDivision string `json:"taxonomic_division"`
}

type UniprotAttr struct {
	Accession []string     `json:"accessions"`
	Gene      []string     `json:"gene_name"` // todo review again
	Name      []string     `json:"name"`
	AltName   []string     `json:"alternative_name"`
	SubName   []string     `json:"submitted_name"`
	Features  []UniFeature `json:"features"`
	Sequence  UniSequence  `json:"sequence"`
}

type UniFeature struct {
	Type        string      `json:"type"`
	Description string      `json:"description"`
	ID          string      `json:"id"`
	Evidences   []string    `json:"evidences"`
	Original    string      `json:"original"`
	Variatian   string      `json:"variatian"`
	Loc         UniLocation `json:"location"`
}

type UniSequence struct {
	Seq      string `json:"sequence"`
	Length   int    `json:"length"`
	Mass     int    `json:"mass"`
	Checksum string `json:"checksum"`
}

type UniLocation struct {
	Begin int `json:"begin"`
	End   int `json:"end"`
}

type CommonAttr struct {
	Name         string `json:"name"`
	DiseaseName  string `json:"disease_name"`
	PathwayName  string `json:"pathway_name"`
	Type         string `json:"type"`
	MoleculeType string `json:"molecule_type"`
	Method       string `json:"method"`
	Chains       string `json:"chains"`
	Resuloution  string `json:"resoultion"`
}
