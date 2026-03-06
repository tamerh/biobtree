# ClinicalTrials.gov Database

## Overview

ClinicalTrials.gov provides comprehensive information about clinical studies for a wide range of diseases and conditions, serving as the world's largest clinical trials registry. Contains 500,000+ registered studies from 220+ countries with detailed protocols, eligibility criteria, interventions, outcomes, facilities, sponsors, and study status. Essential for clinical research planning, patient recruitment, drug development tracking, and evidence-based medicine requiring authoritative clinical trial information.

**Source**: ClinicalTrials.gov API (clinicaltrials.gov)
**Data Type**: Clinical trial registration data with study protocols and metadata

## Integration Architecture

### Storage Model

**Primary Entries**:
- NCT IDs (e.g., `NCT06401707`) serve as primary keys
- National Clinical Trial numbers assigned by ClinicalTrials.gov

**Searchable Text Links**:
- NCT IDs indexed as keywords for direct lookup
- Condition names, intervention names, and sponsor names searchable

**Attributes Stored** (protobuf ClinicalTrialAttr):
- `brief_title`: Short study title
- `official_title`: Full official study title
- `brief_summary`: Study description and objectives
- `overall_status`: Current status (RECRUITING, COMPLETED, TERMINATED, etc.)
- `phase`: Trial phase (PHASE1, PHASE2, PHASE3, PHASE4, NA)
- `study_type`: Type (INTERVENTIONAL, OBSERVATIONAL, EXPANDED_ACCESS)
- `enrollment`: Target/actual participant count
- `start_date`, `completion_date`: Study timeline
- `eligibility`: Inclusion/exclusion criteria, age range, gender
- `outcomes`: Primary and secondary outcome measures
- `interventions`: Drugs, devices, procedures being studied
- `conditions`: Diseases/conditions being investigated
- `sponsors`: Lead and collaborator organizations
- `facilities`: Study locations with addresses and recruitment status
- `study_arms`: Treatment groups and control arms

**Cross-References**:
- **MeSH Terms**: Medical subject headings for conditions
- **Intervention Names**: Drug names, device names
- **Facilities**: Geographic locations and institutions
- **Sponsors**: Universities, pharmaceutical companies, NIH
- **Related Studies**: Connected trials

### Special Features

**Unique Build Architecture**:
- **Only dataset that requires existing biobtree database during build phase**
- Opens read-only LMDB lookup database to enable intelligent cross-referencing
- Maps clinical trial data to existing biobtree entities (ChEMBL drugs, MONDO diseases)
- See `src/update/clinical_trials.go:initLookupDB()` for implementation details

**Medical Term Processing** (10+ mapping strategies):
- Intelligent NLP for medical terminology with sophisticated fallback chain
- Disease/condition extraction and normalization to MONDO disease ontology
- **Mapping attempts** (in order): exact match → disease corrections → spelling variations → cancer abbreviations → cancer qualifiers → parentheses removal → slash/or splitting → specific patterns → general qualifiers → anatomical terms → singular/plural
- Anatomical term recognition (13+ specific, 12+ anatomical categories)
- Cancer qualifier mapping (125+ terms: stage, metastasis, receptor patterns)
- Drug/intervention name standardization with ChEMBL molecule mapping
- Configuration-driven via `conf/medical_term_mappings.json`

**Comprehensive Protocol Data**:
- Full eligibility criteria with age/gender constraints
- Detailed outcome measures with descriptions
- Multi-arm study designs
- Intervention dosing and administration details

**Real-Time Status Tracking**:
- Recruitment status per facility
- Study phase progression
- Enrollment milestones
- Timeline updates

**Geographic Coverage**:
- 220+ countries
- Multi-site trials with facility-level detail
- City, state, country information
- Contact information for recruiting sites

## Use Cases

**1. Patient Enrollment**
```
Query: Disease name → Filter by status=RECRUITING → Find eligible trials
Use: Connect patients with relevant clinical studies
```

**2. Drug Development Tracking**
```
Query: Drug name → All trials → Track phase progression
Use: Monitor competitive landscape and development timelines
```

**3. Evidence Synthesis**
```
Query: Condition → COMPLETED trials → Extract outcomes
Use: Systematic reviews and meta-analyses
```

**4. Site Selection**
```
Query: Disease + Geographic area → Active trials and facilities
Use: Identify collaboration opportunities
```

**5. Regulatory Intelligence**
```
Query: Sponsor name → All trials → Phase distribution
Use: Track pharmaceutical pipelines
```

**6. Comparative Effectiveness**
```
Query: Multiple interventions → Compare study designs
Use: Assess evidence quality and study heterogeneity
```

## Test Cases

**Current Tests** (9 declarative tests):
- 3 ID lookup tests (multiple NCT IDs)
- 3 attribute presence tests (phase, study_type, overall_status)
- 1 multi-lookup test (batch of 3 trials)
- 1 case-insensitive test
- 1 invalid ID test

**Coverage**:
- ✅ NCT ID lookup and validation
- ✅ Phase annotation (PHASE1, PHASE2, PHASE3, PHASE4)
- ✅ Study type classification
- ✅ Overall status tracking
- ✅ Case-insensitive search
- ✅ Invalid ID handling

**Recommended Additions**:
- Eligibility criteria parsing validation
- Outcome measures structure check
- Intervention data completeness
- Multi-site trial validation
- Sponsor information check
- Date range validation (start < completion)
- Enrollment number validity
- Facility status verification

## Performance

- **Test Build**: ~10-15s (20 clinical trials)
- **Data Source**: ClinicalTrials.gov API (ZIP download + XML parsing)
- **Full Build**: Hours (500,000+ trials)
- **Test Data**: ~20 clinical trials
- **Database Size**: Varies (protocols can be extensive)
- **Update Frequency**: Daily (new trials registered continuously)

## Known Limitations

**Memory Requirements** (Critical):
- Clinical trials processing is **unique** among biobtree datasets: it requires an **existing biobtree database** during the build phase for intelligent cross-referencing
- Uses read-only LMDB lookup to map intervention names to ChEMBL molecules and conditions to MONDO diseases
- Memory allocation for LMDB memory-mapped I/O may fail in resource-constrained environments: "mdb_env_open: cannot allocate memory"
- **This is not a bug** - it's an environmental constraint requiring sufficient available memory for database mapping
- Workaround: Build on systems with adequate memory (typically >4GB available)
- See `src/update/clinical_trials.go:83-117` for lookup DB initialization
- The lookup DB enables sophisticated medical term normalization (10+ mapping strategies)

**Data Completeness**:
- Not all fields populated for all trials
- Historical trials may lack detail
- Some trials have minimal protocol information
- Completion dates may be estimates

**Medical Term Mapping**:
- Terminology normalization is complex
- Some disease names have multiple representations
- Anatomical terms may be ambiguous
- Cancer qualifiers continuously expanding

**Update Latency**:
- Daily updates from ClinicalTrials.gov
- Some status changes may lag real-world events
- Facility recruitment status not always current

**Protocol Details**:
- Eligibility criteria in free text (not structured)
- Outcome measures vary in specificity
- Intervention descriptions lack standardization
- Study arm details may be incomplete

## Future Work

- Resolve memory allocation issues for test build
- Add MeSH term cross-references
- Link to PubMed for published results
- Geographic search by proximity
- Disease ontology mapping (integrate with MONDO, HPO)
- Drug name standardization (link to ChEMBL)
- Outcome measure extraction and structuring
- Eligibility criteria NLP parsing
- Multi-trial analysis tests
- Timeline validation tests

## Maintenance

- **Release Schedule**: Daily updates from ClinicalTrials.gov
- **Current Trials**: 500,000+ registered studies
- **Data Format**: XML (downloaded as ZIP, parsed to JSON)
- **Test Data**: 20 trials spanning various phases and conditions
- **License**: Public domain (U.S. government data)
- **API Documentation**: https://clinicaltrials.gov/data-api/about-api

## References

- **Citation**: Zarin DA, et al. (2016) Update on Trial Registration 11 Years after the ICMJE Policy Was Established. N Engl J Med. 375(24):2381-2385.
- **Website**: https://clinicaltrials.gov/
- **API**: https://clinicaltrials.gov/data-api/api
- **Data Downloads**: https://clinicaltrials.gov/data-api/about-api/download-data
- **License**: Public domain (U.S. government)
