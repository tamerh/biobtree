# Dataset Reference

Biobtree integrates **73 datasets** across major biological domains. Each dataset has detailed documentation including storage model, use cases, and query examples.

---

## Genomics & Genes {#genomics}

Genes, transcripts, variants, and genomic coordinates.

| Dataset | Description | Documentation |
|---------|-------------|---------------|
| ensembl | Ensembl genomes (main + bacteria, fungi, metazoa, plants, protists) | [README](ensembl.md) |
| hgnc | HUGO Gene Nomenclature Committee | [README](hgnc.md) |
| entrez | NCBI Entrez Gene | [README](entrez.md) |
| refseq | NCBI Reference Sequences | [README](refseq.md) |
| dbsnp | NCBI dbSNP variants | [README](dbsnp.md) |
| encode_ccre | ENCODE cis-Regulatory Elements | [README](encode_ccre.md) |

---

## Proteins & Structure {#proteins}

Protein sequences, structures, domains, and features.

| Dataset | Description | Documentation |
|---------|-------------|---------------|
| uniprot | UniProt protein sequences and annotations | [README](uniprot.md) |
| uniparc | UniProt Archive (sequence clusters) | [README](uniparc.md) |
| uniref50 | UniRef 50% identity clusters | [README](uniref50.md) |
| uniref90 | UniRef 90% identity clusters | [README](uniref90.md) |
| uniref100 | UniRef 100% identity clusters | [README](uniref100.md) |
| alphafold | AlphaFold predicted structures | [README](alphafold.md) |
| alphamissense | AlphaMissense pathogenicity predictions | [README](alphamissense.md) |
| alphamissense_transcript | Transcript-level pathogenicity | [README](alphamissense_transcript.md) |
| pdb | Protein Data Bank structures | [README](pdb.md) |
| interpro | Protein domains and families | [README](interpro.md) |
| esm2_similarity | ESM2 embedding-based protein similarity | [README](esm2_similarity.md) |
| diamond_similarity | DIAMOND sequence-based protein similarity | [README](diamond_similarity.md) |
| jaspar | TF binding profiles (PFMs/PWMs) | [README](jaspar.md) |
| antibody | Antibody sequences | [README](antibody.md) |

---

## Chemistry & Drugs {#chemistry}

Chemical compounds, drugs, binding affinity, and metabolism.

| Dataset | Description | Documentation |
|---------|-------------|---------------|
| chembl | ChEMBL drug discovery database | [README](chembl.md) |
| chebi | Chemical Entities of Biological Interest | [README](chebi.md) |
| pubchem | PubChem chemical compounds | [README](pubchem.md) |
| pubchem_activity | PubChem bioactivity data | [README](pubchem_activity.md) |
| hmdb | Human Metabolome Database | [README](hmdb.md) |
| bindingdb | Binding affinity measurements | [README](bindingdb.md) |
| swisslipids | SwissLipids lipid structures | [README](swisslipids.md) |
| lipidmaps | LIPID MAPS lipid classification | [README](lipidmaps.md) |
| rhea | Rhea biochemical reactions | [README](rhea.md) |
| brenda | BRENDA enzyme database | [README](brenda.md) |
| patent | SureChEMBL patent compounds | [README](patent.md) |

---

## Pathways & Interactions {#pathways}

Biological pathways, protein-protein interactions, signaling networks.

| Dataset | Description | Documentation |
|---------|-------------|---------------|
| reactome | Reactome pathways | [README](reactome.md) |
| signor | SIGNOR causal signaling networks | [README](signor.md) |
| intact | IntAct protein interactions | [README](intact.md) |
| string | STRING protein networks | [README](string.md) |
| biogrid | BioGRID genetic interactions | [README](biogrid.md) |
| collectri | CollecTRI TF-target regulation | [README](collectri.md) |
| cellphonedb | CellPhoneDB ligand-receptor | [README](cellphonedb.md) |
| corum | CORUM protein complexes | [README](corum.md) |

---

## Disease & Phenotype {#disease}

Disease associations, clinical variants, rare diseases.

| Dataset | Description | Documentation |
|---------|-------------|---------------|
| clinvar | ClinVar clinical variants | [README](clinvar.md) |
| mondo | MONDO disease ontology | [README](mondo.md) |
| hpo | Human Phenotype Ontology | [README](hpo.md) |
| orphanet | Orphanet rare diseases | [README](orphanet.md) |
| gwas | GWAS Catalog associations | [README](gwas.md) |
| gwas_study | GWAS Catalog studies | [README](gwas_study.md) |
| gencc | Gene-disease validity curations | [README](gencc.md) |
| clinical_trials | ClinicalTrials.gov | [README](clinical_trials.md) |
| pharmgkb | PharmGKB pharmacogenomics | [README](pharmgkb.md) |

---

## Ontologies {#ontologies}

Controlled vocabularies and hierarchical classifications.

| Dataset | Description | Documentation |
|---------|-------------|---------------|
| go | Gene Ontology (BP, MF, CC) | [README](go.md) |
| efo | Experimental Factor Ontology | [README](efo.md) |
| eco | Evidence & Conclusion Ontology | [README](eco.md) |
| uberon | UBERON anatomy ontology | [README](uberon.md) |
| cl | Cell Ontology | [README](cl.md) |
| mesh | MeSH medical subject headings | [README](mesh.md) |
| bao | BioAssay Ontology | [README](bao.md) |
| oba | Ontology of Biological Attributes | [README](oba.md) |
| obi | Ontology for Biomedical Investigations | [README](obi.md) |
| pato | Phenotype And Trait Ontology | [README](pato.md) |
| xco | Experimental Conditions Ontology | [README](xco.md) |

---

## Expression & Single-Cell {#expression}

Gene expression, tissue specificity, single-cell data.

| Dataset | Description | Documentation |
|---------|-------------|---------------|
| bgee | Bgee gene expression | [README](bgee.md) |
| cellxgene | CELLxGENE Census single-cell | [README](cellxgene.md) |
| scxa | Single Cell Expression Atlas | [README](scxa.md) |
| fantom5 | FANTOM5 CAGE expression | [README](fantom5.md) |
| mirdb | miRDB microRNA targets | [README](mirdb.md) |
| rnacentral | RNACentral non-coding RNAs | [README](rnacentral.md) |

---

## Taxonomy & Other {#other}

Taxonomy, toxicogenomics, gene sets, and specialized datasets.

| Dataset | Description | Documentation |
|---------|-------------|---------------|
| taxonomy | NCBI Taxonomy | [README](taxonomy.md) |
| ctd | CTD toxicogenomics | [README](ctd.md) |
| msigdb | MSigDB gene sets | [README](msigdb.md) |

---

## Dataset Connectivity (EDGES)

Each dataset connects to others via cross-references. Key connections:

```
ensembl: uniprot, go, transcript, exon, ortholog, paralog, hgnc, entrez, refseq, bgee, gwas, gencc
uniprot: ensembl, alphafold, interpro, pdb, go, reactome, chembl_target, string, intact, biogrid
chembl_molecule: mesh, chembl_target, pubchem, chebi, clinical_trials
clinvar: hgnc, mondo, hpo, dbsnp, orphanet
go: ensembl, uniprot, reactome, msigdb, interpro
```

See [Edge Reference](../api/edge-reference.md) for complete connectivity map.

---

## Build Recipes

### Full Production Build

```bash
./bb.sh                       # Update all datasets
./bb.sh --generate            # Build database
./bb.sh --activate            # Activate new version
```

### Selective Updates

```bash
# Update specific datasets only
./bb.sh --only chembl,uniprot,pdb

# Resume from a dataset
./bb.sh --from pubchem
```

### Development/Testing

```bash
# Build test database with limited data
biobtree -d "uniprot,chembl,ensembl" test
```
