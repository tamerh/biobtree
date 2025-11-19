# dbSNP - Single Nucleotide Polymorphism Database

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
- `allele_frequency`: Global allele frequency from 1000 Genomes
- `gnomad_frequency`: gnomAD population frequency (reserved for future use)

#### Gene Context
- `gene_names`: Gene symbols associated with variant (array, e.g., ["BRCA1", "TP53"])
- `gene_ids`: Entrez Gene IDs (array)
- `pseudogene_names`: Pseudogene symbols (array)
- `pseudogene_ids`: Pseudogene IDs (array)
- `gene_locus`: Cytogenetic location (e.g., "17q21.31")

#### Variant Origin & Quality
- `sao`: Variant Allele Origin
  - 0 = unspecified
  - 1 = Germline (inherited variation)
  - 2 = Somatic (cancer/tumor variants)
  - 3 = Both germline and somatic
- `is_common`: Common SNP flag (MAF ≥ 1% in 1000 Genomes populations)
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
- `pubmed_ids`: PubMed citation IDs (array)

#### Clinical & Historical Data
- `clinical_significance`: Clinical significance from ClinVar
- `merged_rs_ids`: Historical merged rs IDs (array)

### Cross-References

**Gene Associations**:
- **NCBI Gene (EntrezGene)**: Via gene_ids field for direct gene lookup
- **Ensembl Genes**: Via gene symbol lookup using addXrefViaGeneSymbol()
  - Handles paralogs by creating xrefs to all matching Ensembl genes
  - Uses chromosome information for context
  - Example: "BRCA1" search → Ensembl gene → "BRCA1 >> dbsnp" → all SNPs

**Text Search**:
- rs IDs indexed as keywords (direct lookup: "rs7903146")
- Gene symbols indexed (symbol search: "TCF7L2" finds associated SNPs)

### Special Features

**Gene Symbol to Ensembl Mapping**:
- Gene symbols from GENEINFO and PSEUDOGENEINFO fields create bidirectional links
- Uses `addXrefViaGeneSymbol()` for paralog-aware mapping
- Creates xrefs to ALL matching Ensembl genes (deterministic principle)
- Example workflow:
  1. Search "DDX11L16" → finds Ensembl gene(s)
  2. Query "DDX11L16 >> dbsnp" → returns all SNPs in that gene
  3. For paralogs, returns SNPs from all chromosome copies

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
Query: rs7903146 → Retrieve all attributes
Use: Get comprehensive information about a known variant
```

**2. Gene to Variants Mapping**
```
Query: TCF7L2 >> dbsnp → All SNPs in TCF7L2 gene
Use: Find all genetic variation in a gene of interest
```

**3. Functional Impact Filtering**
```
Query: All SNPs → Filter nsm=true OR nsn=true → Protein-changing variants
Use: Focus on variants likely to affect protein function
```

**4. Common vs Rare Variants**
```
Query: All SNPs → Filter is_common=true → Common variants (MAF ≥ 1%)
Query: All SNPs → Filter is_common=false → Rare variants
Use: Population genetics and rare disease studies
```

**5. Germline vs Somatic Filtering**
```
Query: All SNPs → Filter sao=1 → Germline variants only
Query: All SNPs → Filter sao=2 → Somatic/cancer variants only
Use: Separate inherited variation from cancer mutations
```

**6. Quality Filtering**
```
Query: All SNPs → Filter ssr=0 → High-quality variants only
Query: All SNPs → Filter has_publication=true → Well-studied variants
Use: Remove suspect variants, focus on validated SNPs
```

**7. Splice Site Variants**
```
Query: All SNPs → Filter ass=true OR dss=true → Splice site variants
Use: Identify variants affecting RNA splicing
```

**8. Cross-Database Integration**
```
Query: SNP → xrefs → Ensembl genes, ClinVar entries, GWAS associations
Use: Link genomic data with genes, diseases, and traits
```

## Test Cases

**Current Tests** (10 total):
- 4 declarative tests (JSON-based)
- 6 custom tests (Python logic)

**Coverage**:
- ✅ Basic rs ID lookup
- ✅ Attribute presence validation
- ✅ Multiple ID batch lookup (5 SNPs)
- ✅ Invalid ID handling
- ✅ Genomic position data (chromosome, position)
- ✅ Gene cross-references (via gene_id)
- ✅ Gene symbol text search
- ✅ Allele frequency data
- ✅ Clinical significance
- ✅ Variant type classification

**Recommended Additions**:
- Test functional annotation flags (NSM, NSN, SYN, splice sites)
- Test variant origin filtering (SAO values)
- Test common variant flag (is_common)
- Test quality filtering (SSR codes)
- Test publication flags (has_publication, has_pubmed_ref)
- Test paralog handling (genes on multiple chromosomes)
- Test pseudogene associations
- Validate chromosome normalization (NC_000001.11 → 1)
- Test variant type determination (SNV, insertion, deletion, MNV)

## Performance

- **Test Build**: ~30s (10,000 variants from chr1 only)
- **Data Source**: VCF file from NCBI FTP (GCF_000001405.40.gz)
- **Full Build**: Several hours (depends on chromosome selection)
- **Total Variants**: 1+ billion across all releases
- **Test Mode**: chr1 only, limited to 10,000 entries
- **Production Mode**: All chromosomes (1-22, X, Y, MT)
- **Test Database Size**: ~5 MB (10,000 variants)

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
- Special cases: NC_000023 → X, NC_000024 → Y, NC_012920 → MT

**Fields NOT Included**:
- **FREQ**: Complex allele frequency list from multiple studies
  - Would require special parsing and storage structure
  - Current AF field provides 1000 Genomes global frequency
- **ClinVar-specific INFO fields**: CLNHGVS, CLNVI, CLNORIGIN, CLNDISDB, CLNDN, CLNREVSTAT, CLNACC
  - Redundant with separate ClinVar dataset
  - Avoiding data duplication across datasets

**Version Tracking**:
- dbSNP releases quarterly
- No historical version storage
- Build ID tracked per variant (build_id field)

## Future Work

- Add gnomAD frequency integration (gnomad_frequency field reserved)
- Implement FREQ field parsing for multi-study frequency data
- Add consequence prediction (VEP/SnpEff integration)
- Add conservation scores (PhyloP, GERP++)
- Add population-specific frequencies (AFR, EUR, EAS, SAS, AMR)
- Test coverage for all functional annotation flags
- Cross-reference validation (SNP → gene linkage accuracy)
- Paralog-specific test cases
- Performance benchmarks for full chromosome builds

## Maintenance

- **Release Schedule**: Quarterly (dbSNP builds)
- **Current Build**: dbSNP Build 157 (December 2024)
- **Assembly**: GRCh38.p14
- **Data Format**: VCF 4.2
- **Test Data**: 10,000 variants from chr1
- **License**: Public domain (US government work)
- **FTP**: ftp://ftp.ncbi.nlm.nih.gov/snp/latest_release/VCF/

## References

- **Citation**: Sherry ST, et al. (2001) dbSNP: the NCBI database of genetic variation. Nucleic Acids Res. 29(1):308-311.
- **Website**: https://www.ncbi.nlm.nih.gov/snp/
- **FTP**: ftp://ftp.ncbi.nlm.nih.gov/snp/
- **Documentation**: https://www.ncbi.nlm.nih.gov/snp/docs/
- **VCF Format**: https://samtools.github.io/hts-specs/VCFv4.2.pdf
- **License**: Public domain
