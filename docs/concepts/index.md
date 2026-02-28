# Core Concepts

## What is Biobtree?

Biobtree is a data integration platform that:

1. **Aggregates** data from 70+ biological databases
2. **Indexes** entries into a local LMDB database
3. **Maps** cross-references between databases
4. **Queries** via intuitive chain syntax

## Data Model

### Entries

Each entry has:
- **ID**: Primary identifier (e.g., `P04637`)
- **Dataset**: Source database (e.g., `uniprot`)
- **Attributes**: Dataset-specific fields (name, sequence, etc.)
- **Cross-references**: Links to entries in other datasets

### Cross-References (Xrefs)

Xrefs connect entries across datasets:

```
P04637 (uniprot) ←→ ENSG00000141510 (ensembl)
P04637 (uniprot) ←→ GO:0006915 (go)
P04637 (uniprot) ←→ CHEMBL203 (chembl_target)
```

Xrefs are bidirectional - biobtree automatically creates reverse mappings.

## Query Model

### Chain Syntax

Use `>>` to traverse datasets:

```
identifier >> dataset1 >> dataset2
```

**First `>>`** = Lookup (find the identifier)
**Subsequent `>>`** = Map (follow cross-references)

### Examples

```bash
# Gene symbol → Gene → Protein
TP53 >> ensembl >> uniprot

# Protein → Drug targets → Drugs
P04637 >> chembl_target >> chembl_molecule

# Disease → Genes → Proteins
"breast cancer" >> mondo >> gencc >> hgnc >> ensembl >> uniprot
```

### Filters

Apply CEL-based filters at any step:

```bash
# Boolean filter
>> uniprot[reviewed==true]

# Numeric filter
>> pdb[resolution<2.0]

# String filter
>> go[type=="biological_process"]
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     DATA SOURCES                             │
│   UniProt  Ensembl  ChEMBL  PubChem  ClinVar  GO  ...       │
└───────────────────────────┬─────────────────────────────────┘
                            ↓
┌───────────────────────────────────────────────────────────────┐
│                       UPDATE PHASE                            │
│   Download → Parse → Extract entries & xrefs → Bucket files   │
└───────────────────────────┬───────────────────────────────────┘
                            ↓
┌───────────────────────────────────────────────────────────────┐
│                       GENERATE PHASE                          │
│   Sort buckets → K-way merge → Build LMDB database            │
└───────────────────────────┬───────────────────────────────────┘
                            ↓
┌───────────────────────────────────────────────────────────────┐
│                       WEB SERVICE                             │
│   REST API   │   gRPC   │   Web UI   │   CLI                  │
└───────────────────────────────────────────────────────────────┘
```

## Response Modes

- **Full mode**: Complete data with all attributes (default)
- **Lite mode**: Compact IDs-only format (~50x smaller)

Lite mode is optimized for AI agents and bulk operations.

## See Also

- [Query Syntax](../api/query-syntax.md) - Detailed query reference
- [Datasets](../datasets/index.md) - All supported databases
- [Technical Internals](../internals/) - Architecture deep-dives
