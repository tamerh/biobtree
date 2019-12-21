var UseCases4 = {
  "mix": [{
    "name": "mix search identifiers",
    "type": "0",
    "source": "",
    "searchTerm": "RAG1_HUMAN,ENSMUSG00000023456,GO:0002020,AC020895,hsa:7409",
    "mapFilterTerm": ""
  }, {
    "name": "mix proteins to go term biological",
    "type": "1",
    "source": "",
    "searchTerm": "SHH_HUMAN,P53_HUMAN,RAG1_HUMAN",
    "mapFilterTerm": "map(go).filter(go.type==\"biological_process\")"
  }, {
    "name": "mix cancer related genes to protein mutation features",
    "type": "1",
    "source": "hgnc",
    "searchTerm": "PMS2,MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
    "mapFilterTerm": "map(uniprot).map(ufeature).filter(ufeature.type==\"mutagenesis site\")"
  }, {
    "name": "mix proteins to interpro with Domain type",
    "type": "1",
    "source": "",
    "searchTerm": "SHH_HUMAN,P53_HUMAN,RAG1_HUMAN,CLOCK_HUMAN,BMAL1_HUMAN,AICDA_HUMAN,AT5G3_HUMAN",
    "mapFilterTerm": "map(interpro).filter(interpro.type==\"Domain\")"
  }, {
    "name": "mix taxid to its grand children by division",
    "type": "1",
    "source": "",
    "searchTerm": "862507",
    "mapFilterTerm": "map(taxchild).map(taxchild).filter(taxonomy.taxonomic_division==\"ROD\")"
  }],
  "protein": [{
    "name": "protein search identifiers",
    "type": "0",
    "source": "",
    "searchTerm": "rag1_human,clock_human,bmal1_human,shh_human,aicda_human,at5g3_human,p53_HUMAN",
    "mapFilterTerm": ""
  }, {
    "name": "protein search \u0026 filter by name",
    "type": "1",
    "source": "",
    "searchTerm": "rag1_human,clock_human,bmal1_human,shh_human,aicda_human,at5g3_human,p53_HUMAN",
    "mapFilterTerm": "filter(\"Sonic hedgehog protein\" in uniprot.names)"
  }, {
    "name": "protein search \u0026 filter by sequence mass",
    "type": "1",
    "source": "",
    "searchTerm": "rag1_human,clock_human,bmal1_human,shh_human,aicda_human,at5g3_human,p53_human",
    "mapFilterTerm": "filter(uniprot.sequence.mass \u003e 45000)"
  }, {
    "name": "protein human proteins by sequence size",
    "type": "1",
    "source": "",
    "searchTerm": "homo sapiens",
    "mapFilterTerm": "map(uniprot).filter(size(uniprot.sequence.seq) \u003e 400)"
  }, {
    "name": "protein go term molecular",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(go).filter(go.type==\"molecular_function\")"
  }, {
    "name": "protein go term cellular",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(go).filter(go.type==\"cellular_component\")"
  }, {
    "name": "protein go term boolean",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(go).filter(go.name.contains(\"binding\") || go.name.contains(\"activity\"))"
  }, {
    "name": "protein filter first then go terms contains word",
    "type": "1",
    "source": "",
    "searchTerm": "SHH_HUMAN,P53_HUMAN,RAG1_HUMAN,CLOCK_HUMAN,BMAL1_HUMAN,AICDA_HUMAN,AT5G3_HUMAN",
    "mapFilterTerm": "filter(size(uniprot.sequence.seq) \u003e 400).map(go).filter(go.name.contains(\"binding\") || go.name.contains(\"activity\"))"
  }, {
    "name": "protein interpro Conserved site",
    "type": "1",
    "source": "",
    "searchTerm": "SHH_HUMAN,P53_HUMAN,RAG1_HUMAN,CLOCK_HUMAN,BMAL1_HUMAN,AICDA_HUMAN,AT5G3_HUMAN",
    "mapFilterTerm": "map(interpro).filter(interpro.type==\"Conserved_site\")"
  }, {
    "name": "protein ENA type mRNA",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(ena).filter(ena.type==\"mrna\")"
  }, {
    "name": "protein ENA type genomic DNA",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(ena).filter(ena.type==\"genomic_dna\")"
  }, {
    "name": "protein to refseqs",
    "type": "1",
    "source": "",
    "searchTerm": "RAG1_HUMAN,CLOCK_HUMAN,BMAL1_HUMAN,SHH_HUMAN,AICDA_HUMAN,AT5G3_HUMAN,P53_HUMAN",
    "mapFilterTerm": "map(refseq)"
  }, {
    "name": "protein cancer related gene variants",
    "type": "1",
    "source": "hgnc",
    "searchTerm": "PMS2,MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
    "mapFilterTerm": "map(uniprot).filter(uniprot.reviewed).map(ufeature).map(variantid)"
  }, {
    "name": "protein feature helix type",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(ufeature).filter(ufeature.type==\"helix\")"
  }, {
    "name": "protein feature sequence variant",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(ufeature).filter(ufeature.type==\"sequence variant\")"
  }, {
    "name": "protein genes to mutation feature or contains",
    "type": "1",
    "source": "",
    "searchTerm": "her2,ras,p53",
    "mapFilterTerm": "map(uniprot).map(ufeature).filter(ufeature.type==\"mutagenesis site\" || ufeature.description.contains(\"cancer\"))"
  }, {
    "name": "protein feature location",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(ufeature).filter(ufeature.location.begin\u003e0 \u0026\u0026 ufeature.location.end\u003c300)"
  }, {
    "name": "protein feature description contains",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(ufeature).filter(ufeature.description.contains(\"tumor\"))"
  }, {
    "name": "protein feature specific variant",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(ufeature).filter(ufeature.original==\"I\" \u0026\u0026 ufeature.variation==\"S\")"
  }, {
    "name": "protein feature maps variantid",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(ufeature).filter(ufeature.original==\"I\" \u0026\u0026 ufeature.variation==\"S\").map(variantid)"
  }, {
    "name": "protein feature has evidences",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(ufeature).filter(size(ufeature.evidences)\u003e1)"
  }, {
    "name": "protein feature has experimental evidence",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(ufeature).filter(ufeature.evidences.exists(a,a.type==\"ECO:0000269\"))"
  }, {
    "name": "protein feature has pubmed evidence",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(ufeature).filter(ufeature.evidences.exists(a,a.source==\"pubmed\"))"
  }, {
    "name": "protein feature pdb evidence",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(ufeature).filter(ufeature.evidences.exists(a,a.source==\"pdb\"))"
  }, {
    "name": "protein pdb method NMR",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(pdb).filter(pdb.method==\"nmr\")"
  }, {
    "name": "protein pdb chains",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(pdb).filter(pdb.chains==\"A/C=95-292\")"
  }, {
    "name": "protein pdb resolution",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(pdb).filter(pdb.resolution==\"2.60 A\")"
  }, {
    "name": "protein pdb method or chains",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(pdb).filter(pdb.method==\"nmr\" || pdb.chains==\"C/D=1-177\")"
  }, {
    "name": "protein reactome activation pathways",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(reactome).filter(reactome.pathway.contains(\"activation\"))"
  }, {
    "name": "protein reactome signaling pathways",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(reactome).filter(reactome.pathway.contains(\"signaling\"))"
  }, {
    "name": "protein reactome regulation pathways",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(reactome).filter(reactome.pathway.contains(\"Regulation\"))"
  }, {
    "name": "protein orphanet disease name",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(orphanet).filter(orphanet.disease.contains(\"cancer\"))"
  }, {
    "name": "protein durgs by drugbank",
    "type": "1",
    "source": "",
    "searchTerm": "shh_human,p53_human",
    "mapFilterTerm": "map(drugbank)"
  }],
  "taxonomy": [{
    "name": "taxonomy taxonomy children",
    "type": "1",
    "source": "",
    "searchTerm": "9606",
    "mapFilterTerm": "map(taxchild)"
  }, {
    "name": "taxonomy  taxonomy grand children",
    "type": "1",
    "source": "",
    "searchTerm": "862507",
    "mapFilterTerm": "map(taxchild).map(taxchild)"
  }, {
    "name": "taxonomy taxonomy grand^2 parent",
    "type": "1",
    "source": "",
    "searchTerm": "10090",
    "mapFilterTerm": "map(taxparent).map(taxparent).map(taxparent)"
  }, {
    "name": "taxonomy taxonomy Asian children",
    "type": "1",
    "source": "",
    "searchTerm": "10090",
    "mapFilterTerm": "map(taxchild).filter(taxonomy.common_name.contains(\"Asian\"))"
  }, {
    "name": "taxonomy taxonomy European children",
    "type": "1",
    "source": "",
    "searchTerm": "10090",
    "mapFilterTerm": "map(taxchild).filter(taxonomy.common_name.contains(\"European\"))"
  }, {
    "name": "taxonomy go term parent",
    "type": "1",
    "source": "",
    "searchTerm": "GO:0004707",
    "mapFilterTerm": "map(goparent)"
  }, {
    "name": "taxonomy go term parent type",
    "type": "1",
    "source": "",
    "searchTerm": "GO:0004707",
    "mapFilterTerm": "map(goparent).filter(go.type==\"biological_process\")"
  }, {
    "name": "taxonomy efo disaease name",
    "type": "0",
    "source": "",
    "searchTerm": "inflammatory bowel disease",
    "mapFilterTerm": ""
  }, {
    "name": "taxonomy efo children",
    "type": "1",
    "source": "",
    "searchTerm": "EFO:0003767",
    "mapFilterTerm": "map(efochild)"
  }, {
    "name": "taxonomy efo parent",
    "type": "1",
    "source": "",
    "searchTerm": "EFO:0000384",
    "mapFilterTerm": "map(efoparent)"
  }, {
    "name": "taxonomy eco children",
    "type": "1",
    "source": "",
    "searchTerm": "ECO:0000269",
    "mapFilterTerm": "map(ecochild)"
  }, {
    "name": "taxonomy eco parent",
    "type": "1",
    "source": "",
    "searchTerm": "ECO:0007742",
    "mapFilterTerm": "map(ecoparent)"
  }]
};

export default UseCases4;