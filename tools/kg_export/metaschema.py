"""Meta-graph (schema view): biolink category --predicate--> category, derived
from the mapping tables (categories.yaml + predicates.yaml + the GO rules). Shows
the big-picture shape of the KG -- what node types exist and how they connect --
independent of any instance data.

    python -m tools.kg_export.metaschema --out kg_meta.html [--print]
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

# dbSNP layer (built by tools/dbsnp_py/extract.py, not a predicate pair): variant->gene
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
  <a href='./index.html'>Docs</a><a href='./sub/'>Subgraph</a>
  <a href='https://github.com/tamerhuseyin/biobtree' target='_blank' rel='noopener'>GitHub</a></nav>
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
 and the relationships it participates in. A separate importable <b>subgraph</b> (human + disease +
 molecule) and the full docs are linked in the header.</p></div>
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
 <a href='./index.html'>Docs</a><a href='./sub/'>Subgraph</a>
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
    ap.add_argument("--categories", default="mappings/categories.yaml")
    ap.add_argument("--predicates", default="mappings/predicates.yaml")
    ap.add_argument("--conf", default="conf", help="dataset config dir (for ontology attribution)")
    ap.add_argument("--out", default=None, help="pyvis HTML output path")
    ap.add_argument("--mermaid", default=None, help="Mermaid HTML output path (cleaner)")
    ap.add_argument("--cytoscape", default=None, help="Cytoscape.js interactive HTML")
    ap.add_argument("--explorer", default=None,
                    help="Combined Graph (edges-on-click) + Matrix explorer HTML")
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
    if a.show or not (a.out or a.mermaid or a.cytoscape or a.explorer):
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


if __name__ == "__main__":
    main()
