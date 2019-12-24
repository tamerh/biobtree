var UseCases = {
  "mix": [{
    "name": "search identifiers",
    "type": "0",
    "source": "",
    "searchTerm": "RAG1_HUMAN,ENSMUSG00000023456,GO:0002020,CHEMBL2242,AC020895,hsa:7409",
    "mapFilterTerm": ""
  }, {
    "name": "proteins to go term biological",
    "type": "1",
    "source": "",
    "searchTerm": "SHH_HUMAN,P53_HUMAN,RAG1_HUMAN",
    "mapFilterTerm": "map(go).filter(go.type==\"biological_process\")"
  }, {
    "name": "cancer related genes to protein mutation features",
    "type": "1",
    "source": "hgnc",
    "searchTerm": "PMS2,MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
    "mapFilterTerm": "map(uniprot).map(ufeature).filter(ufeature.type==\"mutagenesis site\")"
  }, {
    "name": "proteins to interpro with Domain type",
    "type": "1",
    "source": "",
    "searchTerm": "SHH_HUMAN,P53_HUMAN,RAG1_HUMAN,CLOCK_HUMAN,BMAL1_HUMAN,AICDA_HUMAN,AT5G3_HUMAN",
    "mapFilterTerm": "map(interpro).filter(interpro.type==\"Domain\")"
  }, {
    "name": "ensembl human genes to mouse Ortholog genes",
    "type": "1",
    "source": "",
    "searchTerm": "SHH,VAV1,TP53",
    "mapFilterTerm": "filter(ensembl.genome==\"homo_sapiens\").map(ortholog).filter(ensembl.genome==\"mus_musculus\")"
  }, {
    "name": "kinase activity goterm to hgnc genes",
    "type": "1",
    "source": "",
    "searchTerm": "GO:0004707",
    "mapFilterTerm": "map(ensembl).map(hgnc)"
  }, {
    "name": "probe id to ensembl then hgnc by location",
    "type": "1",
    "source": "",
    "searchTerm": "202763_at,209310_s_at,207500_at",
    "mapFilterTerm": "map(transcript).map(ensembl).filter(ensembl.genome==\"homo_sapiens\").map(hgnc).filter(hgnc.location==\"4q35.1\")"
  }, {
    "name": "crispr cas9 genes by genomes to transcript",
    "type": "1",
    "source": "",
    "searchTerm": "cas9",
    "mapFilterTerm": "filter(ensembl.genome==\"campylobacter_coli_gca_001717605\").map(transcript)"
  }, {
    "name": "inflammatory bowel disease to chembl molecules phase 3,4",
    "type": "1",
    "source": "",
    "searchTerm": "inflammatory bowel disease",
    "mapFilterTerm": "map(chembl_molecule).filter(chembl.molecule.highestDevelopmentPhase\u003e2)"
  }, {
    "name": "taxid to its grand children by division",
    "type": "1",
    "source": "",
    "searchTerm": "862507",
    "mapFilterTerm": "map(taxchild).map(taxchild).filter(taxonomy.taxonomic_division==\"ROD\")"
  }],
  "gene": [{
    "name": "search crispr cas9 genes",
    "type": "0",
    "source": "",
    "searchTerm": "cas9",
    "mapFilterTerm": ""
  }, {
    "name": "ensembl id to Entrez id",
    "type": "1",
    "source": "",
    "searchTerm": "ENSG00000139618",
    "mapFilterTerm": "map(entrez)"
  }, {
    "name": "entrez id to ensembl then goterm",
    "type": "1",
    "source": "",
    "searchTerm": "675",
    "mapFilterTerm": "map(ensembl).map(go)"
  }, {
    "name": "genes to molecular goterm",
    "type": "1",
    "source": "",
    "searchTerm": "tpi1,shh",
    "mapFilterTerm": "filter(ensembl.genome==\"homo_sapiens\").map(go).filter(go.type==\"molecular_function\")"
  }, {
    "name": "crispr cas9 genes to transcripts",
    "type": "1",
    "source": "",
    "searchTerm": "cas9",
    "mapFilterTerm": "map(transcript)"
  }, {
    "name": "crispr cas9 genes by genomes to ENA",
    "type": "1",
    "source": "",
    "searchTerm": "cas9",
    "mapFilterTerm": "filter(ensembl.genome==\"campylobacter_coli_gca_001717605\").map(ena)"
  }, {
    "name": "crispr cas9 genes by genomes to go terms",
    "type": "1",
    "source": "",
    "searchTerm": "cas9",
    "mapFilterTerm": "filter(ensembl.genome==\"campylobacter_coli_gca_001717605\").map(go).filter(go.type==\"biological_process\")"
  }, {
    "name": "crispr cas9 genes to NCBI Genbank",
    "type": "1",
    "source": "",
    "searchTerm": "cas9",
    "mapFilterTerm": "map(ena).map(genbank)"
  }, {
    "name": "cancer related genes to transcripts",
    "type": "1",
    "source": "",
    "searchTerm": "PMS2,MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
    "mapFilterTerm": "filter(ensembl.genome==\"homo_sapiens\").map(transcript)"
  }, {
    "name": "cancer related genes to uniprot",
    "type": "1",
    "source": "",
    "searchTerm": "PMS2,MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
    "mapFilterTerm": "filter(ensembl.genome==\"homo_sapiens\").map(uniprot)"
  }, {
    "name": "cancer related genes to uniprot go terms",
    "type": "1",
    "source": "",
    "searchTerm": "PMS2,MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
    "mapFilterTerm": "filter(ensembl.genome==\"homo_sapiens\").map(uniprot).map(go)"
  }, {
    "name": "cancer related genes to uniprot via hgnc",
    "type": "1",
    "source": "hgnc",
    "searchTerm": "PMS2,MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
    "mapFilterTerm": "map(uniprot).filter(uniprot.reviewed)"
  }, {
    "name": "cancer related genes to uniprot go terms via hgnc",
    "type": "1",
    "source": "hgnc",
    "searchTerm": "PMS2,MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
    "mapFilterTerm": "map(uniprot).filter(uniprot.reviewed).map(go).filter(go.type==\"cellular_component\")"
  }, {
    "name": "ensembl id to transcripts by type",
    "type": "1",
    "source": "",
    "searchTerm": "ENSG00000073910",
    "mapFilterTerm": "map(transcript).filter(transcript.biotype==\"protein_coding\")"
  }, {
    "name": "probe id to ensembl",
    "type": "1",
    "source": "",
    "searchTerm": "202763_at,209310_s_at,207500_at",
    "mapFilterTerm": "map(transcript).map(ensembl).filter(ensembl.genome==\"homo_sapiens\")"
  }, {
    "name": "probe id to ensembl then hgnc",
    "type": "1",
    "source": "",
    "searchTerm": "202763_at,209310_s_at,207500_at",
    "mapFilterTerm": "map(transcript).map(ensembl).map(hgnc)"
  }, {
    "name": "ensembl with location then uniprot reviewed",
    "type": "1",
    "source": "",
    "searchTerm": "homo_sapiens",
    "mapFilterTerm": "map(ensembl).filter(ensembl.start\u003e100000000 \u0026\u0026 ensembl.seq_region==\"X\").map(uniprot).filter(uniprot.reviewed)"
  }, {
    "name": "ensembl id to exons",
    "type": "1",
    "source": "",
    "searchTerm": "ENSG00000141510",
    "mapFilterTerm": "map(transcript).map(exon)"
  }, {
    "name": "gene to exons by region",
    "type": "1",
    "source": "",
    "searchTerm": "tp53",
    "mapFilterTerm": "map(transcript).filter(transcript.biotype==\"protein_coding\").map(exon).filter(exon.seq_region==\"17\")"
  }, {
    "name": "gene to exons by location",
    "type": "1",
    "source": "",
    "searchTerm": "tp53",
    "mapFilterTerm": "map(transcript).filter(transcript.biotype==\"protein_coding\").map(exon).filter(exon.end \u003e= 7687538)"
  }, {
    "name": "ensembl id to orthologs",
    "type": "1",
    "source": "",
    "searchTerm": "ENSG00000139618",
    "mapFilterTerm": "map(ortholog)"
  }, {
    "name": "gene orthologs",
    "type": "1",
    "source": "",
    "searchTerm": "shh",
    "mapFilterTerm": "filter(ensembl.genome==\"homo_sapiens\").map(ortholog)"
  }, {
    "name": "ensembl id to paralog",
    "type": "1",
    "source": "",
    "searchTerm": "ENSG00000073910",
    "mapFilterTerm": "map(paralog)"
  }, {
    "name": "gene to Paralog",
    "type": "1",
    "source": "",
    "searchTerm": "FRY",
    "mapFilterTerm": "filter(ensembl.genome==\"homo_sapiens\").map(paralog)"
  }, {
    "name": "gene name to paralog transcripts",
    "type": "1",
    "source": "",
    "searchTerm": "FRY",
    "mapFilterTerm": "filter(ensembl.genome==\"homo_sapiens\").map(paralog).map(transcript)"
  }, {
    "name": "refseq to interpro family",
    "type": "1",
    "source": "",
    "searchTerm": "NM_005359,NM_000546",
    "mapFilterTerm": "map(hgnc).map(uniprot).map(interpro).filter(interpro.type==\"Family\")"
  }, {
    "name": "refseq to interpro domain",
    "type": "1",
    "source": "",
    "searchTerm": "NM_005359,NM_000546",
    "mapFilterTerm": "map(hgnc).map(uniprot).map(interpro).filter(interpro.type==\"Domain\")"
  }, {
    "name": "ensembl human genes with MAP kinase activity",
    "type": "1",
    "source": "",
    "searchTerm": "GO:0004707",
    "mapFilterTerm": "map(ensembl).filter(ensembl.branch==1 \u0026\u0026 ensembl.genome==\"homo_sapiens\")"
  }],
  "protein": [{
    "name": "search identifiers",
    "type": "0",
    "source": "",
    "searchTerm": "RAG1_HUMAN,CLOCK_HUMAN,BMAL1_HUMAN,SHH_HUMAN,AICDA_HUMAN,AT5G3_HUMAN,P53_HUMAN",
    "mapFilterTerm": ""
  }, {
    "name": "search \u0026 filter by name",
    "type": "1",
    "source": "",
    "searchTerm": "RAG1_HUMAN,CLOCK_HUMAN,BMAL1_HUMAN,SHH_HUMAN,AICDA_HUMAN,AT5G3_HUMAN,P53_HUMAN",
    "mapFilterTerm": "filter(uniprot.names.exists(a,a==\"Sonic hedgehog protein\"))"
  }, {
    "name": "search \u0026 filter by sequence mass",
    "type": "1",
    "source": "",
    "searchTerm": "RAG1_HUMAN,CLOCK_HUMAN,BMAL1_HUMAN,SHH_HUMAN,AICDA_HUMAN,AT5G3_HUMAN,P53_HUMAN",
    "mapFilterTerm": "filter(uniprot.sequence.mass \u003e 45000)"
  }, {
    "name": "search \u0026 filter by sequence size",
    "type": "1",
    "source": "",
    "searchTerm": "RAG1_HUMAN,CLOCK_HUMAN,BMAL1_HUMAN,SHH_HUMAN,AICDA_HUMAN,AT5G3_HUMAN,P53_HUMAN",
    "mapFilterTerm": "filter(size(uniprot.sequence.seq) \u003e 400)"
  }, {
    "name": "go term molecular",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(go).filter(go.type==\"molecular_function\")"
  }, {
    "name": "go term cellular",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(go).filter(go.type==\"cellular_component\")"
  }, {
    "name": "go term boolean",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(go).filter(go.name.contains(\"binding\") || go.name.contains(\"activity\"))"
  }, {
    "name": "filter first then go terms contains word",
    "type": "1",
    "source": "",
    "searchTerm": "SHH_HUMAN,P53_HUMAN,RAG1_HUMAN,CLOCK_HUMAN,BMAL1_HUMAN,AICDA_HUMAN,AT5G3_HUMAN",
    "mapFilterTerm": "filter(size(uniprot.sequence.seq) \u003e 400).map(go).filter(go.name.contains(\"binding\") || go.name.contains(\"activity\"))"
  }, {
    "name": "interpro Conserved site",
    "type": "1",
    "source": "",
    "searchTerm": "SHH_HUMAN,P53_HUMAN,RAG1_HUMAN,CLOCK_HUMAN,BMAL1_HUMAN,AICDA_HUMAN,AT5G3_HUMAN",
    "mapFilterTerm": "map(interpro).filter(interpro.type==\"Conserved_site\")"
  }, {
    "name": "ENA type mRNA",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ena).filter(ena.type==\"mrna\")"
  }, {
    "name": "ENA type genomic DNA",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ena).filter(ena.type==\"genomic_dna\")"
  }, {
    "name": "to refseqs",
    "type": "1",
    "source": "",
    "searchTerm": "RAG1_HUMAN,CLOCK_HUMAN,BMAL1_HUMAN,SHH_HUMAN,AICDA_HUMAN,AT5G3_HUMAN,P53_HUMAN",
    "mapFilterTerm": "map(refseq)"
  }, {
    "name": "cancer related gene variants",
    "type": "1",
    "source": "hgnc",
    "searchTerm": "PMS2,MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
    "mapFilterTerm": "map(uniprot).filter(uniprot.reviewed).map(ufeature).map(variantid)"
  }, {
    "name": "feature helix type",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.type==\"helix\")"
  }, {
    "name": "feature sequence variant",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.type==\"sequence variant\")"
  }, {
    "name": "genes to mutation feature or contains",
    "type": "1",
    "source": "",
    "searchTerm": "her2,ras,p53",
    "mapFilterTerm": "map(uniprot).map(ufeature).filter(ufeature.type==\"mutagenesis site\" || ufeature.description.contains(\"cancer\"))"
  }, {
    "name": "feature location",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.location.begin\u003e0 \u0026\u0026 ufeature.location.end\u003c300)"
  }, {
    "name": "feature description contains",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.description.contains(\"tumor\"))"
  }, {
    "name": "feature specific variant",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.original==\"I\" \u0026\u0026 ufeature.variation==\"S\")"
  }, {
    "name": "feature maps variantid",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.original==\"I\" \u0026\u0026 ufeature.variation==\"S\").map(variantid)"
  }, {
    "name": "feature has evidences",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(size(ufeature.evidences)\u003e1)"
  }, {
    "name": "feature has experimental evidence",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.evidences.exists(a,a.type==\"ECO:0000269\"))"
  }, {
    "name": "feature has pubmed evidence",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.evidences.exists(a,a.source==\"pubmed\"))"
  }, {
    "name": "feature pdb evidence",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.evidences.exists(a,a.source==\"pdb\"))"
  }, {
    "name": "pdb method NMR",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(pdb).filter(pdb.method==\"nmr\")"
  }, {
    "name": "pdb chains",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(pdb).filter(pdb.chains==\"A/C=95-292\")"
  }, {
    "name": "pdb resolution",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(pdb).filter(pdb.resolution==\"2.60 A\")"
  }, {
    "name": "pdb method or chains",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(pdb).filter(pdb.method==\"nmr\" || pdb.chains==\"C/D=1-177\")"
  }, {
    "name": "reactome activation pathways",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(reactome).filter(reactome.pathway.contains(\"activation\"))"
  }, {
    "name": "reactome signaling pathways",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(reactome).filter(reactome.pathway.contains(\"signaling\"))"
  }, {
    "name": "reactome regulation pathways",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(reactome).filter(reactome.pathway.contains(\"Regulation\"))"
  }, {
    "name": "orphanet disease name",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(orphanet).filter(orphanet.disease.contains(\"cancer\"))"
  }, {
    "name": "durgs by drugbank",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(drugbank)"
  }],
  "chembl": [{
    "name": "search target",
    "type": "0",
    "source": "",
    "searchTerm": "CHEMBL2242",
    "mapFilterTerm": ""
  }, {
    "name": "search target component",
    "type": "0",
    "source": "",
    "searchTerm": "CHEMBL_TC_47",
    "mapFilterTerm": ""
  }, {
    "name": "target single protein to uniprot",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL2789",
    "mapFilterTerm": "filter(chembl.target.type==\"single_protein\").map(chembl_target_component).map(uniprot)"
  }, {
    "name": "cancer related genes to targets",
    "type": "1",
    "source": "",
    "searchTerm": "PMS2,MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
    "mapFilterTerm": "map(uniprot).map(chembl_target_component).map(chembl_target)"
  }, {
    "name": "cancer related genes to target with type",
    "type": "1",
    "source": "hgnc",
    "searchTerm": "PMS2,MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
    "mapFilterTerm": "map(uniprot).map(chembl_target_component).map(chembl_target).filter(chembl.target.type==\"protein-protein_interaction\")"
  }, {
    "name": "search molecule",
    "type": "0",
    "source": "",
    "searchTerm": "GSK2606414",
    "mapFilterTerm": ""
  }, {
    "name": "search molecule by smiles",
    "type": "0",
    "source": "",
    "searchTerm": "Cn1cc(c2ccc3N(CCc3c2)C(=O)Cc4cccc(c4)C(F)(F)F)c5c(N)ncnc15",
    "mapFilterTerm": ""
  }, {
    "name": "search molecule by inchi key",
    "type": "0",
    "source": "",
    "searchTerm": "SIXVRXARNAVBTC-UHFFFAOYSA-N",
    "mapFilterTerm": ""
  }, {
    "name": "molecule activities",
    "type": "1",
    "source": "",
    "searchTerm": "GSK2606414",
    "mapFilterTerm": "map(chembl_activity)"
  }, {
    "name": "molecule activities filter bao",
    "type": "1",
    "source": "",
    "searchTerm": "GSK2606414",
    "mapFilterTerm": "map(chembl_activity).filter(chembl.activity.bao==\"BAO_0000190\")"
  }, {
    "name": "molecule activities filter value",
    "type": "1",
    "source": "",
    "searchTerm": "GSK2606414",
    "mapFilterTerm": "map(chembl_activity).filter(chembl.activity.value \u003e 10.0)"
  }, {
    "name": "molecule activities AND",
    "type": "1",
    "source": "",
    "searchTerm": "GSK2606414",
    "mapFilterTerm": "map(chembl_activity).filter(chembl.activity.value \u003e 10.0 \u0026\u0026 chembl.activity.bao==\"BAO_0000190\")"
  }, {
    "name": "molecule activities OR",
    "type": "1",
    "source": "",
    "searchTerm": "GSK2606414",
    "mapFilterTerm": "map(chembl_activity).filter(chembl.activity.value\u003e10.0 || chembl.activity.pChembl\u003e5.0)"
  }, {
    "name": "molecule targets",
    "type": "1",
    "source": "",
    "searchTerm": "GSK2606414",
    "mapFilterTerm": "map(chembl_activity).map(chembl_document).map(chembl_assay).map(chembl_target)"
  }, {
    "name": "search document",
    "type": "0",
    "source": "",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": ""
  }, {
    "name": "document activities",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL1121978",
    "mapFilterTerm": "map(chembl_activity)"
  }, {
    "name": "document assay",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay)"
  }, {
    "name": "document assay filter",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).filter(chembl.assay.type==\"Functional\" || chembl.assay.type==\"Binding\")"
  }, {
    "name": "document cell line",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).map(chembl_cell_line)"
  }, {
    "name": "document cell line Filter",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).map(chembl_cell_line).filter(chembl.cellLine.tax==\"9615\" || chembl.cellLine.efo==\"EFO_0002841\")"
  }, {
    "name": "document target",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).map(chembl_target)"
  }, {
    "name": "document target protein type",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).map(chembl_target).filter(chembl.target.type==\"single_protein\")"
  }, {
    "name": "document target tissue",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).map(chembl_target).filter(chembl.target.type==\"tissue\")"
  }, {
    "name": "document target organism",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).map(chembl_target).filter(chembl.target.type==\"organism\")"
  }, {
    "name": "document target protein uniprot",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).map(chembl_target).map(chembl_target_component).map(uniprot)"
  }, {
    "name": "document molecule",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_molecule)"
  }, {
    "name": "document molecule filter",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_molecule).filter(chembl.molecule.heavyAtoms \u003c 30.0 \u0026\u0026 chembl.molecule.aromaticRings \u003c2.0)"
  }, {
    "name": "search assay",
    "type": "0",
    "source": "",
    "searchTerm": "CHEMBL615156",
    "mapFilterTerm": ""
  }, {
    "name": "assay target",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL615156",
    "mapFilterTerm": "map(chembl_target)"
  }, {
    "name": "assay cell line",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL3424821",
    "mapFilterTerm": "map(chembl_cell_line)"
  }, {
    "name": "assay target protein",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL615156",
    "mapFilterTerm": "map(chembl_target).filter(chembl.target.type==\"single_protein\")"
  }, {
    "name": "assay target protein uniprot",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL615156",
    "mapFilterTerm": "map(chembl_target).map(chembl_target_component).map(uniprot)"
  }, {
    "name": "search activity",
    "type": "0",
    "source": "",
    "searchTerm": "CHEMBL_ACT_93229",
    "mapFilterTerm": ""
  }, {
    "name": "activity molecule with filter",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL_ACT_93229",
    "mapFilterTerm": "filter(chembl.activity.bao==\"BAO_0000179\").map(chembl_molecule)"
  }, {
    "name": "search cell line",
    "type": "0",
    "source": "",
    "searchTerm": "CHEMBL3307241",
    "mapFilterTerm": ""
  }, {
    "name": "search cell line assay",
    "type": "1",
    "source": "",
    "searchTerm": "CHEMBL3307241",
    "mapFilterTerm": "map(chembl_assay)"
  }],
  "taxonomy": [{
    "name": "taxonomy children",
    "type": "1",
    "source": "",
    "searchTerm": "9606",
    "mapFilterTerm": "map(taxchild)"
  }, {
    "name": " taxonomy grand children",
    "type": "1",
    "source": "",
    "searchTerm": "862507",
    "mapFilterTerm": "map(taxchild).map(taxchild)"
  }, {
    "name": "taxonomy grand^2 parent",
    "type": "1",
    "source": "",
    "searchTerm": "10090",
    "mapFilterTerm": "map(taxparent).map(taxparent).map(taxparent)"
  }, {
    "name": "taxonomy Asian children",
    "type": "1",
    "source": "",
    "searchTerm": "10090",
    "mapFilterTerm": "map(taxchild).filter(taxonomy.common_name.contains(\"Asian\"))"
  }, {
    "name": "taxonomy European children",
    "type": "1",
    "source": "",
    "searchTerm": "10090",
    "mapFilterTerm": "map(taxchild).filter(taxonomy.common_name.contains(\"European\"))"
  }, {
    "name": "go term parent",
    "type": "1",
    "source": "",
    "searchTerm": "GO:0004707",
    "mapFilterTerm": "map(goparent)"
  }, {
    "name": "go term parent type",
    "type": "1",
    "source": "",
    "searchTerm": "GO:0004707",
    "mapFilterTerm": "map(goparent).filter(go.type==\"biological_process\")"
  }, {
    "name": "efo disaease name",
    "type": "0",
    "source": "",
    "searchTerm": "inflammatory bowel disease",
    "mapFilterTerm": ""
  }, {
    "name": "efo children",
    "type": "1",
    "source": "",
    "searchTerm": "EFO:0003767",
    "mapFilterTerm": "map(efochild)"
  }, {
    "name": "efo parent",
    "type": "1",
    "source": "",
    "searchTerm": "EFO:0000384",
    "mapFilterTerm": "map(efoparent)"
  }, {
    "name": "eco children",
    "type": "1",
    "source": "",
    "searchTerm": "ECO:0000269",
    "mapFilterTerm": "map(ecochild)"
  }, {
    "name": "eco parent",
    "type": "1",
    "source": "",
    "searchTerm": "ECO:0007742",
    "mapFilterTerm": "map(ecoparent)"
  }]
};

export default UseCases;