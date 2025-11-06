# ChEBI (Chemical Entities of Biological Interest) Dataset

## Overview

ChEBI is a freely available dictionary of molecular entities focused on small chemical compounds. Provides comprehensive chemical structure information, nomenclature, and classification for biologically relevant molecules including metabolites, drugs, enzyme inhibitors, and toxins.

**Source**: ChEBI database_accession.tsv file (FTP download)
**Data Type**: **Cross-reference-only dataset** (special integration pattern)

## Integration Architecture

### Storage Model - Special Cross-Reference Pattern

**IMPORTANT**: ChEBI has a unique integration pattern in biobtree. Unlike other datasets that store primary searchable entries, ChEBI operates as a **cross-reference-only dataset**:

**Normal Datasets** (e.g., UniProt, GO, HGNC):
```
Entry ID → Searchable entry with attributes → Cross-references to other datasets
```

**ChEBI Pattern**:
```
ChEBI ID → ONLY cross-references → Stored ON target database entries
```

### How It Works

**Data Processing**:
1. Parser reads `database_accession.tsv` containing mappings:
   ```
   CHEBI:X → target_database → target_id
   ```
2. For each ChEBI ID, creates cross-references FROM ChEBI TO other databases
3. These are stored as **inverse xrefs** ON the target database entries
4. No primary ChEBI entries are created in biobtree

**What This Means**:
- ✅ ChEBI IDs (e.g., `CHEBI:15377`) **ARE searchable** via bidirectional xrefs
- ✅ ChEBI cross-references **ARE stored** on target entries (UniProt, GO, etc.)
- ✅ Query ChEBI ID → See which proteins/GO terms reference that compound
- ✅ Query UniProt entry → See ChEBI xrefs for that protein's small molecule interactions
- ✅ Query GO term → See ChEBI xrefs for related chemical entities
- ❌ But NO compound metadata stored (names, structures, formulas)

**Cross-References Stored**:
- ChEBI → UniProt (proteins that interact with compounds)
- ChEBI → GO (Gene Ontology terms related to compounds)
- ChEBI → Other databases where compound references exist

**Why This Pattern?**:
- Current implementation focuses on cross-reference mappings only
- Avoids duplicating chemical structure data
- ChEBI IDs searchable via bidirectional xrefs, but no compound metadata available
- **See Future Work section for planned full ChEBI integration**

### Special Features

**Inverse Cross-Reference Architecture**:
- ChEBI mappings enriched target database entries with chemical compound information
- Enables queries like: UniProt protein → Get related ChEBI compounds

**No Attributes Stored**:
- ChEBI only provides cross-reference mappings
- Chemical names, structures, formulas not stored in biobtree
- Users should query ChEBI directly for compound details

## Use Cases

**1. Protein-Compound Interaction Discovery**
```
Query: UniProt entry (with x=true) → Get ChEBI xrefs → Identify interacting compounds
Use: Drug target analysis, metabolic pathway research
```

**2. GO Term Chemical Associations**
```
Query: GO term (with x=true) → Get ChEBI xrefs → Find related chemical entities
Use: Understanding molecular function in chemical context
```

**3. Cross-Database Compound Linking**
```
Query: Biological entry → ChEBI xref → Use ChEBI ID to query PubChem/ChEMBL
Use: Connect biobtree data to external chemical databases
```

**4. Metabolic Network Construction**
```
Query: Multiple proteins → Aggregate ChEBI xrefs → Build compound interaction networks
Use: Systems biology, metabolic pathway analysis
```

**5. Drug Target Validation**
```
Query: Disease-related proteins → ChEBI xrefs → Identify known drug compounds
Use: Drug repurposing, target validation studies
```

## Test Cases

**Current Tests** (4 total):
- All custom tests (no declarative tests due to special architecture)

**Coverage**:
- ✅ ChEBI ID processing verification (100 IDs logged)
- ✅ Cross-reference-only structure documentation
- ✅ Non-searchability validation (confirms ChEBI IDs not primary entries)
- ✅ Sample xref detection in target databases

**Test Strategy**:
- Tests focus on verifying ChEBI processing, not entry lookup
- Validates that ChEBI IDs were processed during build
- Confirms expected behavior (non-searchability of ChEBI IDs)
- Attempts to find ChEBI xrefs in other dataset entries (informational)

**Why Different Tests?**:
- Cannot use standard ID lookup tests (ChEBI IDs not searchable)
- Cannot use attribute tests (no attributes stored)
- Tests verify processing and cross-reference architecture instead

## Performance

- **Test Build**: ~0.25s (100 ChEBI IDs processed)
- **Data Source**: TSV file from ChEBI FTP server
- **Update Frequency**: Monthly releases from ChEBI
- **Total Compounds**: ~200,000+ chemical entities with database cross-references
- **No searchable entries created**: All data stored as xrefs on target entries

## Known Limitations

**Limited ChEBI Metadata**:
- ChEBI IDs are searchable via bidirectional xrefs
- But no compound attributes stored (names, structures, formulas, properties)
- For compound details, must query ChEBI website directly

**Limited Test Validation**:
- Test database may not have entries with ChEBI xrefs (depends on which entries selected)
- Cross-reference validation is informational only
- Cannot comprehensively test xref storage without full multi-dataset build

**No Compound Metadata**:
- Chemical names, structures, formulas, properties not stored
- Only cross-reference mappings maintained
- biobtree acts as linking layer, not chemical database

**Target Database Dependency**:
- ChEBI xrefs only useful if target databases (UniProt, GO) are in build
- Isolated ChEBI build creates no searchable entries

## Future Work

### 🔴 **PRIORITY: Full ChEBI Integration**

**Current Limitation**: ChEBI only stores cross-references without compound metadata. While ChEBI IDs are searchable via bidirectional xrefs, queries return no chemical information.

**Proposed Enhancement**: Implement proper ChEBI dataset integration:

1. **Parse ChEBI OBO/XML files** (in addition to database_accession.tsv)
   - Extract compound names, synonyms, formulas, InChI, SMILES
   - Store molecular weight, charge, monoisotopic mass
   - Include ChEBI ontology classifications (role, application, subclass)

2. **Create Primary ChEBI Entries** with attributes:
   - Searchable by ChEBI ID, compound name, synonyms
   - Chemical formulas and identifiers as text keywords
   - Ontology relationships (parent-child in chemical classification)

3. **Maintain Cross-References** (existing functionality):
   - Keep bidirectional xrefs to UniProt, GO, etc.
   - Enable queries: Compound → Proteins/Pathways/Functions

4. **Benefits of Full Integration**:
   - Direct compound lookup with full chemical details
   - Synonym-based chemical search
   - Chemical classification browsing
   - Complete compound information without external queries
   - Better integration with metabolic and drug discovery workflows

**Impact**: Transforms ChEBI from cross-reference-only to fully integrated chemical database, making biobtree a comprehensive biochemical knowledge base.

---

### Other Future Work

- Add comprehensive multi-dataset test (ChEBI + UniProt + GO) to validate xref storage
- Document which target databases receive ChEBI xrefs
- Add statistics on ChEBI xref distribution across datasets

## Maintenance

- **Release Schedule**: Monthly updates from ChEBI
- **Data Format**: TSV (tab-separated values) - stable format
- **Test Data**: Processes 100 ChEBI IDs to verify parser functionality
- **Special Testing**: ID logging used instead of entry retrieval

## References

- **Citation**: Hastings J et al. The ChEBI reference database and ontology for biologically relevant chemistry. Nucleic Acids Research.
- **Website**: https://www.ebi.ac.uk/chebi/
- **License**: CC BY 4.0 (freely available)
