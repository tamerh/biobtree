j# dbSNP - Single Nucleotide Polymorphism Database

## Overview

dbSNP is NCBI's authoritative database of genetic variation, providing comprehensive information on single nucleotide polymorphisms (SNPs) and other small-scale genetic variants. Contains over 1 billion human variants with detailed genomic coordinates, population frequencies, functional annotations, and clinical significance data. Essential for variant interpretation, GWAS studies, and clinical genomics applications requiring high-quality variant catalogs.

**Source**: NCBI dbSNP VCF files (ftp.ncbi.nlm.nih.gov)
**Data Type**: Genetic variants with genomic coordinates and functional annotations
**Assembly**: GRCh38.p14 (human reference genome)

## Integration Architecture

### Storage Model

**Primary Entries**:
- RefSNP IDs (e.g., `rs7903146`) serve as primary keys
- Comprehensive variant metadata stored as attributes

**Searchable Text Links**:
- rs IDs indexed as keywords for direct lookup
- Gene symbols indexed for symbol-based search (e.g., "BRCA1" finds associated SNPs)

**Attributes Stored** (protobuf DbsnpAttr):

#### Basic Variant Information
- `rs_id`: RefSNP identifier (e.g., "rs7903146")
- `build_id`: dbSNP build version (e.g., "157")
- `chromosome`: Chromosome location (e.g., "1", "X", "MT")
- `position`: Chromosomal position (1-based coordinate)
- `ref_allele`: Reference allele sequence
- `alt_allele`: Alternate allele sequence
- `variant_type`: Determined variant type (SNV, insertion, deletion, MNV, complex)
- `variant_class`: dbSNP variant class annotation

#### Population Frequencies
- `gnomad_frequency`: gnomAD global allele frequency (from FREQ field)
- `gnomad_populations`: Population-specific gnomAD frequencies (array of PopulationFrequency)
- `thousand_genomes_populations`: 1000 Genomes population frequencies (array of PopulationFrequency)

**PopulationFrequency** structure:
- `population`: Population name (e.g., "GnomAD_genomes", "TOPMED", "1000Genomes")
- `frequency`: Alternate allele frequency (0.0-1.0)
- `allele_count`: Number of alternate alleles observed
- `allele_number`: Total number of alleles
- `homozygote_count`: Number of homozygous individuals

#### Gene Context
- `gene_names`: Gene symbols associated with variant (array, e.g., ["BRCA1", "TP53"])
- `gene_ids`: Entrez Gene IDs (array)
- `pseudogene_names`: Pseudogene symbols (array)
- `pseudogene_ids`: Pseudogene IDs (array)
- `gene_locus`: Cytogenetic location from LOC field (e.g., "17q21.31")

#### HGVS Nomenclature
- `hgvs_mane`: MANE Select transcript annotation (HgvsAnnotation)
- `hgvs_transcripts`: All transcript annotations (array of HgvsAnnotation)

**HgvsAnnotation** structure:
- `transcript_id`: RefSeq transcript ID (e.g., "NM_001005484.2")
- `gene_symbol`: Gene symbol (e.g., "OR4F5")
- `hgvs_c`: HGVS c. notation (e.g., "c.339T>G", "c.9+1317")
- `hgvs_p`: HGVS p. notation for protein changes (reserved)
- `consequence`: Variant consequence ("coding", "intronic", "5_utr", "3_utr")
- `is_mane_select`: True if this is the MANE Select transcript
- `is_mane_plus_clinical`: True if MANE Plus Clinical transcript

#### Variant Origin & Quality
- `sao`: Variant Allele Origin
  - 0 = unspecified
  - 1 = Germline (inherited variation)
  - 2 = Somatic (cancer/tumor variants)
  - 3 = Both germline and somatic
- `is_common`: Common SNP flag (MAF >= 1% in 1000 Genomes populations)
- `ssr`: Suspect Reason Codes (quality flags, can be summed)
  - 1 = Paralog (maps to paralogous sequence)
  - 2 = byEST (variant called from EST data)
  - 4 = oldAlign (mapping based on older genome build)
  - 8 = Para_EST (paralogous EST)
  - 16 = 1kg_failed (failed 1000 Genomes QC)
  - 1024 = other

#### Functional Impact Flags - Coding Region Effects
- `nsf`: Non-synonymous frameshift (FxnClass=44)
  - Coding region variant that changes all downstream amino acids
- `nsm`: Non-synonymous missense (FxnClass=42)
  - Coding region variant that changes protein peptide
- `nsn`: Non-synonymous nonsense (FxnClass=41)
  - Coding region variant that creates STOP codon
- `syn`: Synonymous (FxnCode=3)
  - Coding region variant that does not change encoded amino acid

#### Functional Impact Flags - UTR & Splice Sites
- `u3`: In 3' UTR (FxnCode=53)
  - Variant in 3' untranslated region
- `u5`: In 5' UTR (FxnCode=55)
  - Variant in 5' untranslated region
- `ass`: In acceptor splice site (FxnCode=73)
  - Variant affects acceptor splice site
- `dss`: In donor splice site (FxnCode=75)
  - Variant affects donor splice site

#### Functional Impact Flags - Gene Regions
- `intron`: In intron (FxnCode=6)
  - Variant located in intronic region
- `r3`: In 3' gene region (FxnCode=13)
  - Variant in 3' gene flanking region
- `r5`: In 5' gene region (FxnCode=15)
  - Variant in 5' gene flanking region

#### Evidence & Literature
- `has_publication`: PM flag - Variant has associated publication
- `has_pubmed_ref`: PUB flag - RefSNP mentioned in a publication
- `has_genotypes`: GNO flag - Genotypes available for this variant
- `pubmed_ids`: PubMed citation IDs from PMID field (array)

#### Clinical Data (ClinVar Integration)
- `clinical_significance`: Clinical significance from ClinVar (CLNSIG field)
- `clinvar_variation_id`: ClinVar Variation ID from CLNVI field
- `clinvar_accession`: ClinVar accession from CLNACC field (e.g., "RCV000123456")
- `clinvar_review_status`: Review status from CLNREVSTAT field
- `clinvar_disease_names`: Disease names from CLNDN field (array)
- `clinvar_disease_ids`: Disease database IDs from CLNDISDB field (array, e.g., "MedGen:C0020445")
- `clinvar_origin`: Allele origin from CLNORIGIN field
- `clinvar_hgvs`: ClinVar HGVS notation from CLNHGVS field

#### Historical Data
- `merged_rs_ids`: Historical merged rs IDs from OLD field (array)

### Cross-References

**Gene Associations**:
- **HGNC**: Via gene symbol lookup (official human gene symbols only, ~45% coverage)
- **NCBI Gene (Entrez)**: Via gene symbol lookup (comprehensive, ~78% coverage, includes LOC identifiers)
- **Ensembl**: Via gene symbol lookup (human genome only, ~45% coverage)
- See "Gene Symbol to Human Gene Database Mapping" section for details on coverage differences

**Clinical Database Links**:
- **ClinVar**: Via clinvar_variation_id for variants with clinical annotations
- **PubMed**: Via pubmed_ids for literature references

**Pathogenicity Predictions (AlphaMissense)**:
- **AlphaMissense (coordinate-based)**: Direct link to variant-level pathogenicity predictions
  - Only for coding SNVs (excludes intronic, UTR, and flanking region variants)
  - Uses coordinate format: `chr:pos:ref:alt` (e.g., `1:69094:G:T`)
  - Query: `rs12345 >> dbsnp >> alphamissense`
- **AlphaMissense Transcript (via RefSeq)**: Link to transcript-level mean pathogenicity
  - Uses HGVS annotations to link to RefSeq transcripts
  - Chain: `rs12345 >> dbsnp >> refseq >> transcript >> alphamissense_transcript`

**Transcript Links**:
- **RefSeq Transcripts**: Via HGVS annotations (MANE Select and all transcripts)
  - Enables mapping to Ensembl transcripts and AlphaMissense data
  - Uses base transcript ID without version (e.g., `NM_001005484`)

**Text Search**:
- rs IDs indexed as keywords (direct lookup: "rs7903146")
- Gene symbols indexed (symbol search: "TCF7L2" finds associated SNPs)

### Gene Symbol to Human Gene Database Mapping

dbSNP gene symbols (from GENEINFO and PSEUDOGENEINFO fields) are cross-referenced to **three** human gene databases using `addHumanGeneXrefsAll()`:

| Database | Coverage | Description |
|----------|----------|-------------|
| **HGNC** | ~45% | Official human gene nomenclature (~43K genes). Only approved symbols. |
| **Entrez** | ~78% | Comprehensive NCBI Gene database (~60K+ human genes). Includes LOC identifiers, predicted genes, pseudogenes. |
| **Ensembl** | ~45% | Human genes with genomic coordinates. Similar coverage to HGNC. |

**Why the coverage differs:**

- **LOC identifiers** (e.g., LOC123456789): These are NCBI locus identifiers for predicted/uncharacterized genes. They exist **only in Entrez**, not in HGNC or Ensembl.
- **Predicted genes**: NCBI predicts genes that Ensembl may not annotate. These are in Entrez only.
- **Discontinued genes**: Some gene symbols are retired by NCBI. These won't map to any database.

**Query paths for different gene types:**

```
# Official genes (BRCA1, TP53, etc.) - all paths work:
BRCA1 >> hgnc >> dbsnp     ✓
BRCA1 >> entrez >> dbsnp   ✓
BRCA1 >> ensembl >> dbsnp  ✓

# LOC identifiers - only Entrez path works:
LOC123456 >> entrez >> dbsnp   ✓
LOC123456 >> hgnc >> dbsnp     ✗ (not in HGNC)
LOC123456 >> ensembl >> dbsnp  ✗ (not in Ensembl)

# LOC to Ensembl (indirect, if Entrez has the xref):
LOC123456 >> entrez >> ensembl  (works if Entrez has Ensembl xref for this gene)
```

**Recommendation for MCP queries:**
- For **official gene analysis**: Use any path (HGNC preferred for nomenclature)
- For **comprehensive variant analysis**: Use Entrez path (highest coverage)
- For **genomic context** (coordinates, transcripts): Use Ensembl path

### Special Features

**HGVS Nomenclature Support**:
- Uses RefSeq GFF3 annotation file (~77MB) for accurate transcript mapping
- Loads 19,394 genes and 67,331 transcripts at startup
- Provides both MANE Select annotation (clinical standard) and all transcript annotations
- Computes proper c. notation:
  - Coding variants: `c.339T>G` (position within CDS)
  - Intronic variants: `c.9+1317` (relative to nearest exon boundary)
  - UTR variants: `c.-50G>A` (5' UTR) or `c.*100A>G` (3' UTR)
- Cached GFF3 file for fast subsequent loads

**Population Frequency Integration**:
- Parses FREQ field for multi-source frequency data
- Supports gnomAD, 1000 Genomes, TOPMED, KOREAN, and other populations
- Case-insensitive matching for population names
- Stores both global gnomAD frequency and detailed population breakdowns

**Gene Symbol to Human Gene Database Mapping**:
- Gene symbols from GENEINFO and PSEUDOGENEINFO fields create xrefs to HGNC, Entrez, and Ensembl
- Uses `addHumanGeneXrefsAll()` which iterates through ALL entries to find human-specific genes
- Entrez uses taxonomy 9606 filter, Ensembl uses genome="homo_sapiens" filter
- See "Gene Symbol to Human Gene Database Mapping" section above for coverage details
- Example workflow:
  1. Search "DDX11L16" -> finds HGNC, Entrez, and/or Ensembl entries
  2. Query "DDX11L16 >> dbsnp" -> returns all SNPs in that gene
  3. For LOC identifiers, only Entrez path works (LOC genes not in HGNC/Ensembl)

**Comprehensive Functional Annotation**:
- 11 functional impact flags enable precise filtering
- Coding effects: frameshift, missense, nonsense, synonymous
- Splice sites: acceptor and donor splice sites
- Gene regions: UTRs, introns, flanking regions

**Quality and Evidence Indicators**:
- SSR codes identify potentially problematic variants
- Publication flags indicate well-studied variants
- Genotype availability flags indicate data richness

**Streaming VCF Processing**:
- No in-memory accumulation - processes variants one at a time
- Optimized INFO field parsing (extracts only needed fields)
- Handles large VCF files (multi-GB) without memory explosion
- Test mode: chr1 only for fast validation
- Production mode: all chromosomes (1-22, X, Y, MT)

## Use Cases

**1. Variant Lookup by rs ID**
```
Query: rs7903146 -> Retrieve all attributes
Use: Get comprehensive information about a known variant
```

**2. Gene to Variants Mapping**
```
Query: TCF7L2 >> dbsnp -> All SNPs in TCF7L2 gene
Use: Find all genetic variation in a gene of interest
```

**3. Functional Impact Filtering**
```
Query: All SNPs -> Filter nsm=true OR nsn=true -> Protein-changing variants
Use: Focus on variants likely to affect protein function
```

**4. Common vs Rare Variants**
```
Query: All SNPs -> Filter is_common=true -> Common variants (MAF >= 1%)
Query: All SNPs -> Filter is_common=false -> Rare variants
Use: Population genetics and rare disease studies
```

**5. Germline vs Somatic Filtering**
```
Query: All SNPs -> Filter sao=1 -> Germline variants only
Query: All SNPs -> Filter sao=2 -> Somatic/cancer variants only
Use: Separate inherited variation from cancer mutations
```

**6. Quality Filtering**
```
Query: All SNPs -> Filter ssr=0 -> High-quality variants only
Query: All SNPs -> Filter has_publication=true -> Well-studied variants
Use: Remove suspect variants, focus on validated SNPs
```

**7. Splice Site Variants**
```
Query: All SNPs -> Filter ass=true OR dss=true -> Splice site variants
Use: Identify variants affecting RNA splicing
```

**8. Population Frequency Filtering**
```
Query: All SNPs -> Filter gnomad_frequency < 0.01 -> Rare in gnomAD
Query: All SNPs -> Check gnomad_populations for specific populations
Use: Population-specific frequency analysis
```

**9. HGVS-based Queries**
```
Query: SNP -> hgvs_mane.hgvs_c -> Get clinical HGVS notation
Query: SNP -> hgvs_transcripts -> Get all transcript-level annotations
Use: Clinical reporting, variant nomenclature standardization
```

**10. Cross-Database Integration**
```
Query: SNP -> xrefs -> Ensembl genes, ClinVar entries, PubMed articles
Use: Link genomic data with genes, diseases, literature, and traits
```

**11. AlphaMissense Pathogenicity Lookup (Variant-Level)**
```
Query: rs12345 >> dbsnp >> alphamissense
Result: Pathogenicity score (0-1), classification (likely_benign/ambiguous/likely_pathogenic)
Use: Assess missense variant pathogenicity for clinical interpretation
Note: Only works for coding SNVs - intronic/UTR variants excluded
```

**12. AlphaMissense Transcript Pathogenicity (Gene-Level)**
```
Query: rs12345 >> dbsnp >> refseq >> transcript >> alphamissense_transcript
Result: Mean pathogenicity score for the transcript
Use: Assess overall mutation tolerance of the affected transcript/gene
Chain: dbSNP HGVS -> RefSeq transcript -> Ensembl transcript -> AlphaMissense
```

## Test Cases

**Current Tests** (21 total):
- 4 declarative tests (JSON-based)
- 17 custom tests (Python logic)

**Coverage**:
- Basic rs ID lookup
- Attribute presence validation
- Multiple ID batch lookup (5 SNPs)
- Invalid ID handling
- Genomic position data (chromosome, position)
- Gene cross-references (via gene_id)
- Gene symbol text search
- Allele frequency data
- Clinical significance
- Variant type classification
- Functional annotation flags (NSM, NSN, SYN, splice sites, etc.)
- Variant origin filtering (SAO values)
- Common variant flag (is_common)
- Publication flags (has_publication, has_pubmed_ref, has_genotypes)
- Population frequencies (gnomAD, 1000 Genomes)
- gnomAD global frequency field
- Enhanced ClinVar fields (variation_id, accession, review_status, disease info)
- PubMed IDs from PMID field
- Merged rs IDs from OLD field
- Gene locus (cytogenetic location)
- HGVS nomenclature (MANE Select and all transcripts)
- RefSeq transcript cross-references (via HGVS annotations)
- AlphaMissense cross-references (coordinate-based, coding SNVs only)

## Performance

- **Test Build**: ~15s (50,000 variants from chr1 only)
- **Data Source**: VCF file from NCBI FTP (GCF_000001405.40.gz)
- **Full Build**: Several hours (depends on chromosome selection)
- **Total Variants**: 1+ billion across all releases
- **Test Mode**: chr1 only, limited to 50,000 entries
- **Production Mode**: All chromosomes (1-22, X, Y, MT)
- **Test Database Size**: ~10 MB (50,000 variants)
- **HGVS Mapper**: Loads 19,394 genes, 67,331 transcripts (~77MB GFF3 file)

## Known Limitations

**No Deduplication Needed**:
- NCBI VCF files are already deduplicated at source
- No in-memory dedup map (saves 50-100GB memory)
- If duplicates exist, addProp3() upserts automatically

**Chromosome Filtering in Test Mode**:
- Test mode only processes chr1 (NC_000001.11)
- Production mode processes all chromosomes
- Filtering happens during parsing for memory efficiency

**Large INFO Fields**:
- Some SNPs have MB-sized INFO fields
- Parser optimized to extract only needed fields (targetFields whitelist)
- Streaming parse avoids strings.Split() memory allocation

**RefSeq Accession Format**:
- Input: NC_000001.11 (RefSeq accession)
- Normalized: 1 (simple chromosome name)
- Special cases: NC_000023 -> X, NC_000024 -> Y, NC_012920 -> MT

**HGVS Limitations**:
- Only computes c. (coding) notation, not p. (protein) notation yet
- Requires RefSeq GFF3 file to be downloaded/cached
- Variants outside annotated transcripts have no HGVS annotation
- Multi-allelic variants use first alternate allele for HGVS

**ID Validation**:
- PubMed IDs must start with a digit (numeric validation)
- ClinVar variation IDs must start with a digit (numeric validation)
- Invalid IDs (e.g., ".,") are skipped during cross-reference creation

**AlphaMissense Cross-Reference Filtering**:
- Only coding SNVs create AlphaMissense xrefs
- Excluded: intronic (`INT`), 3' UTR (`U3`), 5' UTR (`U5`), 3' region (`R3`), 5' region (`R5`)
- AlphaMissense only contains missense predictions, so non-coding variants would never match
- Reduces unnecessary xrefs and improves query precision

**Version Tracking**:
- dbSNP releases quarterly
- No historical version storage
- Build ID tracked per variant (build_id field)

## Future Work

- Add p. (protein) notation to HGVS annotations
- Add consequence prediction (VEP/SnpEff integration)
- Add conservation scores (PhyloP, GERP++)
- dbNSFP enrichment (CADD, SIFT, PolyPhen scores)
- COSMIC somatic mutation links
- Cross-reference validation (SNP -> gene linkage accuracy)
- Paralog-specific test cases
- Performance benchmarks for full chromosome builds

## Maintenance

- **Release Schedule**: Quarterly (dbSNP builds)
- **Current Build**: dbSNP Build 157 (December 2024)
- **Assembly**: GRCh38.p14
- **Data Format**: VCF 4.2
- **Test Data**: 50,000 variants from chr1
- **License**: Public domain (US government work)
- **FTP**: ftp://ftp.ncbi.nlm.nih.gov/snp/latest_release/VCF/

## References

- **Citation**: Sherry ST, et al. (2001) dbSNP: the NCBI database of genetic variation. Nucleic Acids Res. 29(1):308-311.
- **Website**: https://www.ncbi.nlm.nih.gov/snp/
- **FTP**: ftp://ftp.ncbi.nlm.nih.gov/snp/
- **Documentation**: https://www.ncbi.nlm.nih.gov/snp/docs/
- **VCF Format**: https://samtools.github.io/hts-specs/VCFv4.2.pdf
- **HGVS Nomenclature**: https://varnomen.hgvs.org/
- **MANE Project**: https://www.ncbi.nlm.nih.gov/refseq/MANE/
- **License**: Public domain
