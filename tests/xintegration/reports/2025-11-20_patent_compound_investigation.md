# Patent Compound Investigation Report

**Date:** 2025-11-20
**Issue:** InChI/SMILES keyword searches failing with "Entry not found identifier 29384284 dataset patent_compound"

## Problem Summary

Keyword searches for InChI keys and SMILES strings fail when patent data is included in the database, even when specifying `d=chembl_molecule`:

```bash
curl "http://scc2:9292/ws/?i=SIXVRXARNAVBTC-UHFFFAOYSA-N&d=chembl_molecule"
# Error: "Entry not found identifier 29384284 dataset patent_compound"
```

## Root Cause Analysis

### Current patents.go Implementation

**processCompounds() - Lines 248-256:**
```go
// InChI Key → Patent Compound (link - will auto-connect to ChEMBL via linkdataset)
if inchiKey != "" {
    p.d.addXref(inchiKey, textLinkID, compoundID, "patent_compound", true)
}

// SMILES → Patent Compound (link)
if smiles != "" {
    p.d.addXref(smiles, textLinkID, compoundID, "patent_compound", true)
}
```

**processMappings() - Line 327:**
```go
// Patent ↔ Patent Compound (bidirectional)
p.d.addXref(patentNumber, fr, compoundID, "patent_compound", false)
```

### What's Missing

1. **No patent_compound entries created** - only xrefs
2. **No xrefs to chembl_molecule** - linkdataset mechanism can't work
3. **Keyword xrefs block ChEMBL results** - search fails before trying ChEMBL

### How Keyword Search Works (service.go:672-821)

When searching for InChI key:

1. **Line 705:** Gets all xrefs for the keyword (both patent_compound AND chembl_molecule)
2. **Line 736:** For each xref, calls `getLmdbResult2(identifier, dataset)` to fetch the entry
3. **Line 1060 (getLmdbResult2):** Tries to fetch entry "29384284" from database
4. **Line 1091-1092:** Entry doesn't exist → **throws error and stops**
5. **Never reaches ChEMBL xrefs** - error prevents further processing

Even with `d=chembl_molecule`, the patent_compound xref is processed first and fails.

### Verification

```bash
# Patent compound entry doesn't exist
curl "http://scc2:9292/ws/?i=29384284"
# Returns: {"message": "No results found"}

# ChEMBL molecule exists and has the InChI key
curl "http://scc2:9292/ws/?i=CHEMBL2171124" | grep inchiKey
# Returns: "inchiKey": "SIXVRXARNAVBTC-UHFFFAOYSA-N"
```

## Linkdataset Mechanism

**Configuration (default.dataset.json):**
```json
"patent_compound": {
  "name": "Patent Compound",
  "id": "352",
  "linkdataset": "chembl_molecule"
}
```

**How it should work:**
- When querying `>>patent_compound>>X`, system auto-injects: `>>chembl_molecule>>X`
- But requires patent_compound entries with xrefs to chembl_molecule

**Current status:**
- Linkdataset can't work because patent_compound entries don't exist

## Solution Pattern (from clinical_trials.go)

Clinical trials uses `d.lookup()` to resolve identifiers during processing:

```go
func (ct *clinicalTrials) lookupAndCollectChEMBL(name string, chemblDatasetID uint32, chemblIDs map[string]bool) {
    result, err := ct.lookup(name)
    if err != nil || result == nil || len(result.Results) == 0 {
        return
    }

    for _, xref := range result.Results {
        if xref.IsLink {
            for _, entry := range xref.Entries {
                if entry.Dataset == chemblDatasetID {
                    chemblIDs[entry.Identifier] = true
                }
            }
        } else if xref.Dataset == chemblDatasetID {
            chemblIDs[xref.Identifier] = true
        }
    }
}
```

## Proposed Fix for patents.go

Modify `processCompounds()` to:

```go
func (p *patents) processCompounds() (int, error) {
    // ... existing setup code ...

    chemblMoleculeDatasetID := config.DataconfStringToInt["chembl_molecule"]
    patentCompoundDatasetID := config.DataconfStringToInt["patent_compound"]

    for j := range parser.Stream() {
        // ... existing code ...

        compoundID := getString(j, "id")
        if compoundID == "" || compoundID == "0" {
            continue
        }

        inchiKey := getString(j, "inchi_key")
        smiles := getString(j, "smiles")

        // Try to find ChEMBL molecule using InChI key or SMILES
        var chemblMoleculeID string

        if inchiKey != "" {
            // Lookup by InChI key
            if result, err := p.d.lookup(inchiKey); err == nil && result != nil {
                chemblMoleculeID = findChEMBLMoleculeID(result, chemblMoleculeDatasetID)
            }
        }

        // If not found by InChI, try SMILES
        if chemblMoleculeID == "" && smiles != "" {
            if result, err := p.d.lookup(smiles); err == nil && result != nil {
                chemblMoleculeID = findChEMBLMoleculeID(result, chemblMoleculeDatasetID)
            }
        }

        // Only create patent_compound entry if we found a ChEMBL match
        if chemblMoleculeID != "" {
            // Create actual patent_compound entry
            p.d.addEntry(compoundID, patentCompoundDatasetID)

            // Create xref: patent_compound → chembl_molecule
            p.d.addXref(compoundID, chemblMoleculeDatasetID, chemblMoleculeID, "chembl_molecule", false)

            // Optional: Create keyword xrefs (may not be needed since ChEMBL has them)
            // if inchiKey != "" {
            //     p.d.addXref(inchiKey, textLinkID, compoundID, "patent_compound", true)
            // }
            // if smiles != "" {
            //     p.d.addXref(smiles, textLinkID, compoundID, "patent_compound", true)
            // }
        }
    }

    return count, nil
}

// Helper function to extract ChEMBL molecule ID from lookup results
func findChEMBLMoleculeID(result *pbuf.Result, chemblDatasetID uint32) string {
    for _, xref := range result.Results {
        if xref.IsLink {
            for _, entry := range xref.Entries {
                if entry.Dataset == chemblDatasetID {
                    return entry.Identifier
                }
            }
        } else if xref.Dataset == chemblDatasetID {
            return xref.Identifier
        }
    }
    return ""
}
```

## Alternative Solutions

### Option 1: Remove Keyword Xrefs (Simpler)
Don't create keyword xrefs to patent_compound at all, since ChEMBL already has them:

```go
// processCompounds() - just create entries and ChEMBL xrefs, NO keyword xrefs
if chemblMoleculeID != "" {
    p.d.addEntry(compoundID, patentCompoundDatasetID)
    p.d.addXref(compoundID, chemblMoleculeDatasetID, chemblMoleculeID, "chembl_molecule", false)
}
```

This way:
- InChI/SMILES searches find ChEMBL molecules directly
- Patent→compound mappings work via processMappings
- Linkdataset mechanism works for patent queries

### Option 2: Fix Search to Skip Missing Entries
Modify `service.go:getLmdbResult2()` to not throw error for missing linkdataset entries:

```go
// Check if this is a linkdataset
datasetName := config.DataconfIDIntToString[domainID]
if _, isLinkDataset := config.Dataconf[datasetName]["linkdataset"]; isLinkDataset {
    // For linkdatasets, missing entries are OK - just skip them
    return nil, nil
}
```

This is a band-aid solution that doesn't fix the root cause.

## Recommendation

**Use Option 1** (Remove keyword xrefs):
- Simplest solution
- Avoids duplicate keyword indexing
- ChEMBL already provides comprehensive InChI/SMILES search
- Patent_compound entries serve their purpose for patent mappings
- Linkdataset mechanism works as intended

## Test Impact

After fix, these tests should pass:

```json
{
  "category": "chembl_drug",
  "name": "Molecule Search by SMILES",
  "query": ">>*>>chembl_molecule",
  "should_pass": ["Cn1cc(-c2ccc3c(c2)CCN3C(=O)Cc2cccc(C(F)(F)F)c2)c2c(N)ncnc21"]
},
{
  "category": "chembl_drug",
  "name": "Molecule Search by InChI Key",
  "query": ">>*>>chembl_molecule",
  "should_pass": ["SIXVRXARNAVBTC-UHFFFAOYSA-N"]
}
```

Current: **FAIL** (Entry not found error)
After fix: **PASS** (Returns CHEMBL2171124)

## Next Steps

1. Decide on solution approach (Option 1 recommended)
2. Implement fix in patents.go
3. Rebuild database
4. Re-run integration tests
5. Verify patent→compound→ChEMBL mappings work via linkdataset
