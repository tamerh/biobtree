"""Meta-graph (schema view): biolink category --predicate--> category, derived
from the mapping tables (categories.yaml + predicates.yaml + the GO rules). Shows
the big-picture shape of the KG -- what node types exist and how they connect --
independent of any instance data.

    python -m kg_export.metaschema --out kg_meta.html [--print]
"""

from __future__ import annotations

import argparse
from collections import defaultdict

from .categories import CategoryMap
from .curie import to_curie
from .datasets import DatasetRegistry
from .go import ASPECT_CATEGORY
from .ontology import _ontology_targets
from .predicates import PredicateMap
from pathlib import Path as _MSP
_MS_MAP = _MSP(__file__).resolve().parent / "mappings"

# GO annotation sources (go.py): subject dataset -> category, and the 3 aspects.
_GO_SOURCES = [("uniprot", "biolink:Protein"), ("ensembl", "biolink:Gene")]
_GO_EDGES = [
    ("biolink:enables", "biolink:MolecularActivity"),
    ("biolink:actively_involved_in", "biolink:BiologicalProcess"),
    ("biolink:located_in", "biolink:CellularComponent"),
]

# RefSeq (refseq.py): one dataset split by accession type into 3 node categories,
# plus its typed cross-references. Kept here so the meta-graph reflects it.
_REFSEQ_NODE_CATS = (
    "biolink:Transcript", "biolink:NoncodingRNAProduct", "biolink:Protein",
)
_REFSEQ_EDGES = [
    ("biolink:Transcript", "biolink:transcribed_from", "biolink:Gene"),
    ("biolink:NoncodingRNAProduct", "biolink:transcribed_from", "biolink:Gene"),
    ("biolink:Transcript", "biolink:translates_to", "biolink:Protein"),
    ("biolink:Gene", "biolink:has_gene_product", "biolink:Protein"),
    ("biolink:Protein", "biolink:same_as", "biolink:Protein"),
    ("biolink:Transcript", "biolink:in_taxon", "biolink:OrganismTaxon"),
    ("biolink:NoncodingRNAProduct", "biolink:in_taxon", "biolink:OrganismTaxon"),
    ("biolink:Protein", "biolink:in_taxon", "biolink:OrganismTaxon"),
]

# Ontology cross-ontology close_match (ontology.py) is emitted only by the hub
# ontologies that carry the cross-references: MONDO (disease merge ontology) and
# uPheno (cross-species phenotype hub). subclass_of contributors are derived per
# ontology from the registry (see schema_triples), so the meta-graph panel shows
# the real dataset names (mondo/doid/efo/...) instead of a generic "ontology".
_ONTOLOGY_CLOSEMATCH_SOURCES = {
    "biolink:Disease": ["mondo"],
    "biolink:PhenotypicFeature": ["upheno"],
}

# Structure edges emitted by structure.py (not via predicate pairs): cds->protein
# coding link and protein->feature containment. transcript has_part exon/cds come
# from the `transcript>exon`/`transcript>cds` direct pairs (already in pairs()).
_STRUCTURE_EDGES = [
    ("biolink:CodingSequence", "biolink:translates_to", "biolink:Protein", "cds"),
    ("biolink:Protein", "biolink:has_part", "biolink:ProteinDomain", "ufeature"),
]

# dbSNP layer (built by src/scripts/kg_export/dbsnp_py/extract.py, not a predicate pair): variant->gene
# is already shown via the dbsnp>entrez pair; the variant->transcript consequence edge
# isn't pair-derived, so surface it here.
_DBSNP_EDGES = [
    ("biolink:SequenceVariant", "biolink:is_sequence_variant_of", "biolink:Transcript", "dbsnp"),
]

# explorer enrichment ------------------------------------------------------------
# one-sentence plain-English blurb per category (keyed by short name); orients a
# non-biolink visitor. Missing keys simply show no blurb.
_BLURBS = {
    "Gene": "A region of genome that encodes a functional product.",
    "Protein": "A gene product; a sequence of amino acids.",
    "Transcript": "An RNA molecule transcribed from a gene.",
    "NoncodingRNAProduct": "A non-protein-coding RNA product.",
    "SequenceVariant": "A position where a genome differs from the reference (e.g. an SNP).",
    "Disease": "A disorder of normal body function.",
    "PhenotypicFeature": "An observable characteristic or trait.",
    "SmallMolecule": "A low-molecular-weight chemical compound.",
    "Drug": "A chemical used to treat, cure, or prevent disease.",
    "Pathway": "A series of molecular interactions in a cell.",
    "MolecularActivity": "A molecular-level function (GO molecular function).",
    "BiologicalProcess": "A biological objective achieved by molecular events (GO).",
    "CellularComponent": "A location in the cell where a gene product acts (GO).",
    "Cell": "A cell type.",
    "CellLine": "An immortalized population of cells used in research.",
    "GrossAnatomicalStructure": "An anatomical structure (tissue/organ).",
    "OrganismTaxon": "A taxonomic group of organisms.",
    "ProteinDomain": "A structural/functional region of a protein.",
    "Exon": "A coding/expressed segment of a transcript.",
    "CodingSequence": "The protein-coding portion of a transcript.",
    "MicroRNA": "A small regulatory non-coding RNA.",
    "RegulatoryRegion": "A genomic region that regulates transcription.",
    "NucleicAcidSequenceMotif": "A recurring nucleotide pattern (e.g. a TF binding motif).",
    "Publication": "A scientific publication.",
    "ChemicalEntity": "A chemical substance.",
}
# a representative example local id per dataset (-> a full CURIE in the panel)
_EXAMPLE_LOCAL = {
    "hgnc": "1100", "uniprot": "P38398", "ensembl": "ENSG00000139618", "entrez": "672",
    "transcript": "ENST00000357654", "chembl_molecule": "CHEMBL25", "chebi": "15365",
    "drugbank": "DB00945", "mondo": "0007254", "doid": "1612", "efo": "0000305",
    "hpo": "0000118", "orphanet": "145", "mim": "604370", "reactome": "R-HSA-68886",
    "go": "GO:0006915", "uberon": "0002107", "cl": "0000236", "cellosaurus": "CVCL_0031",
    "interpro": "IPR001357", "drugcentral": "100", "pubchem": "2244", "corum": "351",
}
# predicates whose edges carry ECO evidence / qualifiers in the KG (for panel badges)
_EVIDENCE_PREDS = {
    "biolink:enables", "biolink:actively_involved_in", "biolink:located_in",
    "biolink:participates_in", "biolink:has_participant", "biolink:has_part",
}
_QUALIFIER_PREDS = {
    "biolink:interacts_with", "biolink:affects", "biolink:related_to",
    "biolink:gene_associated_with_condition", "biolink:associated_with",
    "biolink:in_clinical_trials_for", "biolink:treats_or_applied_or_studied_to_treat",
}

_PALETTE = [
    "#4e79a7", "#f28e2b", "#59a14f", "#e15759", "#76b7b2", "#edc948",
    "#b07aa1", "#ff9da7", "#9c755f", "#bab0ac", "#86bc86", "#d37295",
    "#7f7f7f", "#17becf", "#bcbd22", "#aec7e8", "#ffbb78", "#98df8a",
]


def schema_triples(cats: CategoryMap, preds: PredicateMap, registry: DatasetRegistry | None = None):
    """(subject_category, predicate, object_category) -> set of contributing datasets."""
    triples: dict[tuple, set] = defaultdict(set)

    # runtime-typed datasets have no categories.yaml entry; supply the category an
    # edge endpoint to them takes at the schema level (e.g. miRDB targets refseq
    # transcripts). go is multi-aspect so left out (its edges come via _GO_EDGES).
    runtime_cat = {"refseq": "biolink:Transcript"}

    def cat_of(ds):
        return cats.category_for(ds) or runtime_cat.get(ds)

    def add(sc, p, oc, ds):
        if sc and oc:
            triples[(sc, p, oc)].add(ds)

    for key in preds.pairs():
        s, o = key.split(">")
        r = preds.rule_for(s, o)
        if r.is_skip:
            continue
        if r.flip:
            s, o = o, s
        add(cats.category_for(s), r.predicate, cats.category_for(o), f"{key}")

    for ds in preds.reified_datasets():
        r = preds.reified_rule(ds)
        if r.kind in ("pairwise", "star"):
            c = cats.category_for(r.partner)
            add(c, r.predicate, c, ds)
        else:  # bipartite (object resolved via `via`/symbol is still rule.object)
            add(cat_of(r.subject), r.predicate, cat_of(r.object), ds)
            for extra in (r.extra_objects or []):
                add(cat_of(r.subject), r.predicate, cat_of(extra), ds)

    for src_ds, sc in _GO_SOURCES:
        for p, oc in _GO_EDGES:
            add(sc, p, oc, f"go({src_ds})")

    for sc, p, oc in _REFSEQ_EDGES:
        add(sc, p, oc, "refseq")

    for sc, p, oc, ds in _STRUCTURE_EDGES:
        add(sc, p, oc, ds)

    for sc, p, oc, ds in _DBSNP_EDGES:
        add(sc, p, oc, ds)

    # Ontology hierarchy (ontology.py): attribute each subclass_of self-loop to
    # the real source ontologies, and close_match to the hub ontologies.
    if registry is not None:
        for ds, _prefix, category, _parent_id in _ontology_targets(registry, cats):
            if ds == "go":
                for aspect_cat in ASPECT_CATEGORY.values():
                    add(aspect_cat, "biolink:subclass_of", aspect_cat, "go")
            else:
                add(category, "biolink:subclass_of", category, ds)
        for cat, sources in _ONTOLOGY_CLOSEMATCH_SOURCES.items():
            for ds in sources:
                add(cat, "biolink:close_match", cat, ds)
    return triples


def render_html(triples, out_html):
    from pyvis.network import Network
    cats = sorted({c for (s, _, o) in triples for c in (s, o)})
    color = {c: _PALETTE[i % len(_PALETTE)] for i, c in enumerate(cats)}
    net = Network(height="900px", width="100%", directed=True, bgcolor="#ffffff")
    net.barnes_hut(spring_length=240)
    for c in cats:
        net.add_node(c, label=c.split(":")[1], title=c, color=color[c], size=24)
    for (s, p, o), ds in sorted(triples.items()):
        net.add_edge(s, o, label=p.split(":")[1], title=f"{p}  ({len(ds)} datasets: {', '.join(sorted(ds))})", arrows="to")
    net.write_html(out_html, notebook=False, open_browser=False)


def render_mermaid(triples, out_html):
    """A clean left-to-right Mermaid diagram (layered, readable)."""
    def nid(cat):
        return cat.split(":")[1]
    cats = sorted({c for (s, _, o) in triples for c in (s, o)})
    lines = ["graph LR"]
    for c in cats:
        lines.append(f'  {nid(c)}["{nid(c)}"]')
    for (s, p, o), ds in sorted(triples.items()):
        lines.append(f"  {nid(s)} -->|{p.split(':')[1]}| {nid(o)}")
    # highlight the two hubs
    lines.append("  classDef hub fill:#e15759,stroke:#900,color:#fff;")
    lines.append("  class Gene,Protein hub;")
    mermaid = "\n".join(lines)
    html = (
        "<!doctype html><html><head><meta charset='utf-8'>"
        "<script src='https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.min.js'></script>"
        "<style>body{font-family:sans-serif;margin:0} .mermaid{width:100%}</style></head>"
        "<body><h3 style='padding:8px'>BioBTree KG schema (node type —predicate→ node type)</h3>"
        f"<pre class='mermaid'>\n{mermaid}\n</pre>"
        "<script>mermaid.initialize({startOnLoad:true,maxTextSize:200000,"
        "flowchart:{useMaxWidth:false,rankSpacing:90,nodeSpacing:50}});</script>"
        "</body></html>"
    )
    with open(out_html, "w") as f:
        f.write(html)


def render_cytoscape(triples, out_html):
    """Interactive Cytoscape.js viewer: layered (dagre)/concentric/force layouts,
    predicate filter, click-to-highlight, colored by node type."""
    import json
    def nid(c):
        return c.split(":")[1]
    cats = sorted({c for (s, _, o) in triples for c in (s, o)})
    color = {nid(c): _PALETTE[i % len(_PALETTE)] for i, c in enumerate(cats)}
    nodes = [{"data": {"id": nid(c), "label": nid(c), "color": color[nid(c)]}} for c in cats]
    edges = []
    for (s, p, o), ds in sorted(triples.items()):
        pl = p.split(":")[1]
        edges.append({"data": {"id": f"{nid(s)}|{pl}|{nid(o)}", "source": nid(s),
                                "target": nid(o), "label": pl, "n": len(ds)}})
    elements = json.dumps({"nodes": nodes, "edges": edges})
    html = """<!doctype html><html><head><meta charset='utf-8'>
<script src='https://unpkg.com/cytoscape@3.28.1/dist/cytoscape.min.js'></script>
<script src='https://unpkg.com/dagre@0.8.5/dist/dagre.min.js'></script>
<script src='https://unpkg.com/cytoscape-dagre@2.5.0/cytoscape-dagre.js'></script>
<style>html,body{margin:0;height:100%;font-family:sans-serif}
#cy{width:100%;height:calc(100% - 44px);background:#fafafa}
#bar{height:44px;display:flex;gap:8px;align-items:center;padding:0 10px;border-bottom:1px solid #ddd}
button{padding:4px 10px;cursor:pointer}</style></head>
<body><div id='bar'>
<b>BioBTree KG schema</b>
<button onclick="lay('dagre-LR')">Layered &rarr;</button>
<button onclick="lay('dagre-TB')">Layered &darr;</button>
<button onclick="lay('concentric')">Hubs center</button>
<button onclick="lay('cose')">Force</button>
<span style='color:#888'>click a node to focus its connections; double-click bg to reset</span>
</div><div id='cy'></div>
<script>
var ELEMENTS=__ELEMENTS__;
var cy=cytoscape({container:document.getElementById('cy'),elements:ELEMENTS,
 style:[
  {selector:'node',style:{'background-color':'data(color)','label':'data(label)','font-size':11,'text-valign':'center','color':'#111','text-outline-color':'#fff','text-outline-width':2,'width':42,'height':42}},
  {selector:'edge',style:{'label':'data(label)','font-size':8,'color':'#555','curve-style':'bezier','target-arrow-shape':'triangle','line-color':'#bbb','target-arrow-color':'#bbb','width':1.2,'text-rotation':'autorotate','text-background-color':'#fff','text-background-opacity':0.8}},
  {selector:'.faded',style:{'opacity':0.12}},
  {selector:'.hi',style:{'line-color':'#e15759','target-arrow-color':'#e15759','width':2.5,'color':'#900'}}
 ]});
function lay(name){var o={name:'cose'};
 if(name=='dagre-LR')o={name:'dagre',rankDir:'LR',nodeSep:40,rankSep:140};
 if(name=='dagre-TB')o={name:'dagre',rankDir:'TB',nodeSep:40,rankSep:110};
 if(name=='concentric')o={name:'concentric',concentric:function(n){return n.degree()},levelWidth:function(){return 2},minNodeSpacing:40};
 cy.layout(o).run();}
cy.on('tap','node',function(e){var n=e.target;cy.elements().addClass('faded');
 var nb=n.closedNeighborhood();nb.removeClass('faded');nb.edges().addClass('hi');n.removeClass('faded');});
cy.on('tap',function(e){if(e.target===cy){cy.elements().removeClass('faded hi');}});
lay('dagre-LR');
</script></body></html>"""
    with open(out_html, "w") as f:
        f.write(html.replace("__ELEMENTS__", elements))


def _dataset_homepage(registry, ds):
    """A homepage-ish URL for a dataset from its conf url template (strip the
    £{id} deep-link placeholder)."""
    if registry is None:
        return None
    d = registry.by_name(ds)
    tpl = (d.raw.get("url") if d else None) or None
    if not tpl:
        return None
    base = tpl.split("£{")[0].rstrip("/?;#&=")
    if base.startswith("//"):
        base = "https:" + base
    return base or None


def render_explorer(triples, out_html, catmap=None, primary_names=None, registry=None):
    """Public schema explorer: Graph (edges on node click) + Matrix, with a rich
    right panel (datasets primary/cross-reference, CURIE + example, relationships
    with evidence/qualifier badges). The published "face" of the BioBTree KG.
    """
    import json
    primary_names = set(primary_names or ())
    primary_names |= {"go", "refseq", "dbsnp", "mesh"}            # runtime -> named nodes
    primary_names -= {"ortholog", "paralog", "literature_mappings"}  # xref-only despite source1

    def nid(c):
        return c.split(":")[1]
    cats = sorted({c for (s, _, o) in triples for c in (s, o)})
    color = {nid(c): _PALETTE[i % len(_PALETTE)] for i, c in enumerate(cats)}
    nodes = [{"data": {"id": nid(c), "label": nid(c), "color": color[nid(c)]}} for c in cats]
    edges = []
    for (s, p, o), ds in sorted(triples.items()):
        pl = p.split(":")[1]
        edges.append({"data": {"id": f"{nid(s)}|{pl}|{nid(o)}", "source": nid(s),
                                "target": nid(o), "label": pl, "n": len(ds),
                                "datasets": sorted(ds),
                                "ev": p in _EVIDENCE_PREDS, "ql": p in _QUALIFIER_PREDS}})
    # gene-linking datasets carry gene<->gene EDGES, not nodes -> not in the panel
    link_only = {"ortholog", "paralog", "orthologentrez", "relatedentrez", "neighborentrez"}

    def entry(ds, prefix):
        ex = _EXAMPLE_LOCAL.get(ds)
        example = (ex if ":" in (ex or "") else to_curie(prefix, ex)) if ex else None
        return {"ds": ds, "prefix": prefix, "primary": ds in primary_names,
                "example": example, "url": _dataset_homepage(registry, ds)}
    node_ds = defaultdict(list)
    if catmap is not None:
        for ds in sorted(catmap.datasets()):
            if ds in link_only:
                continue
            e = catmap.entry_for(ds)
            if e:
                node_ds[nid(e.category)].append(entry(ds, e.prefix))
    for aspect_cat in ("biolink:MolecularActivity", "biolink:BiologicalProcess",
                       "biolink:CellularComponent"):
        node_ds[nid(aspect_cat)].append(entry("go", "GO"))
    for rs_cat in _REFSEQ_NODE_CATS:
        node_ds[nid(rs_cat)].append(entry("refseq", "refseq"))
    n_datasets = len({e["ds"] for v in node_ds.values() for e in v})
    payload = json.dumps({"nodes": nodes, "edges": edges, "cats": [nid(c) for c in cats],
                          "colors": color, "nodeDatasets": node_ds, "blurbs": _BLURBS,
                          "meta": {"cats": len(cats), "edges": len(edges), "datasets": n_datasets}})
    tmpl = r"""<!doctype html><html lang='en'><head><meta charset='utf-8'>
<meta name='viewport' content='width=device-width,initial-scale=1'>
<title>BioBTree KG — schema explorer</title>
<meta name='description' content='Interactive schema of the BioBTree biolink knowledge graph: biolink node categories, typed relationships, and the datasets behind them.'>
<link rel='icon' href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='7' fill='%234e79a7'/%3E%3Ccircle cx='10' cy='11' r='3.2' fill='%23fff'/%3E%3Ccircle cx='22' cy='10' r='3.2' fill='%23f28e2b'/%3E%3Ccircle cx='16' cy='23' r='3.2' fill='%2359a14f'/%3E%3Cpath d='M10 11 L22 10 M22 10 L16 23 M16 23 L10 11' stroke='%23fff' stroke-width='1.4' opacity='.7'/%3E%3C/svg%3E">
<meta name='theme-color' content='#1f3a5f'>
<script src='https://unpkg.com/cytoscape@3.28.1/dist/cytoscape.min.js'></script>
<script src='https://unpkg.com/dagre@0.8.5/dist/dagre.min.js'></script>
<script src='https://unpkg.com/cytoscape-dagre@2.5.0/cytoscape-dagre.js'></script>
<style>
html,body{margin:0;height:100%;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:13px;color:#2c3744;-webkit-font-smoothing:antialiased}
body{display:flex;flex-direction:column;height:100vh}
#hdr{display:flex;align-items:center;justify-content:space-between;gap:12px;height:48px;padding:0 16px;background:#1f3a5f;color:#eaf0f6;flex-wrap:wrap;flex:none}
#hdr .brand{display:flex;align-items:center;gap:10px;min-width:0}
#hdr .logo{width:22px;height:22px;border-radius:5px;flex:none;background:conic-gradient(#4e79a7 0 25%,#f28e2b 0 50%,#59a14f 0 75%,#76b7b2 0)}
#hdr .wm{font-size:16px;font-weight:600;white-space:nowrap}#hdr .wm b{color:#ffbb78}
#hdr .tag{font-size:12px;color:#9fb3c8;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
#hdr .links{display:flex;align-items:center;gap:14px;font-size:12px;white-space:nowrap}
#hdr .stat{color:#9fb3c8}#hdr .links a{color:#cfe0f0;text-decoration:none;border-bottom:1px solid transparent}
#hdr .links a:hover{border-bottom-color:#cfe0f0}
#bar{display:flex;gap:14px;align-items:center;padding:6px 16px;background:#f7f9fb;border-bottom:1px solid #e2e8ee;flex-wrap:wrap;flex:none}
#bar .grp{display:flex;gap:6px;align-items:center}
#bar .grp>label{font-size:11px;text-transform:uppercase;letter-spacing:.05em;color:#8a99a8}
.seg{display:inline-flex;border:1px solid #cdd7e1;border-radius:6px;overflow:hidden}
.seg button{border:0;border-left:1px solid #cdd7e1;background:#fff;color:#34495e;padding:5px 11px;font-size:12px;cursor:pointer;transition:background .12s}
.seg button:first-child{border-left:0}.seg button:hover{background:#eef3f8}.seg button.on{background:#4e79a7;color:#fff}
#bar input{padding:5px 8px;border:1px solid #cdd7e1;border-radius:6px;font-size:12px;width:150px}
#bar .tbtn{border:1px solid #cdd7e1;border-radius:6px;background:#fff;color:#34495e;padding:5px 10px;font-size:12px;cursor:pointer}
#bar .tbtn:hover{background:#eef3f8}#bar .tbtn.on{background:#4e79a7;color:#fff}
#bar .tip{font-size:11px;color:#9aa7b3;margin-left:auto}
#legend{display:flex;gap:18px;align-items:center;flex-wrap:wrap;padding:5px 16px;background:#fff;border-bottom:1px solid #eef2f6;font-size:11.5px;color:#5b6b7a;flex:none}
#legend i{display:inline-block;vertical-align:middle;margin-right:4px}
#legend .dot{width:11px;height:11px;border-radius:50%;background:conic-gradient(#4e79a7,#f28e2b,#59a14f,#76b7b2,#4e79a7)}
#legend .arr{color:#c0392b;font-weight:700}
#legend .ch{width:14px;height:9px;border-radius:2px}
#legend .ch-p{background:#dfe7ef;border:1px solid #b9c6d4}
#legend .ch-x{background:repeating-linear-gradient(45deg,#eef2f6 0 3px,#fff 3px 6px);border:1px dashed #c4cfd9}
#legend .lk{background:none;border:0;color:#4e79a7;cursor:pointer;font-size:11.5px;text-decoration:underline;padding:0;margin-left:auto}
#about{display:none;padding:9px 16px;background:#f7f9fb;border-bottom:1px solid #eef2f6;font-size:12.5px;line-height:1.5;color:#3c4b59;flex:none}
#about.open{display:block}#about p{margin:0;max-width:900px}#about a{color:#4e79a7}
#main{flex:1;min-height:0;display:flex}
#left{flex:1;position:relative;min-width:0}
#cy{width:100%;height:100%;background:#fafafa}
#catlegend{position:absolute;top:8px;right:8px;max-height:62%;overflow:auto;background:#ffffffee;border:1px solid #dde3ea;border-radius:6px;padding:6px 9px;font-size:11px;z-index:20;display:none;box-shadow:0 1px 4px #0001}
#catlegend .lg{cursor:pointer;white-space:nowrap;padding:1.5px 0}#catlegend .lg.dim{opacity:.35;text-decoration:line-through}
#catlegend .sw{display:inline-block;width:11px;height:11px;border-radius:2px;margin-right:5px;vertical-align:middle}
#catlegend .ch{font-size:10px;color:#888;cursor:pointer;border-bottom:1px dotted #aaa;margin-bottom:3px;display:inline-block}
#onboard{position:absolute;left:50%;top:14px;transform:translateX(-50%);background:#333;color:#fff;padding:6px 13px;border-radius:14px;font-size:12px;z-index:25;opacity:.93;transition:opacity .3s}
#loading{position:absolute;inset:0;z-index:30;display:flex;gap:10px;align-items:center;justify-content:center;background:#fafafa;color:#7a8794;font-size:13px;transition:opacity .4s}
#loading.gone{opacity:0;pointer-events:none}
.spin{width:18px;height:18px;border:3px solid #d6dee6;border-top-color:#4e79a7;border-radius:50%;animation:sp .8s linear infinite}
@keyframes sp{to{transform:rotate(360deg)}}
#matrix{position:absolute;inset:0;box-sizing:border-box;overflow:auto;padding:10px;display:none;background:#fff}
#side{width:360px;border-left:1px solid #e2e8ee;overflow:auto;padding:14px 16px;box-sizing:border-box;background:#fff}
table{border-collapse:collapse}td,th{border:1px solid #eee;padding:2px 5px;font-size:11px;text-align:center}
th.rot{height:120px;white-space:nowrap}th.rot div{transform:rotate(-60deg);width:18px}
td.rh{text-align:right;font-weight:bold;white-space:nowrap}td.cell{cursor:pointer}.muted{color:#bbb}
#side h2{font-size:15px;margin:0 0 4px;display:flex;align-items:center;gap:6px;border-bottom:1px solid #eee;padding-bottom:6px}
#side h3{font-size:11px;color:#8a99a8;text-transform:uppercase;letter-spacing:.04em;margin:12px 0 4px;border-top:1px dashed #eee;padding-top:8px}
#side .hint{color:#9aa7b3;font-size:12px;margin-top:6px}
.blurb{color:#444;font-size:12.5px;line-height:1.45;margin:4px 0 6px;padding-left:9px;border-left:3px solid #ccc}
.curie{font-family:monospace;font-size:11px;color:#555;margin:2px 0 4px}.curie .ex{color:#999}
.counts{font-size:11px;color:#8a99a8;margin:2px 0 6px}
.ds{display:block;font-size:11px;background:#f3f5f8;border:1px solid #e6eaef;border-radius:4px;padding:3px 7px;margin:3px 0;text-decoration:none;color:inherit}
a.ds:hover{background:#e8eef5;border-color:#cdd7e1}.ds b{font-family:monospace}.ds i{color:#2c8a4a;font-style:normal;font-family:monospace;margin-left:6px}
.ds .ex{display:block;color:#9aa7b3;font-size:10px;font-family:monospace;margin-top:1px}
.rel{margin:4px 0;padding:5px 7px;border-radius:4px;background:#fafafa;border:1px solid #eee;transition:background .1s}.rel:hover{background:#f0f4f8;border-color:#cdd}
.rel .p{font-weight:bold;color:#c0392b}.rel .c{color:#2c8a4a}.rel .src{font-family:monospace;font-size:10px;color:#777;display:block;margin-top:2px}
.badge{font-size:9px;font-weight:bold;border-radius:8px;padding:0 6px;margin-left:5px;vertical-align:middle}
.badge.ev{background:#eaf3ea;color:#2c8a4a}.badge.ql{background:#fff3e0;color:#e67e22}
.swatch{display:inline-block;width:12px;height:12px;border-radius:3px;vertical-align:middle}
#ftr{height:30px;display:flex;align-items:center;gap:14px;padding:0 16px;background:#1f3a5f;color:#9fb3c8;font-size:11px;flex:none}
#ftr .sp{flex:1}#ftr a{color:#cfe0f0;text-decoration:none}#ftr a:hover{text-decoration:underline}
@media(max-width:760px){#hdr .tag,#hdr .stat,#bar .tip{display:none}}
@media(max-width:720px){#main{flex-direction:column}#left{flex:1;min-height:55vh}#side{width:auto;max-height:42vh;border-left:0;border-top:1px solid #e2e8ee}}
</style></head>
<body>
<div id='hdr'>
 <div class='brand'><span class='logo'></span><span class='wm'>BioBTree <b>KG</b></span>
  <span class='tag'>the schema of BioBTree's biolink knowledge graph</span></div>
 <nav class='links'><span class='stat'></span>
  <a href='https://github.com/tamerh/biobtree' target='_blank' rel='noopener'>BioBTree on GitHub</a></nav>
</div>
<div id='bar'>
 <span class='seg'><button id='bG' class='on' onclick="view('g')">Graph</button><button id='bM' onclick="view('m')">Matrix</button></span>
 <span id='gc' class='grp' style='gap:14px;flex-wrap:wrap'>
  <span class='grp'><label>Layout</label><span class='seg'>
   <button onclick="lay('dagre-LR')">Layered</button><button onclick="lay('concentric')">Hubs</button><button onclick="lay('cose')">Force</button></span></span>
  <span class='grp'><label>Edges</label><span class='seg'>
   <button id='eC' class='on' onclick="emode('click')">On click</button><button id='eA' onclick="emode('all')">Show all</button></span></span>
  <input id='find' list='catlist' placeholder='find node…'><datalist id='catlist'></datalist>
  <button class='tbtn' onclick="cy.fit(null,40)">Fit</button>
  <button class='tbtn' onclick="resetView()">Reset</button>
  <button id='lblBtn' class='tbtn' onclick="toggleLabels()">Labels</button>
  <button id='legBtn' class='tbtn' onclick="toggleLegend()">Legend</button>
 </span>
 <span class='tip'>click a node to reveal its connections &amp; datasets · press <b>/</b> to search</span>
</div>
<div id='legend'>
 <span><i class='dot'></i> node = biolink category</span>
 <span><i class='arr'>&rarr;</i> edge = typed relationship</span>
 <span><i class='ch ch-p'></i> primary source (own records)</span>
 <span><i class='ch ch-x'></i> cross-reference / identifier</span>
 <button class='lk' onclick="document.getElementById('about').classList.toggle('open')">About this graph</button>
</div>
<div id='about'><p>This is the <b>schema</b> (meta-graph) of the BioBTree knowledge graph — the
 <a href='https://biolink.github.io/biolink-model/' target='_blank' rel='noopener'>biolink</a> entity
 categories and the typed relationships between them, drawn from 150+ integrated biological datasets.
 Each node is a category; click it to see the contributing datasets (separated into <b>primary</b>
 sources with their own records and <b>cross-reference</b> namespaces that only supply identifiers)
 and the relationships it participates in. This is the schema view; a separate importable
 <b>subgraph</b> (human + disease + molecule) is published alongside it.</p></div>
<div id='main'>
 <div id='left'>
  <div id='cy'></div>
  <div id='catlegend'></div>
  <div id='onboard'>&#128072; Click any node to reveal its connections — or hover to preview. Press <b>/</b> to search.</div>
  <div id='loading'><div class='spin'></div><span>Laying out schema…</span></div>
  <div id='matrix'></div>
 </div>
 <div id='side'></div>
</div>
<div id='ftr'><span>BioBTree Knowledge Graph · schema view</span><span class='sp'></span>
 <a href='https://github.com/tamerh/biobtree' target='_blank' rel='noopener'>GitHub</a>
 <a href='https://biolink.github.io/biolink-model/' target='_blank' rel='noopener'>biolink-model</a><span>· CC BY 4.0</span></div>
<script>
var D=__PAYLOAD__;
var LS=null;try{LS=window.localStorage;}catch(e){}
document.querySelector('#hdr .stat').textContent=D.meta.cats+' categories · '+D.meta.edges+' relationships · '+D.meta.datasets+'+ datasets';
document.getElementById('catlist').innerHTML=D.cats.map(function(c){return "<option value='"+c+"'>";}).join('');
var cy=cytoscape({container:document.getElementById('cy'),elements:{nodes:D.nodes,edges:D.edges},
 style:[
  {selector:'node',style:{'background-color':'data(color)','label':'data(label)','font-size':'mapData(deg,0,18,10,15)','text-valign':'center','color':'#111','text-outline-color':'#fff','text-outline-width':2,'min-zoomed-font-size':6,'width':'mapData(deg,0,18,28,74)','height':'mapData(deg,0,18,28,74)'}},
  {selector:'edge',style:{'curve-style':'bezier','target-arrow-shape':'triangle','line-color':'#c4ccd4','target-arrow-color':'#c4ccd4','width':1.2}},
  {selector:'edge.lbl',style:{'label':'data(label)','font-size':8,'color':'#555','text-rotation':'autorotate','text-background-color':'#fff','text-background-opacity':0.85,'min-zoomed-font-size':7,'z-index':10}},
  {selector:'edge.hidden',style:{'display':'none'}},{selector:'.off',style:{'display':'none'}},
  {selector:'.faded',style:{'opacity':0.1}},
  {selector:'.peek',style:{'line-color':'#566','target-arrow-color':'#566','width':2,'z-index':9}},
  {selector:'node.peek',style:{'border-width':3,'border-color':'#445'}},
  {selector:'.hi',style:{'line-color':'#e15759','target-arrow-color':'#e15759','width':2.6,'color':'#900'}}
 ]});
cy.nodes().forEach(function(n){n.data('deg',n.connectedEdges().filter(function(e){return e.source().id()!=e.target().id();}).length);});
var EMODE='click',FOCUSED=false;
function lay(name){if(LS)try{LS.setItem('mg_layout',name);}catch(e){}
 var o={name:'cose'};
 if(name=='dagre-LR')o={name:'dagre',rankDir:'LR',nodeSep:40,rankSep:155};
 if(name=='concentric')o={name:'concentric',concentric:function(n){return n.degree()},levelWidth:function(){return 2},minNodeSpacing:48};
 cy.layout(o).run();cy.one('layoutstop',function(){cy.fit(null,40);});}
function emode(m){EMODE=m;document.getElementById('eC').className=m=='click'?'on':'';document.getElementById('eA').className=m=='all'?'on':'';
 if(m=='all'){cy.edges().removeClass('hidden');}else{cy.edges().addClass('hidden').removeClass('lbl');cy.elements().removeClass('faded hi');}}
function toggleLabels(){var on=cy.edges('.lbl:visible').length==0;cy.edges(':visible').toggleClass('lbl',on);document.getElementById('lblBtn').classList.toggle('on',on);}
function toggleLegend(){var el=document.getElementById('catlegend');var show=el.style.display!='block';el.style.display=show?'block':'none';document.getElementById('legBtn').classList.toggle('on',show);if(show&&!el.dataset.built)buildLegend();}
function buildLegend(){var el=document.getElementById('catlegend');
 el.innerHTML="<span class='ch' onclick=\"legendAll(true)\">all</span> / <span class='ch' onclick=\"legendAll(false)\">none</span>"
  +D.cats.map(function(c){return "<div class='lg' data-c='"+c+"'><span class='sw' style='background:"+D.colors[c]+"'></span>"+c+"</div>";}).join('');
 el.dataset.built=1;
 el.querySelectorAll('.lg').forEach(function(r){r.onclick=function(){var n=cy.getElementById(r.dataset.c);var off=!n.hasClass('off');n.toggleClass('off',off);n.connectedEdges().toggleClass('off',off);r.classList.toggle('dim',off);};});}
function legendAll(on){cy.elements().toggleClass('off',!on);document.querySelectorAll('#catlegend .lg').forEach(function(r){r.classList.toggle('dim',!on);});}
cy.on('mouseover','node',function(e){document.body.style.cursor='pointer';if(FOCUSED)return;var nb=e.target.closedNeighborhood();nb.addClass('peek');if(EMODE=='click')nb.edges().removeClass('hidden').addClass('lbl');});
cy.on('mouseout','node',function(e){document.body.style.cursor='';if(FOCUSED)return;cy.elements().removeClass('peek');if(EMODE=='click')cy.edges().addClass('hidden').removeClass('lbl');});
cy.on('tap','node',function(e){var n=e.target;var o=document.getElementById('onboard');if(o)o.style.display='none';
 if(EMODE=='click'){FOCUSED=true;cy.edges().addClass('hidden').removeClass('lbl');cy.elements().removeClass('faded hi peek');
  var ce=n.connectedEdges();ce.removeClass('hidden').addClass('hi lbl');
  cy.elements().addClass('faded');n.closedNeighborhood().removeClass('faded');}
 showNode(n.id());});
cy.on('tap',function(e){if(e.target===cy)resetView();});
function jumpTo(id){var n=cy.getElementById(id);if(!n.length)return;cy.animate({center:{eles:n},zoom:1.3},{duration:300});n.emit('tap');}
document.getElementById('find').addEventListener('change',function(e){jumpTo(e.target.value);});
function resetView(){FOCUSED=false;cy.elements().removeClass('faded hi peek');if(EMODE=='click')cy.edges().addClass('hidden').removeClass('lbl');resetPanel();}
document.addEventListener('keydown',function(e){if(e.target.tagName=='INPUT')return;
 if(e.key=='/'){e.preventDefault();document.getElementById('find').focus();}
 else if(e.key=='f')cy.fit(null,40);else if(e.key=='Escape')resetView();
 else if(e.key=='g')view('g');else if(e.key=='m')view('m');
 else if(e.key=='1')lay('dagre-LR');else if(e.key=='2')lay('concentric');else if(e.key=='3')lay('cose');});
function esc(s){return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');}
function badges(d){var b="";if(d.ev)b+="<span class='badge ev' title='edges carry ECO evidence'>evidence</span>";if(d.ql)b+="<span class='badge ql' title='edges carry qualifiers'>qualified</span>";return b;}
function chips(list){return list.map(function(x){
 var ex=x.example?"<span class='ex'>e.g. "+esc(x.example)+"</span>":"";
 var inner="<b>"+esc(x.ds)+"</b><i>"+esc(x.prefix)+"</i>"+ex;
 return x.url?"<a class='ds' href='"+esc(x.url)+"' target='_blank' rel='noopener'>"+inner+"</a>":"<span class='ds'>"+inner+"</span>";}).join('');}
function resetPanel(){FOCUSED=false;document.getElementById('side').innerHTML=
 "<h2>BioBTree KG schema</h2><div class='blurb'>Each node is a biolink category; each edge a predicate. Click a node to see its source datasets, identifier namespaces, and relationships.</div>"
 +"<h3>Legend</h3><div class='hint' style='color:#5b6b7a'>"
 +"<div style='margin:3px 0'><span class='swatch' style='background:#59a14f'></span> primary — own records, named nodes</div>"
 +"<div style='margin:3px 0'><span class='swatch' style='background:#bbb'></span> cross-reference — identifier-only stub nodes</div>"
 +"<div style='margin:3px 0'><span class='swatch' style='background:#e15759'></span> linking — gene&harr;gene edges, not nodes</div></div>"
 +"<div class='hint'>Tip: switch to <b>Matrix</b> for the full type&times;type grid.</div>";}
function showNode(cat){var col=D.colors[cat]||'#888';var nd=(D.nodeDatasets[cat]||[]);
 var out=[],inc=[];D.edges.forEach(function(e){var d=e.data;if(d.source==cat)out.push(d);if(d.target==cat)inc.push(d);});
 var prim=nd.filter(function(x){return x.primary;}),xref=nd.filter(function(x){return !x.primary;});
 var h="<h2><span class='swatch' style='background:"+col+"'></span>"+esc(cat)+"</h2>";
 if(D.blurbs[cat])h+="<div class='blurb' style='border-color:"+col+"'>"+esc(D.blurbs[cat])+"</div>";
 var prefs=Array.from(new Set(nd.map(function(x){return x.prefix;}).filter(Boolean)));
 if(prefs.length)h+="<div class='curie'>CURIE: "+esc(prefs.join(' · '))+"</div>";
 h+="<div class='counts'>"+prim.length+" primary · "+xref.length+" xref · "+out.length+" out · "+inc.length+" in</div>";
 h+="<h3>Primary sources ("+prim.length+")</h3>";
 h+=prim.length?"<div class='hint' style='margin:0 0 3px'>own records &rarr; named nodes with attributes &amp; synonyms</div>"+chips(prim):"<div class='hint'>none (runtime/stub only)</div>";
 if(xref.length){h+="<h3>Cross-reference ("+xref.length+")</h3><div class='hint' style='margin:0 0 3px'>identifier-only namespaces &rarr; typed but nameless stub nodes</div>"+chips(xref);}
 function relBlock(d,dir){var other=dir=='out'?d.target:d.source;var arrow=dir=='out'?'&rarr;':'&larr;';
  return "<div class='rel'><span class='p'>"+esc(d.label)+"</span> "+arrow+" <span class='c'>"+esc(other)+"</span>"+badges(d)
   +"<span class='src'>"+esc(d.datasets.join(', '))+"</span></div>";}
 h+="<h3>Outgoing ("+out.length+")</h3>"+(out.length?out.map(function(d){return relBlock(d,'out');}).join(''):"<div class='hint'>none</div>");
 h+="<h3>Incoming ("+inc.length+")</h3>"+(inc.length?inc.map(function(d){return relBlock(d,'in');}).join(''):"<div class='hint'>none</div>");
 document.getElementById('side').innerHTML=h;}
function showCell(s,o){var es=D.edges.filter(function(e){return e.data.source==s&&e.data.target==o;});
 var h="<h2><span class='swatch' style='background:"+(D.colors[s]||'#888')+"'></span>"+esc(s)+" &rarr; <span class='swatch' style='background:"+(D.colors[o]||'#888')+"'></span>"+esc(o)+"</h2>";
 h+="<h3>Relationships ("+es.length+")</h3>";
 h+=es.map(function(e){var d=e.data;return "<div class='rel'><span class='p'>"+esc(d.label)+"</span>"+badges(d)+"<span class='src'>"+esc(d.datasets.join(', '))+"</span></div>";}).join('');
 document.getElementById('side').innerHTML=h;}
function view(v){if(LS)try{LS.setItem('mg_view',v);}catch(e){}
 var g=v=='g';document.getElementById('cy').style.display=g?'block':'none';
 document.getElementById('matrix').style.display=g?'none':'block';
 document.getElementById('gc').style.display=g?'flex':'none';
 document.getElementById('bG').className=g?'on':'';document.getElementById('bM').className=g?'':'on';
 if(!g&&!document.getElementById('matrix').dataset.built)buildMatrix();}
function buildMatrix(){var m={};D.edges.forEach(function(e){var k=e.data.source+'>'+e.data.target;(m[k]=m[k]||[]).push(e.data.label);});
 var c=D.cats,h='<table><tr><th></th>';c.forEach(function(o){h+="<th class='rot'><div>"+o+"</div></th>";});h+='</tr>';
 c.forEach(function(s){h+="<td class='rh' style='color:"+D.colors[s]+"'>"+s+"</td>";
  c.forEach(function(o){var v=m[s+'>'+o];if(v){h+="<td class='cell' onclick=\"showCell('"+s+"','"+o+"')\" title='"+s+' &rarr; '+o+":\n"+v.join('\n')+"' style='background:"+D.colors[s]+"33'>"+(v.length>1?v.length:'&bull;')+"</td>";}else{h+="<td class='muted'></td>";}});h+='</tr>';});
 h+='</table>';document.getElementById('matrix').innerHTML=h;document.getElementById('matrix').dataset.built=1;}
cy.one('layoutstop',function(){var l=document.getElementById('loading');if(l)l.classList.add('gone');});
resetPanel();
lay((LS&&LS.getItem('mg_layout'))||'dagre-LR');emode('click');
if(LS&&LS.getItem('mg_view')=='m')view('m');
</script></body></html>"""
    with open(out_html, "w") as f:
        f.write(tmpl.replace("__PAYLOAD__", payload))


# --- poster / atlas view -----------------------------------------------------------
# Eight thematic bands laid out 4x2, copied from the preprint Fig.1 (introduction.tex):
# every biolink category lives in exactly one band, bands are colour-coded, and the
# whole graph is shown at once (no click-to-expand) -- the "neat, everything-visible"
# face. (key, label, color, grid_col, grid_row, [member categories])
_POSTER_CLUSTERS = [
    ("genes", "Genes & transcripts", "#e15759", 0, 0,
     ["Gene", "Transcript", "Exon", "CodingSequence", "NoncodingRNAProduct",
      "MicroRNA", "RegulatoryRegion", "NucleicAcidSequenceMotif"]),
    ("proteins", "Proteins & structure", "#4f9d4f", 1, 0,
     ["Protein", "ProteinDomain", "ProteinFamily"]),
    ("expression", "Expression & anatomy", "#4ca39c", 2, 0,
     ["Cell", "CellLine", "GrossAnatomicalStructure"]),
    ("pathways", "Pathways & function", "#b07aa1", 3, 0,
     ["Pathway", "MolecularActivity", "BiologicalProcess", "CellularComponent"]),
    ("variants", "Variants & clinical", "#3aa6b5", 0, 1,
     ["SequenceVariant"]),
    ("diseases", "Diseases & phenotypes", "#4e79a7", 1, 1,
     ["Disease", "DiseaseOrPhenotypicFeature", "PhenotypicFeature"]),
    ("drugs", "Drugs & chemistry", "#d6a219", 2, 1,
     ["SmallMolecule", "Drug", "ChemicalEntity"]),
    ("other", "Cross-cutting", "#8a6bbf", 3, 1,
     ["OrganismTaxon", "Publication"]),
]
_SLOT_W, _SLOT_H, _NODE_DX, _NODE_DY = 600, 470, 165, 108


def _txt_color(hex_color):
    """Readable text colour (black/white) for a given background hex."""
    h = hex_color.lstrip("#")
    r, g, b = (int(h[i:i + 2], 16) for i in (0, 2, 4))
    return "#1b232c" if (0.299 * r + 0.587 * g + 0.114 * b) > 150 else "#ffffff"


def _poster_positions():
    """Seed (x, y) per category: a centred grid inside each band's 4x2 slot.
    These are *seed* coordinates -- the `?edit` mode lets us drag + re-export them."""
    import math
    pos, cluster_of = {}, {}
    for key, _lbl, _col, gc, gr, members in _POSTER_CLUSTERS:
        n = len(members)
        bcols = 1 if n == 1 else (2 if n <= 4 else 3)
        brows = math.ceil(n / bcols)
        gw, gh = (bcols - 1) * _NODE_DX, (brows - 1) * _NODE_DY
        cx, cy = gc * _SLOT_W, gr * _SLOT_H
        for i, m in enumerate(members):
            r, c = divmod(i, bcols)
            pos[m] = {"x": round(cx + c * _NODE_DX - gw / 2, 1),
                      "y": round(cy + r * _NODE_DY - gh / 2, 1)}
            cluster_of[m] = key
    return pos, cluster_of


def render_poster(triples, out_html, catmap=None, primary_names=None, registry=None):
    """Static, everything-visible schema poster (preset band layout, faint edges with
    hover-highlight, pan/zoom, no click panel). Inspired by the preprint TikZ figure.
    Append `?edit` to the URL to drag nodes and copy a fresh position map."""
    import json
    primary_names = set(primary_names or ())
    primary_names |= {"go", "refseq", "dbsnp", "mesh"}

    def nid(c):
        return c.split(":")[1]
    cats = sorted({c for (s, _, o) in triples for c in (s, o)})
    pos, cluster_of = _poster_positions()
    clusters = [{"id": k, "label": lbl, "color": col} for k, lbl, col, _gc, _gr, _m in _POSTER_CLUSTERS]
    band_color = {k: col for k, _l, col, _gc, _gr, _m in _POSTER_CLUSTERS}
    # any category not placed in a band (e.g. a new one) -> drop into "other" + warn
    for c in cats:
        if nid(c) not in cluster_of:
            cluster_of[nid(c)] = "other"
            pos.setdefault(nid(c), {"x": 3 * _SLOT_W, "y": 1 * _SLOT_H + 200})
            print(f"  [poster] WARNING: {nid(c)} has no band -> 'other'")

    deg = defaultdict(int)
    edges = []
    for (s, p, o), ds in sorted(triples.items()):
        si, oi, pl = nid(s), nid(o), p.split(":")[1]
        deg[si] += 1
        deg[oi] += 1
        edges.append({"data": {"id": f"{si}|{pl}|{oi}", "source": si, "target": oi,
                               "label": pl, "n": len(ds),
                               "cross": cluster_of.get(si) != cluster_of.get(oi),
                               "ev": p in _EVIDENCE_PREDS, "ql": p in _QUALIFIER_PREDS}})
    # dataset counts per category (shown faintly on hover)
    nds = defaultdict(int)
    if catmap is not None:
        link_only = {"ortholog", "paralog", "orthologentrez", "relatedentrez", "neighborentrez"}
        for ds in catmap.datasets():
            if ds in link_only:
                continue
            e = catmap.entry_for(ds)
            if e:
                nds[nid(e.category)] += 1

    nodes = []
    for c in cats:
        i = nid(c)
        k = cluster_of[i]
        col = band_color[k]
        nodes.append({"data": {"id": i, "label": i, "parent": "c_" + k, "color": col,
                               "txt": _txt_color(col), "deg": deg[i],
                               "blurb": _BLURBS.get(i, ""), "nds": nds.get(i, 0)},
                      "position": pos[i]})
    parents = [{"data": {"id": "c_" + c["id"], "label": c["label"], "color": c["color"],
                         "band": True}} for c in clusters]

    n_datasets = sum(nds.values())
    payload = json.dumps({"parents": parents, "nodes": nodes, "edges": edges,
                          "clusters": clusters,
                          "meta": {"cats": len(cats), "edges": len(edges), "datasets": n_datasets}})
    tmpl = _POSTER_TMPL
    with open(out_html, "w") as f:
        f.write(tmpl.replace("__PAYLOAD__", payload))


_POSTER_TMPL = r"""<!doctype html><html lang='en'><head><meta charset='utf-8'>
<meta name='viewport' content='width=device-width,initial-scale=1'>
<title>BioBTree Knowledge Graph — schema</title>
<meta name='description' content='The BioBTree biolink knowledge-graph schema at a glance: every node category and typed relationship, grouped into thematic bands.'>
<link rel='icon' href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='7' fill='%234e79a7'/%3E%3Ccircle cx='10' cy='11' r='3.2' fill='%23fff'/%3E%3Ccircle cx='22' cy='10' r='3.2' fill='%23f28e2b'/%3E%3Ccircle cx='16' cy='23' r='3.2' fill='%2359a14f'/%3E%3Cpath d='M10 11 L22 10 M22 10 L16 23 M16 23 L10 11' stroke='%23fff' stroke-width='1.4' opacity='.7'/%3E%3C/svg%3E">
<meta name='theme-color' content='#1f3a5f'>
<script src='https://unpkg.com/cytoscape@3.28.1/dist/cytoscape.min.js'></script>
<style>
html,body{margin:0;height:100%;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#2c3744;-webkit-font-smoothing:antialiased}
body{display:flex;flex-direction:column;height:100vh;overflow:hidden}
#hdr{display:flex;align-items:baseline;gap:14px;padding:12px 20px 11px;background:#1f3a5f;color:#eaf0f6;flex:none}
#hdr h1{font-size:16px;font-weight:600;margin:0;letter-spacing:.2px}
#hdr .sub{font-size:12px;color:#aebfd2}
#hdr .sp{flex:1}
#hdr a{color:#cfe0f2;text-decoration:none;font-size:12px;border:1px solid #3c5d86;padding:3px 9px;border-radius:5px}
#hdr a:hover{background:#28456b}
#stage{position:relative;flex:1;min-height:0;background:#fbfcfe;
 background-image:radial-gradient(#e7ecf3 1px,transparent 1px);background-size:26px 26px}
#cy{position:absolute;inset:0}
#legend{position:absolute;left:14px;bottom:14px;background:rgba(255,255,255,.94);border:1px solid #dde4ee;
 border-radius:9px;padding:9px 11px;box-shadow:0 3px 14px rgba(31,58,95,.10);font-size:11.5px;max-width:240px}
#legend .lt{font-weight:600;color:#42536a;margin:0 0 6px;font-size:10.5px;letter-spacing:.5px;text-transform:uppercase}
#legend .row{display:flex;align-items:center;gap:7px;padding:2px 0;cursor:default}
#legend .sw{width:12px;height:12px;border-radius:3px;flex:none}
#hint{position:absolute;right:14px;bottom:14px;background:rgba(255,255,255,.92);border:1px solid #dde4ee;
 border-radius:8px;padding:7px 11px;font-size:11.5px;color:#56657a;box-shadow:0 3px 14px rgba(31,58,95,.10)}
#hint b{color:#33455c}
#tip{position:absolute;display:none;pointer-events:none;background:#1f2a37;color:#eef3f9;font-size:12px;
 padding:7px 10px;border-radius:7px;max-width:280px;box-shadow:0 4px 16px rgba(0,0,0,.28);line-height:1.4;z-index:9}
#tip .tn{font-weight:600}#tip .tb{color:#b9c6d6;margin-top:2px}#tip .tm{color:#8fa6c0;margin-top:4px;font-size:11px}
#toolbar{position:absolute;right:14px;top:14px;display:flex;gap:6px}
#toolbar button{background:#fff;border:1px solid #d3dbe6;border-radius:7px;padding:6px 11px;font-size:12px;
 color:#3a4a5e;cursor:pointer;box-shadow:0 2px 8px rgba(31,58,95,.07)}
#toolbar button:hover{background:#eef3fa;border-color:#b9c7d8}
#edit{position:absolute;left:50%;top:14px;transform:translateX(-50%);background:#e15759;color:#fff;
 border:none;border-radius:7px;padding:7px 14px;font-size:12.5px;cursor:pointer;display:none;box-shadow:0 3px 12px rgba(225,87,89,.35)}
#loading{position:absolute;inset:0;display:flex;align-items:center;justify-content:center;color:#7e8da0;font-size:13px;background:#fbfcfe;z-index:5;transition:opacity .4s}
#loading.gone{opacity:0;pointer-events:none}
</style></head><body>
<div id='hdr'>
 <h1>BioBTree Knowledge Graph</h1>
 <span class='sub' id='counts'></span>
 <span class='sp'></span>
 <a href='https://github.com/tamerh/biobtree' target='_blank' rel='noopener'>GitHub</a>
</div>
<div id='stage'>
 <div id='cy'></div>
 <div id='loading'>building schema…</div>
 <div id='toolbar'><button onclick='cy.fit(cy.elements(),50)'>Fit</button><button onclick='cy.zoom(cy.zoom()*1.2)'>+</button><button onclick='cy.zoom(cy.zoom()/1.2)'>−</button></div>
 <button id='edit' onclick='copyPos()'>📋 Copy positions</button>
 <div id='legend'><div class='lt'>Domains</div></div>
 <div id='hint'><b>Hover</b> a node to trace its links · <b>drag</b> to pan · <b>scroll</b> to zoom</div>
 <div id='tip'></div>
</div>
<script>
var D=__PAYLOAD__;
var EDIT=/[?&]edit\b/.test(location.search);
document.getElementById('counts').textContent=D.meta.cats+' node categories · '+D.meta.edges+' relationship types · '+D.meta.datasets+' datasets';
var els=[].concat(D.parents,D.nodes,D.edges);
var cy=cytoscape({
 container:document.getElementById('cy'),
 elements:els,
 layout:{name:'preset'},
 wheelSensitivity:0.22,
 minZoom:0.15,maxZoom:3,
 style:[
  {selector:'node[band]',style:{'shape':'round-rectangle','background-color':'data(color)','background-opacity':0.07,
    'border-width':1.4,'border-color':'data(color)','border-opacity':0.55,'label':'data(label)',
    'text-valign':'top','text-halign':'center','text-margin-y':-7,'font-size':14,'font-weight':600,
    'color':'data(color)','padding':'26px','z-index':0,'events':'no'}},
  {selector:'node[!band]',style:{'shape':'round-rectangle','background-color':'data(color)','label':'data(label)',
    'color':'data(txt)','text-valign':'center','text-halign':'center','font-size':12,'font-weight':500,
    'text-wrap':'wrap','text-max-width':'118px','width':'label','height':'label','padding':'9px',
    'border-width':1.2,'border-color':'#ffffff','border-opacity':0.65,'z-index':10,
    'transition-property':'opacity,border-color,border-width','transition-duration':'120ms'}},
  {selector:'edge',style:{'curve-style':'bezier','width':1.3,'line-color':'#8aa0bb','opacity':0.16,
    'target-arrow-shape':'triangle','target-arrow-color':'#8aa0bb','arrow-scale':0.6,
    'font-size':10,'color':'#5a6b80','text-background-color':'#fff','text-background-opacity':0.9,
    'text-background-padding':2,'z-index':1,
    'transition-property':'opacity,width,line-color','transition-duration':'120ms'}},
  {selector:'edge[?cross]',style:{'line-color':'#6f86a4','opacity':0.22}},
  {selector:'edge[source = target]',style:{'curve-style':'bezier','loop-direction':'-45deg','loop-sweep':'-40deg'}},
  {selector:'.faded',style:{'opacity':0.06}},
  {selector:'node.faded',style:{'opacity':0.10}},
  {selector:'edge.hot',style:{'opacity':0.95,'width':2.4,'line-color':'data(hotc)','target-arrow-color':'data(hotc)','label':'data(label)','z-index':20}},
  {selector:'node.hot',style:{'opacity':1,'border-color':'#1f2a37','border-width':2.2,'z-index':30}}
 ]
});
cy.nodes('[!band]').grabbable(EDIT);
cy.nodes('[band]').ungrabify();
cy.autoungrabify(!EDIT);
cy.fit(cy.elements(),50);

// build legend from clusters
var lg=document.getElementById('legend');
D.clusters.forEach(function(c){
 var r=document.createElement('div');r.className='row';
 r.innerHTML="<span class='sw' style='background:"+c.color+"'></span>"+c.label;
 r.onmouseenter=function(){highlightCluster(c.id);};
 r.onmouseleave=clearHi;
 lg.appendChild(r);
});

// hover-highlight: dim everything except the node, its edges and neighbours
var tip=document.getElementById('tip');
function clearHi(){cy.elements().removeClass('faded hot');tip.style.display='none';}
cy.on('mouseover','node[!band]',function(e){
 var n=e.target,hood=n.closedNeighborhood();
 cy.elements().addClass('faded');
 hood.removeClass('faded');
 n.addClass('hot');hood.nodes().difference(n).removeClass('faded');
 var col=n.data('color');
 n.connectedEdges().forEach(function(ed){ed.data('hotc',col);}).removeClass('faded').addClass('hot');
 var d=n.data();
 tip.innerHTML="<div class='tn'>"+d.id+"</div>"+(d.blurb?"<div class='tb'>"+d.blurb+"</div>":"")
   +"<div class='tm'>"+d.deg+" relationships"+(d.nds?" · "+d.nds+" datasets":"")+"</div>";
 tip.style.display='block';
});
cy.on('mousemove','node[!band]',function(e){
 var p=e.renderedPosition,st=document.getElementById('stage').getBoundingClientRect();
 tip.style.left=Math.min(p.x+16,st.width-296)+'px';tip.style.top=(p.y+16)+'px';
});
cy.on('mouseout','node[!band]',clearHi);

function highlightCluster(cid){
 cy.elements().addClass('faded');
 var ns=cy.nodes("[!band][parent = 'c_"+cid+"']");
 ns.removeClass('faded');
 ns.connectedEdges().forEach(function(ed){ed.data('hotc',ed.source().data('color'));}).removeClass('faded').addClass('hot');
 ns.connectedEdges().connectedNodes().removeClass('faded');
}

// edit mode: drag nodes, then copy a {id:{x,y}} map for baking back into the source
if(EDIT){
 document.getElementById('edit').style.display='block';
 document.getElementById('hint').innerHTML="<b>EDIT MODE</b> · drag nodes, then Copy positions";
}
function copyPos(){
 var m={};cy.nodes('[!band]').forEach(function(n){var p=n.position();m[n.id()]={x:Math.round(p.x*10)/10,y:Math.round(p.y*10)/10};});
 var s=JSON.stringify(m,null,1);
 navigator.clipboard.writeText(s).then(function(){var b=document.getElementById('edit');b.textContent='✓ copied';setTimeout(function(){b.textContent='📋 Copy positions';},1400);},function(){prompt('positions:',s);});
}
setTimeout(function(){var l=document.getElementById('loading');if(l)l.classList.add('gone');},250);
</script></body></html>"""


# --- ER / schema-flow view ---------------------------------------------------------
# A left-to-right layered diagram (Graphviz dot) in the style of the preprint methods
# figure: every biolink category is a rich "entity" box (header + blurb + example CURIE
# + datasets + ontology badge), relationships are single labelled lines (parallel
# predicates merged), and dot's crossing-minimisation keeps paths followable. Self-loop
# predicates (subclass_of / close_match / same_as) become an in-box badge, not an edge.
_ER_SELFLOOP_PREDS = {"subclass_of", "close_match", "same_as", "related_to"}
# predicates demoted from drawn edges to an in-box badge: low-information "universal"
# annotations whose target adds a hub of long crossing lines (every node is in_taxon
# OrganismTaxon). Shown as a badge on the source box instead; the lone target node
# (OrganismTaxon) then has no edges and is dropped from the diagram.
_ER_BADGE_PREDS = {"in_taxon": "in OrganismTaxon"}
# explicit biological left-to-right columns (central dogma + annotations); forces a
# readable rank order so most relationships flow forward instead of crossing back.
_ER_COLUMNS = [
    ["SequenceVariant", "RegulatoryRegion", "NucleicAcidSequenceMotif", "MicroRNA"],
    ["Gene"],
    ["Transcript", "NoncodingRNAProduct", "Exon", "CodingSequence"],
    ["Protein"],
    ["ProteinDomain", "ProteinFamily", "Pathway", "MolecularActivity",
     "BiologicalProcess", "CellularComponent", "Cell", "CellLine", "GrossAnatomicalStructure"],
    ["Disease", "PhenotypicFeature", "DiseaseOrPhenotypicFeature"],
    ["SmallMolecule", "Drug", "ChemicalEntity", "Publication"],
]


def _html_esc(s):
    return str(s).replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def _er_collect(triples, catmap, registry):
    """Per-category box content + merged relationship lines.
    Returns (ds_of, example, rels, loops, badges)."""
    def nid(c):
        return c.split(":")[1]
    # datasets + a representative example CURIE per category
    link_only = {"ortholog", "paralog", "orthologentrez", "relatedentrez", "neighborentrez"}
    ds_of = defaultdict(list)
    example = {}
    if catmap is not None:
        for ds in sorted(catmap.datasets()):
            if ds in link_only:
                continue
            e = catmap.entry_for(ds)
            if not e:
                continue
            cat = nid(e.category)
            ds_of[cat].append(ds)
            if cat not in example:
                ex = _EXAMPLE_LOCAL.get(ds)
                if ex:
                    example[cat] = ex if ":" in ex else to_curie(e.prefix, ex)
    for ac in ("MolecularActivity", "BiologicalProcess", "CellularComponent"):
        ds_of[ac].append("go")
        example.setdefault(ac, "GO:0006915")
    for rs in ("Transcript",):
        if "refseq" not in ds_of[rs]:
            ds_of[rs].append("refseq")

    rels = defaultdict(set)          # (src,tgt) -> {predicate,...}
    loops = defaultdict(set)         # cat -> {selfloop predicate,...}
    badges = defaultdict(set)        # cat -> {demoted universal annotation,...}
    for (s, p, o), _ds in triples.items():
        si, oi, pl = nid(s), nid(o), p.split(":")[1]
        if pl in _ER_BADGE_PREDS:
            badges[si].add(_ER_BADGE_PREDS[pl])
        elif si == oi:
            loops[si].add(pl)
        else:
            rels[(si, oi)].add(pl)
    return ds_of, example, rels, loops, badges


def render_er(triples, out_html, catmap=None, primary_names=None, registry=None, png=None):
    """ER/flow schema diagram via Graphviz dot (rankdir=LR). The published, everything-
    visible, follow-the-paths face. Writes an HTML page (inline SVG + pan/zoom) and,
    if `png` given, a PNG preview."""
    import os
    import sys
    import graphviz
    # the `dot` binary ships next to this interpreter (conda env); ensure it's findable
    # even when the env isn't "activated" (e.g. invoked as /path/to/env/bin/python -m ...)
    os.environ["PATH"] = os.path.dirname(sys.executable) + os.pathsep + os.environ.get("PATH", "")

    def nid(c):
        return c.split(":")[1]
    cats = sorted({c for (s, _, o) in triples for c in (s, o)})
    _pos, cluster_of = _poster_positions()
    band_color = {k: col for k, _l, col, _gc, _gr, _m in _POSTER_CLUSTERS}
    for c in cats:
        cluster_of.setdefault(nid(c), "other")
    ds_of, example, rels, loops, badges = _er_collect(triples, catmap, registry)
    # drop nodes that, after demoting universal annotations, have no relationships at
    # all (e.g. OrganismTaxon becomes a pure in_taxon target -> a badge, not a node).
    linked = {x for pair in rels for x in pair}
    drawn = [c for c in cats if nid(c) in linked]  # skip nodes left with only a self-loop

    def box_label(cat):
        col = band_color[cluster_of[cat]]
        txt = _txt_color(col)
        blurb = _BLURBS.get(cat, "")
        dss = ds_of.get(cat, [])
        ds_line = ", ".join(dss[:3]) + (f" +{len(dss) - 3}" if len(dss) > 3 else "")
        rows = [f'<TR><TD ALIGN="CENTER" BGCOLOR="{col}" CELLPADDING="5">'
                f'<FONT COLOR="{txt}" POINT-SIZE="12.5"><B>{_html_esc(cat)}</B></FONT></TD></TR>']
        if blurb:
            rows.append(f'<TR><TD ALIGN="LEFT"><FONT POINT-SIZE="8.5" COLOR="#5a6b80">'
                        f'<I>{_html_esc(blurb)}</I></FONT></TD></TR>')
        if cat in example:
            rows.append(f'<TR><TD ALIGN="LEFT"><FONT POINT-SIZE="8.5" COLOR="#33455c">'
                        f'e.g. {_html_esc(example[cat])}</FONT></TD></TR>')
        if ds_line:
            rows.append(f'<TR><TD ALIGN="LEFT"><FONT POINT-SIZE="8" COLOR="#7a8aa0">'
                        f'{len(dss)} datasets &middot; {_html_esc(ds_line)}</FONT></TD></TR>')
        meta_bits = []
        if loops.get(cat):
            meta_bits.append("&#8635; " + _html_esc(", ".join(sorted(loops[cat]))))
        if badges.get(cat):
            meta_bits.append("&#127760; " + _html_esc(", ".join(sorted(badges[cat]))))
        for b in meta_bits:
            rows.append(f'<TR><TD ALIGN="LEFT"><FONT POINT-SIZE="8" COLOR="#9270b8">'
                        f'{b}</FONT></TD></TR>')
        return ("<<TABLE BORDER=\"1\" COLOR=\"#c9d3e0\" CELLBORDER=\"0\" CELLSPACING=\"0\" "
                "CELLPADDING=\"3\" BGCOLOR=\"white\">" + "".join(rows) + "</TABLE>>")

    drawn_ids = {nid(c) for c in drawn}
    g = graphviz.Digraph("er", format="svg")
    g.attr(rankdir="LR", splines="ortho", nodesep="0.5", ranksep="1.6",
           bgcolor="white", fontname="Helvetica", newrank="true", forcelabels="true")
    g.attr("node", shape="plaintext", margin="0", fontname="Helvetica")
    g.attr("edge", fontname="Helvetica", fontsize="8.5", color="#9aabc0",
           arrowsize="0.6", penwidth="1.0")
    for c in drawn:
        g.node(nid(c), label=box_label(nid(c)))
    # force the biological column order: rank=same per column + an invisible weighted
    # chain between the first node of consecutive (present) columns.
    present_cols = []
    for col in _ER_COLUMNS:
        pres = [c for c in col if c in drawn_ids]
        if pres:
            present_cols.append(pres)
            with g.subgraph() as s:
                s.attr(rank="same")
                for c in pres:
                    s.node(c)
    for a, b in zip(present_cols, present_cols[1:]):
        g.edge(a[0], b[0], style="invis", weight="20")
    for (si, oi), preds in sorted(rels.items()):
        col = band_color[cluster_of[si]]
        lbl = "\\n".join(sorted(preds))
        g.edge(si, oi, xlabel=lbl, color=col + "cc", fontcolor="#54657a")

    svg = g.pipe(format="svg").decode("utf-8")
    if png:
        try:
            with open(png, "wb") as f:
                f.write(g.pipe(format="png"))
        except Exception as e:  # pragma: no cover
            print("  [er] png preview failed:", e)
    # strip the XML prolog so the SVG drops straight into the page
    svg = svg[svg.find("<svg"):]
    meta = {"cats": len(drawn), "rels": len(rels),
            "datasets": len({d for v in ds_of.values() for d in v})}
    html = _ER_TMPL.replace("__SVG__", svg).replace(
        "__META__", f"{meta['cats']} categories &middot; {meta['rels']} relationships "
                    f"&middot; {meta['datasets']} datasets")
    with open(out_html, "w") as f:
        f.write(html)


_ER_TMPL = r"""<!doctype html><html lang='en'><head><meta charset='utf-8'>
<meta name='viewport' content='width=device-width,initial-scale=1'>
<title>BioBTree Knowledge Graph — schema</title>
<meta name='description' content='The BioBTree biolink knowledge-graph schema as an entity-relationship flow: every node category, its datasets and identifiers, and the typed relationships between them.'>
<link rel='icon' href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='7' fill='%234e79a7'/%3E%3Ccircle cx='10' cy='11' r='3.2' fill='%23fff'/%3E%3Ccircle cx='22' cy='10' r='3.2' fill='%23f28e2b'/%3E%3Ccircle cx='16' cy='23' r='3.2' fill='%2359a14f'/%3E%3C/svg%3E">
<meta name='theme-color' content='#1f3a5f'>
<script src='https://unpkg.com/svg-pan-zoom@3.6.1/dist/svg-pan-zoom.min.js'></script>
<style>
html,body{margin:0;height:100%;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#2c3744}
body{display:flex;flex-direction:column;height:100vh;overflow:hidden}
#hdr{display:flex;align-items:baseline;gap:14px;padding:12px 20px 11px;background:#1f3a5f;color:#eaf0f6;flex:none}
#hdr h1{font-size:16px;font-weight:600;margin:0}
#hdr .sub{font-size:12px;color:#aebfd2}#hdr .sp{flex:1}
#hdr a{color:#cfe0f2;text-decoration:none;font-size:12px;border:1px solid #3c5d86;padding:3px 9px;border-radius:5px}
#hdr a:hover{background:#28456b}
#stage{position:relative;flex:1;min-height:0;background:#fbfcfe;
 background-image:radial-gradient(#e7ecf3 1px,transparent 1px);background-size:26px 26px}
#wrap{position:absolute;inset:0}#wrap svg{width:100%;height:100%}
#toolbar{position:absolute;right:14px;top:14px;display:flex;gap:6px}
#toolbar button{background:#fff;border:1px solid #d3dbe6;border-radius:7px;padding:6px 11px;font-size:12px;color:#3a4a5e;cursor:pointer;box-shadow:0 2px 8px rgba(31,58,95,.07)}
#toolbar button:hover{background:#eef3fa}
#legend{position:absolute;left:14px;bottom:14px;background:rgba(255,255,255,.94);border:1px solid #dde4ee;border-radius:9px;padding:9px 11px;box-shadow:0 3px 14px rgba(31,58,95,.10);font-size:11.5px}
#legend .lt{font-weight:600;color:#42536a;margin:0 0 6px;font-size:10.5px;letter-spacing:.5px;text-transform:uppercase}
#legend .row{display:flex;align-items:center;gap:7px;padding:2px 0}
#legend .sw{width:12px;height:12px;border-radius:3px;flex:none}
#hint{position:absolute;right:14px;bottom:14px;background:rgba(255,255,255,.92);border:1px solid #dde4ee;border-radius:8px;padding:7px 11px;font-size:11.5px;color:#56657a;box-shadow:0 3px 14px rgba(31,58,95,.10)}
</style></head><body>
<div id='hdr'><h1>BioBTree Knowledge Graph</h1><span class='sub'>__META__</span>
 <span class='sp'></span>
 <a href='https://github.com/tamerh/biobtree' target='_blank' rel='noopener'>GitHub</a></div>
<div id='stage'>
 <div id='wrap'>__SVG__</div>
 <div id='toolbar'><button onclick='pz.fit();pz.center()'>Fit</button><button onclick='pz.zoomIn()'>+</button><button onclick='pz.zoomOut()'>−</button></div>
 <div id='legend'><div class='lt'>Domains</div></div>
</div>
<script>
var LEG=[["#e15759","Genes & transcripts"],["#4f9d4f","Proteins & structure"],["#4ca39c","Expression & anatomy"],["#b07aa1","Pathways & function"],["#3aa6b5","Variants & clinical"],["#4e79a7","Diseases & phenotypes"],["#d6a219","Drugs & chemistry"],["#8a6bbf","Cross-cutting"]];
document.querySelector('#legend').insertAdjacentHTML('beforeend',LEG.map(function(l){return "<div class='row'><span class='sw' style='background:"+l[0]+"'></span>"+l[1]+"</div>";}).join(''));
var svg=document.querySelector('#wrap svg');svg.removeAttribute('width');svg.removeAttribute('height');
var pz=svgPanZoom(svg,{controlIconsEnabled:false,fit:true,center:true,minZoom:0.2,maxZoom:8,zoomScaleSensitivity:0.3});
window.addEventListener('resize',function(){pz.resize();pz.fit();pz.center();});
</script></body></html>"""


# --- neat domain-grouped views (edge bundling + arc), shared payload -----------------
def _neat_payload(triples, catmap, registry):
    """JSON payload for the bundling/arc views: domain-grouped nodes (with the rich box
    info) + merged relationship lines. Reuses the ER collector so the two stay in sync."""
    import json

    def nid(c):
        return c.split(":")[1]
    cats = sorted({nid(c) for (s, _, o) in triples for c in (s, o)})
    _pos, cluster_of = _poster_positions()
    for c in cats:
        cluster_of.setdefault(c, "other")
    ds_of, example, rels, loops, badges = _er_collect(triples, catmap, registry)
    linked = {x for pair in rels for x in pair}

    deg = defaultdict(int)
    for (s, o) in rels:
        deg[s] += 1
        deg[o] += 1
    dom_color = {k: col for k, _l, col, _gc, _gr, _m in _POSTER_CLUSTERS}
    dom_order = {k: i for i, (k, *_r) in enumerate(_POSTER_CLUSTERS)}
    # node order: by domain (visual order), then hubs first within a domain
    nodes_in = [c for c in cats if c in linked]
    # only domains that actually contain a drawn node (every node must map to one, or
    # the radial hierarchy is missing a leaf -> the bundle view crashes)
    present = {cluster_of[c] for c in nodes_in}
    domains = [{"key": k, "label": lbl, "color": col}
               for k, lbl, col, _gc, _gr, _m in _POSTER_CLUSTERS if k in present]
    nodes_in.sort(key=lambda c: (dom_order.get(cluster_of[c], 99), -deg[c], c))
    nodes = [{"id": c, "domain": cluster_of[c], "color": dom_color[cluster_of[c]],
              "blurb": _BLURBS.get(c, ""), "example": example.get(c, ""),
              "datasets": ds_of.get(c, []), "nds": len(ds_of.get(c, [])),
              "loops": sorted(loops.get(c, [])), "badges": sorted(badges.get(c, [])),
              "deg": deg[c]} for c in nodes_in]
    edges = [{"source": s, "target": o, "preds": sorted(p)} for (s, o), p in sorted(rels.items())]
    return json.dumps({"nodes": nodes, "edges": edges, "domains": domains,
                       "meta": {"cats": len(nodes), "rels": len(edges),
                                "datasets": len({d for v in ds_of.values() for d in v})}})


def render_bundle(triples, out_html, catmap=None, primary_names=None, registry=None):
    """Radial hierarchical edge-bundling view (D3): nodes on a circle grouped by domain,
    relationships bundled toward the centre. Hover a node to trace its links."""
    with open(out_html, "w") as f:
        f.write(_BUNDLE_TMPL.replace("__PAYLOAD__", _neat_payload(triples, catmap, registry)))


def render_arc(triples, out_html, catmap=None, primary_names=None, registry=None):
    """Arc-diagram view (D3): nodes in one row grouped into domain blocks, relationships
    as semicircle arcs. Hover a node to trace its links."""
    with open(out_html, "w") as f:
        f.write(_ARC_TMPL.replace("__PAYLOAD__", _neat_payload(triples, catmap, registry)))


_NEAT_HEAD = r"""<!doctype html><html lang='en'><head><meta charset='utf-8'>
<meta name='viewport' content='width=device-width,initial-scale=1'>
<title>BioBTree Knowledge Graph — schema</title>
<meta name='description' content='The BioBTree biolink knowledge-graph schema, grouped by domain: every node category and the typed relationships between them.'>
<link rel='icon' href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='7' fill='%234e79a7'/%3E%3Ccircle cx='10' cy='11' r='3.2' fill='%23fff'/%3E%3Ccircle cx='22' cy='10' r='3.2' fill='%23f28e2b'/%3E%3Ccircle cx='16' cy='23' r='3.2' fill='%2359a14f'/%3E%3C/svg%3E">
<meta name='theme-color' content='#1f3a5f'>
<script src='https://unpkg.com/d3@7.8.5/dist/d3.min.js'></script>
<style>
html,body{margin:0;height:100%;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#2c3744}
body{display:flex;flex-direction:column;height:100vh;overflow:hidden}
#hdr{display:flex;align-items:baseline;gap:14px;padding:12px 20px 11px;background:#1f3a5f;color:#eaf0f6;flex:none}
#hdr .desc{font-size:13px;color:#cfe0f2;font-weight:400;line-height:1.4}#hdr .sp{flex:1}
#hdr a{color:#cfe0f2;text-decoration:none;font-size:12px;border:1px solid #3c5d86;padding:3px 9px;border-radius:5px;white-space:nowrap}
#hdr a:hover{background:#28456b}
#stage{position:relative;flex:1;min-height:0;background:#fbfcfe;background-image:radial-gradient(#e7ecf3 1px,transparent 1px);background-size:26px 26px}
#viz{width:100%;height:100%}
.link{fill:none;stroke-opacity:.16}
.node-dot{cursor:pointer}
.node-lbl{font-size:11px;cursor:pointer;fill:#3a4658}
.faded{opacity:.07}
.lit{stroke-opacity:.95!important}
text.hot{fill:#111;font-weight:600}
.elbl{font-size:9.5px;fill:#33455c;text-anchor:middle;pointer-events:none;paint-order:stroke;stroke:#fbfcfe;stroke-width:3px;stroke-linejoin:round}
/* dark mode (?dark): legible on a dark hero background */
body.dark .node-lbl{fill:#cbd5e1}
body.dark text.hot{fill:#fff}
body.dark .link{stroke-opacity:.3}
body.dark .elbl{fill:#e8eef7;stroke:#0f172a}
body.dark #legend{color:#cbd5e1}
#panel{position:absolute;right:0;top:0;width:300px;max-height:100%;overflow:auto;background:rgba(255,255,255,.97);border-left:1px solid #e2e8f1;box-shadow:-3px 0 16px rgba(31,58,95,.06);padding:16px 18px;box-sizing:border-box;transform:translateX(100%);transition:transform .18s}
#panel.show{transform:none}
#panel h2{margin:0 0 4px;font-size:15px;display:flex;align-items:center;gap:8px}
#panel .sw{width:13px;height:13px;border-radius:3px;display:inline-block}
#panel .dm{font-size:11px;color:#8295ab;text-transform:uppercase;letter-spacing:.4px;margin-bottom:8px}
#panel .bl{font-size:12.5px;color:#46566b;font-style:italic;margin:6px 0}
#panel .kv{font-size:12px;margin:7px 0;color:#33455c}#panel .kv b{color:#5a6b80;font-weight:600}
#panel .rel{font-size:11.5px;padding:3px 0;border-top:1px solid #eef2f7;color:#3a4658}
#panel .rel .p{color:#8295ab}
#panel .ds{font-size:11px;color:#7a8aa0;line-height:1.5}
#legend{position:absolute;left:14px;bottom:14px;background:rgba(255,255,255,.94);border:1px solid #dde4ee;border-radius:9px;padding:9px 11px;box-shadow:0 3px 14px rgba(31,58,95,.10);font-size:11.5px}
#legend .lt{font-weight:600;color:#42536a;margin:0 0 6px;font-size:10.5px;letter-spacing:.5px;text-transform:uppercase}
#legend .row{display:flex;align-items:center;gap:7px;padding:1.5px 0;cursor:pointer}
#legend .row.off{opacity:.35}
#legend .sw{width:12px;height:12px;border-radius:3px;flex:none}
#hint{position:absolute;left:50%;bottom:14px;transform:translateX(-50%);background:rgba(255,255,255,.92);border:1px solid #dde4ee;border-radius:8px;padding:6px 12px;font-size:11.5px;color:#56657a;box-shadow:0 3px 14px rgba(31,58,95,.10)}
#hint b{color:#33455c}
</style></head><body>
<div id='hdr'><span class='desc'>Each node is a biolink category; hover any node to trace its typed relationships across the BioBTree knowledge graph.</span>
 <span class='sp'></span><a href='https://github.com/tamerh/biobtree' target='_blank' rel='noopener'>GitHub</a></div>
<div id='stage'>
 <svg id='viz'></svg>
 <div id='legend'><div class='lt'>Domains</div></div>
 <div id='panel'></div>
 <div id='hint'><b>Hover</b> a node to trace its relationships</div>
</div>
<script>
var D=__PAYLOAD__;
// embedded in an iframe on the site: drop the header band + hint so only the graph
// shows; the host page supplies the title/description/links as normal text.
if(/[?&]embed\b/.test(location.search)){['hdr','hint'].forEach(function(id){var e=document.getElementById(id);if(e)e.style.display='none';});
 // transparent so the host page's hero background shows through the iframe (keep the dots)
 document.documentElement.style.background='transparent';document.body.style.background='transparent';
 var _st=document.getElementById('stage');if(_st)_st.style.background='transparent';}
if(/[?&]dark\b/.test(location.search))document.body.classList.add('dark');
var byId={};D.nodes.forEach(function(n){byId[n.id]=n;});
var adj={};D.nodes.forEach(function(n){adj[n.id]={out:[],in:[]};});
D.edges.forEach(function(e){adj[e.source].out.push(e);adj[e.target].in.push(e);});
var lg=d3.select('#legend');
D.domains.forEach(function(dm){
 lg.append('div').attr('class','row').html("<span class='sw' style='background:"+dm.color+"'></span>"+dm.label)
   .on('mouseenter',function(){if(window._hlDomain)window._hlDomain(dm.key);})
   .on('mouseleave',function(){if(window._clrHi)window._clrHi();});
});
function panel(n){
 var p=d3.select('#panel');
 var rels=adj[n.id].out.map(function(e){return {dir:'→',o:e.target,preds:e.preds};})
   .concat(adj[n.id].in.map(function(e){return {dir:'←',o:e.source,preds:e.preds};}));
 var h="<h2><span class='sw' style='background:"+n.color+"'></span>"+n.id+"</h2>";
 h+="<div class='dm'>"+(D.domains.filter(function(d){return d.key==n.domain;})[0]||{label:n.domain}).label+"</div>";
 if(n.blurb)h+="<div class='bl'>"+n.blurb+"</div>";
 if(n.example)h+="<div class='kv'><b>example</b> "+n.example+"</div>";
 if(n.datasets.length)h+="<div class='kv'><b>"+n.nds+" datasets</b></div><div class='ds'>"+n.datasets.join(', ')+"</div>";
 if(n.loops.length)h+="<div class='kv'>↻ "+n.loops.join(', ')+"</div>";
 if(n.badges.length)h+="<div class='kv'>🌐 "+n.badges.join(', ')+"</div>";
 h+="<div class='kv' style='margin-top:10px'><b>"+rels.length+" relationships</b></div>";
 rels.forEach(function(r){h+="<div class='rel'>"+r.dir+" "+r.o+" <span class='p'>"+r.preds.join(', ')+"</span></div>";});
 p.html(h).classed('show',true);
}
function hidePanel(){d3.select('#panel').classed('show',false);}
</script>
"""


_BUNDLE_TMPL = _NEAT_HEAD + r"""<script>
(function(){
 var svg=d3.select('#viz'),stage=document.getElementById('stage');
 var g=svg.append('g');
 var linkG=g.append('g'),nodeG=g.append('g'),labelG=g.append('g');
 function layout(){
  var W=stage.clientWidth,H=stage.clientHeight;
  svg.attr('viewBox',[-W/2,-H/2,W,H]);
  var R=Math.min(W,H)/2-150;
  // hierarchy root -> domain -> category, then radial cluster
  var rootData={name:'root',children:D.domains.map(function(dm){
    return {name:dm.key,children:D.nodes.filter(function(n){return n.domain==dm.key;})
      .map(function(n){return {name:n.id,data:n};})};})};
  var root=d3.hierarchy(rootData);
  d3.cluster().size([2*Math.PI,R])(root);
  var leaves=root.leaves(),leafById={};
  leaves.forEach(function(l){leafById[l.data.name]=l;});
  var line=d3.lineRadial().curve(d3.curveBundle.beta(0.82)).radius(function(d){return d.y;}).angle(function(d){return d.x;});
  var linkData=D.edges.filter(function(e){return leafById[e.source]&&leafById[e.target];})
    .map(function(e){return {e:e,path:leafById[e.source].path(leafById[e.target])};});
  var links=linkG.selectAll('path').data(linkData).join('path').attr('class','link')
    .attr('d',function(d){return line(d.path);})
    .attr('stroke',function(d){return byId[d.e.source].color;});
  var nodes=nodeG.selectAll('g').data(leaves).join('g')
    .attr('transform',function(d){return 'rotate('+(d.x*180/Math.PI-90)+') translate('+d.y+',0)';});
  nodes.selectAll('circle').data(function(d){return [d];}).join('circle').attr('class','node-dot')
    .attr('r',function(d){return 3+Math.min(6,d.data.data.deg*0.5);}).attr('fill',function(d){return d.data.data.color;});
  nodes.selectAll('text').data(function(d){return [d];}).join('text').attr('class','node-lbl')
    .attr('dy','0.31em').attr('x',function(d){return d.x<Math.PI?8:-8;})
    .attr('text-anchor',function(d){return d.x<Math.PI?'start':'end';})
    .attr('transform',function(d){return d.x>=Math.PI?'rotate(180)':null;})
    .text(function(d){return d.data.name;});
  function hl(id){
   var inc={};links.classed('faded',true).classed('lit',false);labelG.selectAll('*').remove();
   links.filter(function(d){return d.e.source==id||d.e.target==id;})
     .classed('faded',false).classed('lit',true).raise()
     .each(function(d){inc[d.e.source]=1;inc[d.e.target]=1;
       var L=this.getTotalLength(),p=this.getPointAtLength(L*(d.e.source==id?0.8:0.2));
       labelG.append('text').attr('class','elbl').attr('x',p.x).attr('y',p.y).text(d.e.preds.join(', '));});
   nodes.classed('faded',function(d){return !inc[d.data.name];});
   nodes.select('text').classed('hot',function(d){return d.data.name==id;});
   panel(byId[id]);
  }
  function clr(){links.classed('faded',false).classed('lit',false);labelG.selectAll('*').remove();nodes.classed('faded',false);nodes.select('text').classed('hot',false);hidePanel();}
  nodes.on('mouseover',function(_,d){hl(d.data.name);}).on('mouseout',clr);
  // hovering a legend domain: light up every relationship touching that domain
  window._hlDomain=function(key){
   var inc={};links.classed('faded',true).classed('lit',false);labelG.selectAll('*').remove();
   links.filter(function(d){return byId[d.e.source].domain==key||byId[d.e.target].domain==key;})
     .classed('faded',false).classed('lit',true).raise()
     .each(function(d){inc[d.e.source]=1;inc[d.e.target]=1;
       var L=this.getTotalLength(),p=this.getPointAtLength(L/2);
       labelG.append('text').attr('class','elbl').attr('x',p.x).attr('y',p.y).text(d.e.preds.join(', '));});
   nodes.classed('faded',function(d){return !(d.data.data.domain==key||inc[d.data.name]);});
   nodes.select('text').classed('hot',function(d){return d.data.data.domain==key;});
  };
  window._clrHi=clr;
 }
 layout();
 svg.call(d3.zoom().scaleExtent([0.5,6]).on('zoom',function(ev){g.attr('transform',ev.transform);}));
 window.addEventListener('resize',function(){g.selectAll('*').remove();linkG=g.append('g');nodeG=g.append('g');labelG=g.append('g');layout();});
})();
</script></body></html>"""


_ARC_TMPL = _NEAT_HEAD + r"""<style>
/* arc view: horizontal domain legend centred below the arc (overrides shared head) */
#legend{left:50%;right:auto;bottom:0;transform:translateX(-50%);display:flex;flex-wrap:wrap;
 justify-content:center;gap:8px 18px;max-width:92%;border:none;background:none;box-shadow:none;padding:0}
#legend .lt{display:none}
#legend .row{padding:0}
#hint{left:auto;right:14px;transform:none}
</style>
<script>
(function(){
 var svg=d3.select('#viz'),stage=document.getElementById('stage');
 var g=svg.append('g');var linkG=g.append('g'),nodeG=g.append('g'),labelG=g.append('g');
 var SP=42,PADX=70,BASE_FRAC=0.62;
 function layout(){
  var H=stage.clientHeight;
  var ids=D.nodes.map(function(n){return n.id;});
  var W=PADX*2+(ids.length-1)*SP;
  svg.attr('viewBox',[0,0,Math.max(W,stage.clientWidth),H]);
  var x=d3.scalePoint().domain(ids).range([PADX,PADX+(ids.length-1)*SP]);
  var base=H*BASE_FRAC;
  var links=linkG.selectAll('path').data(D.edges).join('path').attr('class','link')
    .attr('stroke',function(e){return byId[e.source].color;})
    .attr('d',function(e){var x1=x(e.source),x2=x(e.target),r=Math.abs(x2-x1)/2;
      return 'M'+x1+','+base+' A'+r+','+r+' 0 0 '+(x1<x2?1:0)+' '+x2+','+base;});
  var nodes=nodeG.selectAll('g').data(D.nodes).join('g').attr('transform',function(n){return 'translate('+x(n.id)+','+base+')';});
  nodes.append('rect').attr('class','node-dot').attr('x',-7).attr('y',-9).attr('width',14).attr('height',18).attr('rx',3).attr('fill',function(n){return n.color;});
  nodes.append('text').attr('class','node-lbl').attr('transform','rotate(45)').attr('x',12).attr('y',2).text(function(n){return n.id;});
  function hl(id){var inc={};
   links.classed('faded',true).classed('lit',false);labelG.selectAll('*').remove();
   links.filter(function(e){return e.source==id||e.target==id;}).classed('faded',false).classed('lit',true).raise()
     .each(function(e){inc[e.source]=1;inc[e.target]=1;
       var L=this.getTotalLength(),p=this.getPointAtLength(L/2);
       labelG.append('text').attr('class','elbl').attr('x',p.x).attr('y',p.y-2).text(e.preds.join(', '));});
   nodes.classed('faded',function(n){return !inc[n.id];});
   nodes.select('text').classed('hot',function(n){return n.id==id;});
   panel(byId[id]);
  }
  function clr(){links.classed('faded',false).classed('lit',false);labelG.selectAll('*').remove();nodes.classed('faded',false);nodes.select('text').classed('hot',false);hidePanel();}
  // click a node -> trace ALL downstream paths (transitive closure over outgoing
  // edges), sticky until you click empty space or the node again. hover = 1-hop peek.
  var pinned=null;
  function reachDir(id,dir){var seen={};seen[id]=1;var q=[id];while(q.length){var c=q.shift();
    adj[c][dir].forEach(function(e){var nx=dir=='out'?e.target:e.source;if(!seen[nx]){seen[nx]=1;q.push(nx);}});}return seen;}
  function showPaths(id){var seen=reachDir(id,'out');
    if(Object.keys(seen).length<2)seen=reachDir(id,'in');   // sink node -> trace upstream paths instead
    labelG.selectAll('*').remove();
    links.classed('faded',true).classed('lit',false);
    links.filter(function(e){return seen[e.source]&&seen[e.target];}).classed('faded',false).classed('lit',true).raise()
      .each(function(e){var L=this.getTotalLength(),p=this.getPointAtLength(L/2);
        labelG.append('text').attr('class','elbl').attr('x',p.x).attr('y',p.y-2).text(e.preds.join(', '));});
    nodes.classed('faded',function(n){return !seen[n.id];});
    nodes.select('text').classed('hot',function(n){return n.id==id;});
    panel(byId[id]);}
  nodes.on('mouseover',function(_,n){if(pinned&&pinned!=n.id)pinned=null;/* hovering another node drops the pin */ if(!pinned)hl(n.id);})
       .on('mouseout',function(){if(!pinned)clr();})
       .on('click',function(ev,n){ev.stopPropagation();if(pinned==n.id){pinned=null;clr();}else{pinned=n.id;showPaths(n.id);}});
  svg.on('click',function(){if(pinned){pinned=null;clr();}});
  // hovering a legend domain: light up every relationship touching that domain
  window._hlDomain=function(key){
   var inc={};links.classed('faded',true).classed('lit',false);labelG.selectAll('*').remove();
   links.filter(function(e){return byId[e.source].domain==key||byId[e.target].domain==key;})
     .classed('faded',false).classed('lit',true).raise()
     .each(function(e){inc[e.source]=1;inc[e.target]=1;
       var L=this.getTotalLength(),p=this.getPointAtLength(L/2);
       labelG.append('text').attr('class','elbl').attr('x',p.x).attr('y',p.y-2).text(e.preds.join(', '));});
   nodes.classed('faded',function(n){return !(n.domain==key||inc[n.id]);});
   nodes.select('text').classed('hot',function(n){return n.domain==key;});
  };
  window._clrHi=function(){if(pinned)showPaths(pinned);else clr();};
  // initial fit: frame the FULL rendered content (incl. the tallest arc + labels)
  // with padding, biased slightly left so the big top arc is fully visible.
  var bb=g.node().getBBox();
  var pad=28;
  var scale=Math.min((stage.clientWidth-pad*2)/bb.width,(stage.clientHeight-pad*2)/bb.height,1.3);
  var freeX=stage.clientWidth-bb.width*scale;
  var tx=Math.max(pad,freeX*0.5)-bb.x*scale;     // centred horizontally
  var ty=pad-bb.y*scale;                          // top-padded -> tallest arc visible
  var t=d3.zoomIdentity.translate(tx,ty).scale(scale);
  // embedded (in an iframe on the site): static fit, no scroll/drag zoom hijack.
  // full page: interactive pan/zoom, seeded from the fit.
  if(EMBED){ g.attr('transform',t.toString()); }
  else { svg.call(zoom); svg.call(zoom.transform,t); }
 }
 var EMBED=/[?&]embed\b/.test(location.search);
 var zoom=d3.zoom().scaleExtent([0.3,5]).on('zoom',function(ev){g.attr('transform',ev.transform);});
 layout();
 window.addEventListener('resize',function(){g.selectAll('*').remove();linkG=g.append('g');nodeG=g.append('g');labelG=g.append('g');layout();});
})();
</script></body></html>"""


def print_summary(triples):
    by_subj = defaultdict(list)
    for (s, p, o), ds in triples.items():
        by_subj[s].append((p, o, ds))
    cats = sorted({c for (s, _, o) in triples for c in (s, o)})
    print(f"NODE TYPES ({len(cats)}): " + ", ".join(c.split(':')[1] for c in cats))
    print(f"SCHEMA EDGES ({len(triples)} category->predicate->category):\n")
    for s in sorted(by_subj):
        for p, o, ds in sorted(by_subj[s]):
            print(f"  {s.split(':')[1]:>22} --{p.split(':')[1]}--> {o.split(':')[1]}  [{len(ds)}]")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--categories", default=str(_MS_MAP / "categories.yaml"))
    ap.add_argument("--predicates", default=str(_MS_MAP / "predicates.yaml"))
    ap.add_argument("--conf", default="conf", help="dataset config dir (for ontology attribution)")
    ap.add_argument("--out", default=None, help="pyvis HTML output path")
    ap.add_argument("--mermaid", default=None, help="Mermaid HTML output path (cleaner)")
    ap.add_argument("--cytoscape", default=None, help="Cytoscape.js interactive HTML")
    ap.add_argument("--explorer", default=None,
                    help="Combined Graph (edges-on-click) + Matrix explorer HTML")
    ap.add_argument("--poster", default=None,
                    help="Static everything-visible band poster HTML (preset layout)")
    ap.add_argument("--er", default=None,
                    help="ER/flow schema diagram HTML (Graphviz dot, rankdir=LR)")
    ap.add_argument("--er-png", default=None, help="optional PNG preview for --er")
    ap.add_argument("--bundle", default=None, help="Radial edge-bundling view HTML (D3)")
    ap.add_argument("--arc", default=None, help="Arc-diagram view HTML (D3)")
    ap.add_argument("--print", action="store_true", dest="show")
    a = ap.parse_args()
    cats = CategoryMap.load(a.categories)
    preds = PredicateMap.load(a.predicates)
    registry = DatasetRegistry.load(a.conf)
    triples = schema_triples(cats, preds, registry)
    # primary (own-records) datasets = source1 + source2; everything else typed as
    # a node is a cross-reference/identifier namespace (xref1/xref2 -> stub nodes).
    import json as _json
    from pathlib import Path as _Path
    primary_names = set()
    for fn in ("source1.dataset.json", "source2.dataset.json"):
        p = _Path(a.conf) / fn
        if p.exists():
            primary_names |= set(_json.loads(p.read_text()))
    if a.show or not (a.out or a.mermaid or a.cytoscape or a.explorer or a.poster or a.er or a.bundle or a.arc):
        print_summary(triples)
    if a.out:
        render_html(triples, a.out)
    if a.mermaid:
        render_mermaid(triples, a.mermaid)
    if a.cytoscape:
        render_cytoscape(triples, a.cytoscape)
        print(f"wrote {a.cytoscape}: {len({c for (s,_,o) in triples for c in (s,o)})} node types, {len(triples)} edges")
    if a.explorer:
        render_explorer(triples, a.explorer, cats, primary_names, registry)
        print(f"wrote {a.explorer}: {len({c for (s,_,o) in triples for c in (s,o)})} node types, {len(triples)} edges")
    if a.poster:
        render_poster(triples, a.poster, cats, primary_names, registry)
        print(f"wrote {a.poster}: {len({c for (s,_,o) in triples for c in (s,o)})} node types, {len(triples)} edges")
    if a.er:
        render_er(triples, a.er, cats, primary_names, registry, png=a.er_png)
        print(f"wrote {a.er}: {len({c for (s,_,o) in triples for c in (s,o)})} node types, {len(triples)} edges")
    if a.bundle:
        render_bundle(triples, a.bundle, cats, primary_names, registry)
        print(f"wrote {a.bundle}")
    if a.arc:
        render_arc(triples, a.arc, cats, primary_names, registry)
        print(f"wrote {a.arc}")


if __name__ == "__main__":
    main()
