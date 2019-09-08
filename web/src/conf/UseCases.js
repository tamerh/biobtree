var UseCases = {
  "chembl": [{
    "name": "chembl Compound",
    "type": "0",
    "searchTerm": "GSK2606414",
    "mapFilterTerm": ""
  }, {
    "name": "chembl Compound By Smiles",
    "type": "0",
    "searchTerm": "Cn1cc(c2ccc3N(CCc3c2)C(=O)Cc4cccc(c4)C(F)(F)F)c5c(N)ncnc15",
    "mapFilterTerm": ""
  }, {
    "name": "chembl Compound By Inchi Key",
    "type": "0",
    "searchTerm": "SIXVRXARNAVBTC-UHFFFAOYSA-N",
    "mapFilterTerm": ""
  }, {
    "name": "chembl Compound Activities",
    "type": "1",
    "searchTerm": "GSK2606414",
    "mapFilterTerm": "map(chembl_activity)"
  }, {
    "name": "chembl Compound Activities Filter Bao",
    "type": "1",
    "searchTerm": "GSK2606414",
    "mapFilterTerm": "map(chembl_activity).filter(chembl.activity.bao==\"BAO_0000190\")"
  }, {
    "name": "chembl Compound Activities Filter Value",
    "type": "1",
    "searchTerm": "GSK2606414",
    "mapFilterTerm": "map(chembl_activity).filter(chembl.activity.value \u003e 10.0)"
  }, {
    "name": "chembl Compound Activities AND",
    "type": "1",
    "searchTerm": "GSK2606414",
    "mapFilterTerm": "map(chembl_activity).filter(chembl.activity.value \u003e 10.0 \u0026\u0026 chembl.activity.bao==\"BAO_0000190\")"
  }, {
    "name": "chembl Compound Activities OR",
    "type": "1",
    "searchTerm": "GSK2606414",
    "mapFilterTerm": "map(chembl_activity).filter(chembl.activity.value\u003e10.0 || chembl.activity.pChembl\u003e5.0)"
  }, {
    "name": "chembl Compound Targets",
    "type": "1",
    "searchTerm": "GSK2606414",
    "mapFilterTerm": "map(chembl_activity).map(chembl_document).map(chembl_assay).map(chembl_target)"
  }, {
    "name": "chembl Document",
    "type": "0",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": ""
  }, {
    "name": "chembl Document Activities",
    "type": "1",
    "searchTerm": "CHEMBL1121978",
    "mapFilterTerm": "map(chembl_activity)"
  }, {
    "name": "chembl Document Assay",
    "type": "1",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay)"
  }, {
    "name": "chembl Document Assay Filter",
    "type": "1",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).filter(chembl.assay.type==\"Functional\" || chembl.assay.type==\"Binding\")"
  }, {
    "name": "chembl Document Cell Line",
    "type": "1",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).map(chembl_cell_line)"
  }, {
    "name": "chembl Document Cell Line Filter",
    "type": "1",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).map(chembl_cell_line).filter(chembl.cellLine.tax==\"9615\" || chembl.cellLine.efo==\"EFO_0002841\")"
  }, {
    "name": "chembl Document Targets",
    "type": "1",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).map(chembl_target)"
  }, {
    "name": "chembl Document Target Protein",
    "type": "1",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).map(chembl_target).filter(chembl.target.type==\"single_protein\")"
  }, {
    "name": "chembl Document Target Tissue",
    "type": "1",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).map(chembl_target).filter(chembl.target.type==\"tissue\")"
  }, {
    "name": "chembl Document Target Organism",
    "type": "1",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).map(chembl_target).filter(chembl.target.type==\"organism\")"
  }, {
    "name": "chembl Document Target Protein Uniprot",
    "type": "1",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_assay).map(chembl_target).map(chembl_target_component).map(uniprot)"
  }, {
    "name": "chembl Document Molecule",
    "type": "1",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_molecule)"
  }, {
    "name": "chembl Document Molecule Filter",
    "type": "1",
    "searchTerm": "CHEMBL3421631",
    "mapFilterTerm": "map(chembl_molecule).filter(chembl.molecule.heavyAtoms \u003c 30.0 \u0026\u0026 chembl.molecule.aromaticRings \u003c2.0)"
  }, {
    "name": "chembl Assay",
    "type": "0",
    "searchTerm": "CHEMBL615156",
    "mapFilterTerm": ""
  }, {
    "name": "chembl Assay Targets",
    "type": "1",
    "searchTerm": "CHEMBL615156",
    "mapFilterTerm": "map(chembl_target)"
  }, {
    "name": "chembl Assay Cell Line",
    "type": "1",
    "searchTerm": "CHEMBL3424821",
    "mapFilterTerm": "map(chembl_cell_line)"
  }, {
    "name": "chembl Assay Target Protein",
    "type": "1",
    "searchTerm": "CHEMBL615156",
    "mapFilterTerm": "map(chembl_target).filter(chembl.target.type==\"single_protein\")"
  }, {
    "name": "chembl Assay Target Protein Uniprot",
    "type": "1",
    "searchTerm": "CHEMBL615156",
    "mapFilterTerm": "map(chembl_target).map(chembl_target_component).map(uniprot)"
  }, {
    "name": "chembl Activity",
    "type": "0",
    "searchTerm": "CHEMBL_ACT_93229",
    "mapFilterTerm": ""
  }, {
    "name": "chembl Activity Filter",
    "type": "1",
    "searchTerm": "CHEMBL_ACT_93229",
    "mapFilterTerm": "filter(chembl.activity.bao==\"BAO_0000179\").map(chembl_molecule)"
  }, {
    "name": "chembl Target Component",
    "type": "0",
    "searchTerm": "CHEMBL_TC_47",
    "mapFilterTerm": ""
  }, {
    "name": "chembl Target",
    "type": "0",
    "searchTerm": "CHEMBL2242",
    "mapFilterTerm": ""
  }, {
    "name": "chembl Target Protein Uniprot",
    "type": "1",
    "searchTerm": "CHEMBL2789",
    "mapFilterTerm": "filter(chembl.target.type==\"single_protein\").map(chembl_target_component).map(uniprot)"
  }, {
    "name": "chembl Cell Line",
    "type": "0",
    "searchTerm": "CHEMBL3307241",
    "mapFilterTerm": ""
  }, {
    "name": "chembl Cell Line Assay",
    "type": "1",
    "searchTerm": "CHEMBL3307241",
    "mapFilterTerm": "map(chembl_assay)"
  }],
  "ensembl": [{
    "name": "ensembl entry",
    "type": "0",
    "searchTerm": "ENSG00000139618",
    "mapFilterTerm": ""
  }, {
    "name": "ensembl gene transcripts",
    "type": "1",
    "searchTerm": "tp53",
    "mapFilterTerm": "map(transcript)"
  }, {
    "name": "ensembl transcript by type",
    "type": "1",
    "searchTerm": "ENSG00000073910",
    "mapFilterTerm": "map(transcript).filter(transcript.biotype==\"protein_coding\")"
  }, {
    "name": "ensembl exons",
    "type": "1",
    "searchTerm": "ENSG00000141510",
    "mapFilterTerm": "map(transcript).map(exon)"
  }, {
    "name": "ensembl gene exons by region",
    "type": "1",
    "searchTerm": "tp53",
    "mapFilterTerm": "map(transcript).filter(transcript.biotype==\"protein_coding\").map(exon).filter(exon.seq_region_name==\"17\")"
  }, {
    "name": "ensembl gene exons by location",
    "type": "1",
    "searchTerm": "tp53",
    "mapFilterTerm": "map(transcript).filter(transcript.biotype==\"protein_coding\").map(exon).filter(exon.end \u003e= 7687538)"
  }, {
    "name": "ensembl Ortholog",
    "type": "1",
    "searchTerm": "ENSG00000139618",
    "mapFilterTerm": "map(ortholog)"
  }, {
    "name": "ensembl Paralog",
    "type": "1",
    "searchTerm": "ENSG00000073910",
    "mapFilterTerm": "map(paralog)"
  }],
  "protein": [{
    "name": "protein name",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "filter(uniprot.names.exists(a,a==\"Sonic hedgehog protein\"))"
  }, {
    "name": "protein sequence mass",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "filter(uniprot.sequence.mass \u003e 45000)"
  }, {
    "name": "protein sequence size",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "filter(size(uniprot.sequence.seq) \u003e 400)"
  }, {
    "name": "protein go term",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(go).filter(go.type==\"molecular_function\")"
  }, {
    "name": "protein go term cellular",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(go).filter(go.type==\"cellular_component\")"
  }, {
    "name": "protein go term boolean",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(go).filter(go.name.contains(\"binding\") || go.name.contains(\"activity\"))"
  }, {
    "name": "protein feature helix type",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.type==\"helix\")"
  }, {
    "name": "protein feature sequence variant",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.type==\"sequence variant\")"
  }, {
    "name": "protein feature location",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.location.begin\u003e0 \u0026\u0026 ufeature.location.end\u003c300)"
  }, {
    "name": "protein feature description",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.description.contains(\"cancer\"))"
  }, {
    "name": "protein feature description contains",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.description.contains(\"tumor\"))"
  }, {
    "name": "protein feature specific variant",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.original==\"I\" \u0026\u0026 ufeature.variation==\"S\")"
  }, {
    "name": "protein feature maps variantid",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.original==\"I\" \u0026\u0026 ufeature.variation==\"S\").map(variantid)"
  }, {
    "name": "protein feature has evidences",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(size(ufeature.evidences)\u003e1)"
  }, {
    "name": "protein feature has experimental evidence",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.evidences.exists(a,a.type==\"ECO:0000269\"))"
  }, {
    "name": "protein feature has pubmed evidence",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.evidences.exists(a,a.source==\"pubmed\"))"
  }, {
    "name": "protein feature pdb evidence",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ufeature).filter(ufeature.evidences.exists(a,a.source==\"pdb\"))"
  }, {
    "name": "protein ENA type mRNA",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ena).filter(ena.type==\"mrna\")"
  }, {
    "name": "protein ENA type genomic DNA",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(ena).filter(ena.type==\"genomic_dna\")"
  }, {
    "name": "protein pdb method NMR",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(pdb).filter(pdb.method==\"nmr\")"
  }, {
    "name": "protein pdb chains",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(pdb).filter(pdb.chains==\"A/C=95-292\")"
  }, {
    "name": "protein pdb resolution",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(pdb).filter(pdb.resolution==\"2.60 A\")"
  }, {
    "name": "protein pdb method or chains",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(pdb).filter(pdb.method==\"nmr\" || pdb.chains==\"C/D=1-177\")"
  }, {
    "name": "protein reactome activation pathways",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(reactome).filter(reactome.pathway.contains(\"activation\"))"
  }, {
    "name": "protein reactome signaling pathways",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(reactome).filter(reactome.pathway.contains(\"signaling\"))"
  }, {
    "name": "protein reactome regulation pathways",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(reactome).filter(reactome.pathway.contains(\"Regulation\"))"
  }, {
    "name": "protein orphanet disease name",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(orphanet).filter(orphanet.disease.contains(\"cancer\"))"
  }, {
    "name": "protein durgs by drugbank",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(drugbank)"
  }, {
    "name": "protein to refseqs",
    "type": "1",
    "searchTerm": "shh_human,P53_HUMAN",
    "mapFilterTerm": "map(refseq)"
  }],
  "taxonomy": [{
    "name": "taxonomy children",
    "type": "1",
    "searchTerm": "9606",
    "mapFilterTerm": "map(taxchild)"
  }, {
    "name": "taxonomy grand children",
    "type": "1",
    "searchTerm": "862507",
    "mapFilterTerm": "map(taxchild).map(taxchild)"
  }, {
    "name": "taxonomy grand^3 parent",
    "type": "1",
    "searchTerm": "10090",
    "mapFilterTerm": "map(taxparent).map(taxparent).map(taxparent)"
  }, {
    "name": "taxonomy Asian children",
    "type": "1",
    "searchTerm": "10090",
    "mapFilterTerm": "map(taxchild).filter(taxonomy.common_name.contains(\"Asian\"))"
  }, {
    "name": "taxonomy European children",
    "type": "1",
    "searchTerm": "10090",
    "mapFilterTerm": "map(taxchild).filter(taxonomy.common_name.contains(\"European\"))"
  }, {
    "name": "taxonomy grand children by division",
    "type": "1",
    "searchTerm": "862507",
    "mapFilterTerm": "map(taxchild).map(taxchild).filter(taxonomy.taxonomic_division==\"ROD\")"
  }]
};

export default UseCases;