# RefSeq Dataset

## Overview
NCBI RefSeq (Reference Sequence) provides curated, non-redundant reference sequences for genomes, transcripts, and proteins. It serves as the gold standard for sequence annotation with consistent nomenclature across species. RefSeq sequences are distinguished by their accession prefixes (NM_, NP_, NC_, etc.) and undergo rigorous curation.

**Source**: NCBI (National Center for Biotechnology Information)
**Data Type**: Curated reference sequences with genomic coordinates, gene annotations, and cross-references

## Accession Prefixes

| Prefix | Type | Description |
|--------|------|-------------|
| NM_ | mRNA | Curated protein-coding transcripts |
| NR_ | ncRNA | Curated non-coding RNA |
| NP_ | protein | Curated proteins |
| NC_ | genomic | Complete genomic molecules (chromosomes, plasmids) |
| NG_ | genomic | Genomic regions (gene clusters, segments) |
| NT_ | genomic | Contigs |
| NW_ | genomic | WGS scaffolds |
| XM_ | predicted_mRNA | Model/predicted mRNA |
| XR_ | predicted_ncRNA | Model/predicted ncRNA |
| XP_ | predicted_protein | Model/predicted protein |
| WP_ | protein | Non-redundant proteins (across prokaryotes) |
| YP_ | protein | Organelle-encoded proteins |

## Integration Architecture

### Storage Model
**Primary Entries**: RefSeq accessions with version (e.g., "NM_001353961.2", "NP_001340890.1")
**Searchable Text Links**: Gene symbols, synonyms
**Attributes Stored**: accession, type, status, symbol, description, synonyms, chromosome, start_position, end_position, orientation, genomic_accession, protein_accession, rna_accession, is_mane_select, is_mane_plus_clinical, ensembl_transcript, ensembl_protein, hgnc_id, ccds_id, uniprot_id, protein_length, molecular_weight, protein_name, organism, seq_length, mol_type, exon_count, taxid
**Cross-References**: Entrez Gene, Taxonomy, Ensembl (via MANE), UniProt, CCDS, internal RefSeq links (transcript ↔ protein)

### Data Processing
RefSeq data is processed from:
1. **assembly_summary.txt** - Maps taxonomy IDs to assembly FTP paths
2. **_rna.gbff.gz** - RNA/transcript annotations (GBFF format)
3. **_protein.gpff.gz** - Protein annotations (GPFF format)
4. **_genomic.gbff.gz** - Genomic positions (for coordinate enrichment)
5. **MANE summary** - MANE Select/Plus Clinical annotations (human only)

### Multi-Species Support
Use `--genome-taxids` to filter to specific organisms:
```bash
# Human only
./biobtree --genome-taxids 9606 -d "refseq" build

# Model organisms
./biobtree --genome-taxids 9606,10090,10116,7955 -d "refseq" build
```

**Note**: Not all genomes have separate RNA annotation files (especially prokaryotes). Missing RNA files are silently skipped - protein files contain the essential data.

## MANE Annotations (Human)
MANE (Matched Annotation from NCBI and EBI) provides gold-standard transcript annotations:
- **MANE Select**: One representative transcript per protein-coding gene
- **MANE Plus Clinical**: Additional clinically relevant transcripts

MANE annotations include matched Ensembl transcript/protein IDs for seamless cross-database queries.

## Use Cases

**1. Transcript Lookup**
```
Query: NM_007294 -> BRCA1 transcript with genomic coordinates
Use: Gene structure analysis, primer design, variant annotation
```

**2. Protein Information**
```
Query: NP_005219 -> EGFR protein with length, molecular weight
Use: Protein characterization, structural analysis
```

**3. Transcript-Protein Mapping**
```
Query: NM_007294 -> NP_009225 (protein accession)
Use: Linking transcript variants to protein products
```

**4. Cross-Database Integration**
```
Query: NM_007294 >> ensembl -> ENST00000357654
Use: Comparing annotations between RefSeq and Ensembl
```

**5. MANE Transcript Identification**
```
Query: BRCA1 transcripts -> Filter is_mane_select=true
Use: Selecting canonical transcripts for clinical analysis
```

**6. Multi-Species Analysis**
```
Build with: --genome-taxids 9606,10090
Query: Compare human and mouse orthologs via RefSeq
Use: Comparative genomics, model organism studies
```

## Test Cases

**Current Tests** (13 total):
- 5 declarative tests (ID lookup, attribute validation, cross-references)
- 8 custom tests:
  - Gene symbol validation
  - Description check
  - Organism info
  - RefSeq status (VALIDATED, REVIEWED, etc.)
  - Cross-reference presence
  - Entrez Gene reference
  - Taxonomy reference
  - Type classification (mRNA, protein, etc.)

**Coverage**:
- Accession lookup
- Gene symbol search
- Attribute validation (status, description, organism)
- Cross-reference validation (Entrez, Taxonomy)
- Type classification by prefix

**Recommended Additions**:
- MANE Select/Plus Clinical validation
- Ensembl cross-reference tests
- UniProt cross-reference tests
- Genomic position validation
- Protein-transcript linkage tests

## Performance

- **Test Build**: ~30-60s (100 entries per taxid)
- **Data Source**: NCBI FTP (assembly_summary.txt + GBFF/GPFF files)
- **Update Frequency**: Continuous at NCBI
- **Total Entries**: Varies by organism selection
- **Special Notes**:
  - Uses `--genome-taxids` for species filtering
  - Downloads assembly-specific files
  - Prokaryotic genomes may lack separate RNA files

## Known Limitations

- **RNA file availability**: Many prokaryotic genomes don't have separate `_rna.gbff.gz` files; RNA annotations are in genomic GBFF only
- **MANE data**: Only available for human (taxid 9606)
- **Genomic positions**: Require genomic GBFF processing for non-MANE entries
- **Assembly selection**: Processes all assemblies for a taxid (may include multiple strains)
- **Large downloads**: Full RefSeq for all species is very large; use `--genome-taxids` to limit

## Configuration

RefSeq uses these configuration options in `conf/source.dataset.json`:
```json
{
  "refseq": {
    "path": "ftp://ftp.ncbi.nlm.nih.gov/genomes/refseq/",
    "speciesGroups": "vertebrate_mammalian,vertebrate_other,invertebrate,plant,fungi,protozoa,bacteria,archaea",
    "manePath": "ftp://ftp.ncbi.nlm.nih.gov/refseq/MANE/MANE_human/current/MANE.GRCh38.v1.3.summary.txt.gz"
  }
}
```

## Future Work

- Add MANE Select/Plus Clinical test validation
- Add Ensembl cross-reference tests (via MANE)
- Add UniProt cross-reference tests
- Validate genomic position accuracy
- Add protein-transcript bidirectional link tests
- Test WP_ (non-redundant protein) entries
- Add chromosome/genomic accession lookup tests

## Maintenance

- **Release Schedule**: Continuous updates at NCBI
- **Data Format**: GBFF (GenBank Flat File) / GPFF (GenPept Flat File)
- **Test Data**: Human RefSeq entries (well-characterized genes)
- **License**: Public domain (NCBI data)

## References

- **Citation**: O'Leary NA, et al. Reference sequence (RefSeq) database at NCBI: current status, taxonomic expansion, and functional annotation. Nucleic Acids Res. 2016.
- **Website**: https://www.ncbi.nlm.nih.gov/refseq/
- **MANE**: https://www.ncbi.nlm.nih.gov/refseq/MANE/
- **License**: Public domain
