 var UseCases1 = {
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
     "name": "mix kinase activity goterm to hgnc genes",
     "type": "1",
     "source": "",
     "searchTerm": "GO:0004707",
     "mapFilterTerm": "map(ensembl).map(hgnc)"
   }, {
     "name": "mix probe id to ensembl then hgnc by location",
     "type": "1",
     "source": "",
     "searchTerm": "202763_at,209310_s_at,207500_at",
     "mapFilterTerm": "map(transcript).map(ensembl).filter(ensembl.genome==\"homo_sapiens\").map(hgnc).filter(hgnc.location==\"4q35.1\")"
   }, {
     "name": "mix taxid to its grand children by division",
     "type": "1",
     "source": "",
     "searchTerm": "862507",
     "mapFilterTerm": "map(taxchild).map(taxchild).filter(taxonomy.taxonomic_division==\"ROD\")"
   }],
   "gene": [{
     "name": "gene search genes",
     "type": "0",
     "source": "",
     "searchTerm": "shh,tpi1",
     "mapFilterTerm": ""
   }, {
     "name": "gene genes to transcripts",
     "type": "1",
     "source": "",
     "searchTerm": "tpi1,ssh",
     "mapFilterTerm": "map(transcript)"
   }, {
     "name": "gene human genes to transcripts",
     "type": "1",
     "source": "",
     "searchTerm": "tpi1,ssh",
     "mapFilterTerm": "filter(ensembl.genome==\"homo_sapiens\").map(transcript)"
   }, {
     "name": "gene human genes in a region",
     "type": "1",
     "source": "",
     "searchTerm": "homo sapiens",
     "mapFilterTerm": "map(ensembl).filter(ensembl.seq_region==\"7\")"
   }, {
     "name": "gene human genes in overlapping range",
     "type": "1",
     "source": "",
     "searchTerm": "homo sapiens",
     "mapFilterTerm": "map(ensembl).filter(ensembl.overlaps(114129278,114129328))"
   }, {
     "name": "gene human genes only within range",
     "type": "1",
     "source": "",
     "searchTerm": "homo sapiens",
     "mapFilterTerm": "map(ensembl).filter(ensembl.within(110000000,114000000))"
   }, {
     "name": "gene human genes only within range and chromosome",
     "type": "1",
     "source": "",
     "searchTerm": "homo sapiens",
     "mapFilterTerm": "map(ensembl).filter(ensembl.within(110000000,114000000) \u0026\u0026 ensembl.seq_region==\"7\")"
   }, {
     "name": "gene genes to golden transcript",
     "type": "1",
     "source": "",
     "searchTerm": "tpi1,ssh",
     "mapFilterTerm": "map(transcript).filter(transcript.source==\"eh\")"
   }, {
     "name": "gene human golden transcripts in overlapping range",
     "type": "1",
     "source": "",
     "searchTerm": "homo sapiens",
     "mapFilterTerm": "map(ensembl).map(transcript).filter(transcript.source==\"eh\" \u0026\u0026  transcript.overlaps(114129278,114129328))"
   }, {
     "name": "gene human transcripts within range and region",
     "type": "1",
     "source": "",
     "searchTerm": "homo sapiens",
     "mapFilterTerm": "map(ensembl).map(transcript).filter(transcript.seq_region==\"12\" \u0026\u0026 transcript.within(110000000,114000000))"
   }, {
     "name": "gene human transcripts covering a genome point",
     "type": "1",
     "source": "",
     "searchTerm": "homo sapiens",
     "mapFilterTerm": "map(ensembl).map(transcript).filter(transcript.covers(110000000))"
   }, {
     "name": "gene human transcripts not protein_coding biotype",
     "type": "1",
     "source": "",
     "searchTerm": "homo sapiens",
     "mapFilterTerm": "map(ensembl).map(transcript).filter(transcript.biotype!=\"protein_coding\")"
   }, {
     "name": "gene mouse transcripts in a region",
     "type": "1",
     "source": "",
     "searchTerm": "mus musculus",
     "mapFilterTerm": "map(ensembl).map(transcript).filter(transcript.seq_region==\"3\")"
   }, {
     "name": "gene ensembl id to transcripts by type",
     "type": "1",
     "source": "",
     "searchTerm": "ENSG00000073910",
     "mapFilterTerm": "map(transcript).filter(transcript.biotype==\"protein_coding\")"
   }, {
     "name": "gene cancer related genes to transcripts",
     "type": "1",
     "source": "",
     "searchTerm": "MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
     "mapFilterTerm": "filter(ensembl.genome==\"homo_sapiens\").map(transcript)"
   }, {
     "name": "gene genes to CDS",
     "type": "1",
     "source": "",
     "searchTerm": "tpi1,shh",
     "mapFilterTerm": "map(transcript).map(cds)"
   }, {
     "name": "gene human  golden CDS in a region",
     "type": "1",
     "source": "",
     "searchTerm": "homo sapiens",
     "mapFilterTerm": "map(ensembl).map(transcript).filter(transcript.source==\"eh\").map(cds).filter(cds.seq_region==\"X\")"
   }, {
     "name": "gene ensembl id to exons",
     "type": "1",
     "source": "",
     "searchTerm": "ENSG00000141510",
     "mapFilterTerm": "map(transcript).map(exon)"
   }, {
     "name": "gene gene to exons by region",
     "type": "1",
     "source": "",
     "searchTerm": "tp53",
     "mapFilterTerm": "map(transcript).filter(transcript.seq_region==\"17\").map(exon)"
   }, {
     "name": "gene genes to overlapping range exons",
     "type": "1",
     "source": "",
     "searchTerm": "tpi1,tp53",
     "mapFilterTerm": "map(transcript).map(exon).filter(exon.overlaps(6867119,6867681))"
   }, {
     "name": "gene genes to within range exons",
     "type": "1",
     "source": "",
     "searchTerm": "tpi1,tp53",
     "mapFilterTerm": "map(transcript).map(exon).filter(exon.within(6867531,6867681))"
   }, {
     "name": "gene golden transcript exons with + strand",
     "type": "1",
     "source": "",
     "searchTerm": "tp53",
     "mapFilterTerm": "map(transcript).filter(transcript.source==\"eh\").map(exon).filter(exon.strand == \"+\")"
   }, {
     "name": "gene ensembl id to Entrez id",
     "type": "1",
     "source": "",
     "searchTerm": "ENSG00000139618",
     "mapFilterTerm": "map(entrez)"
   }, {
     "name": "gene entrez id to ensembl",
     "type": "1",
     "source": "",
     "searchTerm": "675",
     "mapFilterTerm": "map(ensembl)"
   }, {
     "name": "gene genes to molecular goterm",
     "type": "1",
     "source": "",
     "searchTerm": "tpi1,shh",
     "mapFilterTerm": "filter(ensembl.genome==\"homo_sapiens\").map(go).filter(go.type==\"molecular_function\")"
   }, {
     "name": "gene genes by genomes then go terms",
     "type": "1",
     "source": "",
     "searchTerm": "tpi1,tp53",
     "mapFilterTerm": "filter(ensembl.genome==\"mus_musculus\").map(go).filter(go.type==\"biological_process\")"
   }, {
     "name": "gene cancer related genes to uniprot",
     "type": "1",
     "source": "",
     "searchTerm": "PMS2,MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
     "mapFilterTerm": "filter(ensembl.genome==\"mus_musculus\").map(uniprot)"
   }, {
     "name": "gene cancer related genes to uniprot go terms",
     "type": "1",
     "source": "",
     "searchTerm": "PMS2,MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
     "mapFilterTerm": "filter(ensembl.genome==\"homo_sapiens\").map(uniprot).map(go)"
   }, {
     "name": "gene cancer related genes to uniprot via hgnc",
     "type": "1",
     "source": "hgnc",
     "searchTerm": "PMS2,MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
     "mapFilterTerm": "map(uniprot).filter(uniprot.reviewed)"
   }, {
     "name": "gene cancer related genes to uniprot go terms via hgnc",
     "type": "1",
     "source": "hgnc",
     "searchTerm": "PMS2,MLH1,MSH2,MSH6,STK11,BMPR1A,SMAD4,BRCA1,BRCA2,TP53,PTEN,PALB2,TSC1,TSC2,FLCN,MET,CDKN2A,RB1",
     "mapFilterTerm": "map(uniprot).filter(uniprot.reviewed).map(go).filter(go.type==\"cellular_component\")"
   }, {
     "name": "gene probe id to gene",
     "type": "1",
     "source": "",
     "searchTerm": "202763_at,209310_s_at,207500_at",
     "mapFilterTerm": "map(transcript).map(ensembl).filter(ensembl.genome==\"homo_sapiens\")"
   }, {
     "name": "gene probe id to transcript",
     "type": "1",
     "source": "",
     "searchTerm": "202763_at,209310_s_at,207500_at",
     "mapFilterTerm": "map(transcript)"
   }, {
     "name": "gene transcript to probe id",
     "type": "1",
     "source": "",
     "searchTerm": "ENST00000308394",
     "mapFilterTerm": "map(affy_hg_focus)"
   }, {
     "name": "gene gene to probe id",
     "type": "1",
     "source": "",
     "searchTerm": "tpi1,tp53",
     "mapFilterTerm": "map(transcript).map(affy_hg_focus)"
   }, {
     "name": "gene probe id to ensembl then hgnc",
     "type": "1",
     "source": "",
     "searchTerm": "202763_at,209310_s_at,207500_at",
     "mapFilterTerm": "map(transcript).map(ensembl).map(hgnc)"
   }, {
     "name": "gene ensembl with region then uniprot reviewed",
     "type": "1",
     "source": "",
     "searchTerm": "homo_sapiens",
     "mapFilterTerm": "map(ensembl).filter(ensembl.seq_region==\"X\").map(uniprot).filter(uniprot.reviewed)"
   }, {
     "name": "gene ensembl id to orthologs",
     "type": "1",
     "source": "",
     "searchTerm": "ENSG00000139618",
     "mapFilterTerm": "map(ortholog)"
   }, {
     "name": "gene gene orthologs",
     "type": "1",
     "source": "",
     "searchTerm": "shh",
     "mapFilterTerm": "filter(ensembl.genome==\"homo_sapiens\").map(ortholog)"
   }, {
     "name": "gene ensembl id to paralog",
     "type": "1",
     "source": "",
     "searchTerm": "ENSG00000073910",
     "mapFilterTerm": "map(paralog)"
   }, {
     "name": "gene gene to Paralog",
     "type": "1",
     "source": "",
     "searchTerm": "FRY",
     "mapFilterTerm": "filter(ensembl.genome==\"homo_sapiens\").map(paralog)"
   }, {
     "name": "gene gene name to paralog transcripts",
     "type": "1",
     "source": "",
     "searchTerm": "FRY",
     "mapFilterTerm": "filter(ensembl.genome==\"homo_sapiens\").map(paralog).map(transcript)"
   }, {
     "name": "gene refseq to interpro family",
     "type": "1",
     "source": "",
     "searchTerm": "NM_005359,NM_000546",
     "mapFilterTerm": "map(hgnc).map(uniprot).map(interpro).filter(interpro.type==\"Family\")"
   }, {
     "name": "gene refseq to interpro domain",
     "type": "1",
     "source": "",
     "searchTerm": "NM_005359,NM_000546",
     "mapFilterTerm": "map(hgnc).map(uniprot).map(interpro).filter(interpro.type==\"Domain\")"
   }, {
     "name": "gene ensembl human genes with MAP kinase activity",
     "type": "1",
     "source": "",
     "searchTerm": "GO:0004707",
     "mapFilterTerm": "map(ensembl).filter(ensembl.branch==1 \u0026\u0026 ensembl.genome==\"homo_sapiens\")"
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

 export default UseCases1;