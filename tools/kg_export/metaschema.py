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
from .predicates import PredicateMap

# GO annotation sources (go.py): subject dataset -> category, and the 3 aspects.
_GO_SOURCES = [("uniprot", "biolink:Protein"), ("ensembl", "biolink:Gene")]
_GO_EDGES = [
    ("biolink:enables", "biolink:MolecularActivity"),
    ("biolink:actively_involved_in", "biolink:BiologicalProcess"),
    ("biolink:located_in", "biolink:CellularComponent"),
]

_PALETTE = [
    "#4e79a7", "#f28e2b", "#59a14f", "#e15759", "#76b7b2", "#edc948",
    "#b07aa1", "#ff9da7", "#9c755f", "#bab0ac", "#86bc86", "#d37295",
    "#7f7f7f", "#17becf", "#bcbd22", "#aec7e8", "#ffbb78", "#98df8a",
]


def schema_triples(cats: CategoryMap, preds: PredicateMap):
    """(subject_category, predicate, object_category) -> set of contributing datasets."""
    triples: dict[tuple, set] = defaultdict(set)

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
            add(cats.category_for(r.subject), r.predicate, cats.category_for(r.object), ds)

    for src_ds, sc in _GO_SOURCES:
        for p, oc in _GO_EDGES:
            add(sc, p, oc, f"go({src_ds})")
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


def render_explorer(triples, out_html, catmap=None):
    """Combined viewer: Graph (edges revealed on node click) + Matrix grid,
    with a right panel that lists the contributing BioBTree datasets on click."""
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
                                "target": nid(o), "label": pl,
                                "n": len(ds), "datasets": sorted(ds)}})
    # category -> the BioBTree node datasets typed as it (with CURIE prefix)
    node_ds = defaultdict(list)
    if catmap is not None:
        for ds in sorted(catmap.datasets()):
            e = catmap.entry_for(ds)
            if e:
                node_ds[nid(e.category)].append({"ds": ds, "prefix": e.prefix})
    # GO is typed at runtime by term aspect (go.py), so it has no categories.yaml
    # entry -- inject it as the node source for the three GO aspect categories.
    for aspect_cat in ("biolink:MolecularActivity", "biolink:BiologicalProcess",
                       "biolink:CellularComponent"):
        node_ds[nid(aspect_cat)].append({"ds": "go", "prefix": "GO"})
    payload = json.dumps({"nodes": nodes, "edges": edges, "cats": [nid(c) for c in cats],
                          "colors": color, "nodeDatasets": node_ds})
    tmpl = r"""<!doctype html><html><head><meta charset='utf-8'>
<script src='https://unpkg.com/cytoscape@3.28.1/dist/cytoscape.min.js'></script>
<script src='https://unpkg.com/dagre@0.8.5/dist/dagre.min.js'></script>
<script src='https://unpkg.com/cytoscape-dagre@2.5.0/cytoscape-dagre.js'></script>
<style>html,body{margin:0;height:100%;font-family:sans-serif;font-size:13px}
#bar{height:42px;display:flex;gap:6px;align-items:center;padding:0 10px;border-bottom:1px solid #ddd;flex-wrap:wrap}
button{padding:4px 9px;cursor:pointer}.on{background:#e15759;color:#fff}
#main{display:flex;height:calc(100% - 42px)}
#left{flex:1;position:relative;min-width:0}
#cy{width:100%;height:100%;background:#fafafa}
#matrix{position:absolute;top:0;left:0;width:100%;height:100%;box-sizing:border-box;overflow:auto;padding:10px;display:none;background:#fff}
#side{width:330px;border-left:1px solid #ddd;overflow:auto;padding:12px 14px;box-sizing:border-box;background:#fff}
table{border-collapse:collapse}td,th{border:1px solid #eee;padding:2px 5px;font-size:11px;text-align:center}
th.rot{height:120px;white-space:nowrap}th.rot div{transform:rotate(-60deg);width:18px}
td.rh{text-align:right;font-weight:bold;white-space:nowrap}
td.cell{cursor:pointer}.muted{color:#bbb}
#side h2{font-size:15px;margin:0 0 2px}#side h3{font-size:12px;color:#888;text-transform:uppercase;letter-spacing:.04em;margin:14px 0 5px}
#side .hint{color:#aaa;font-size:12px;margin-top:8px}
.ds{display:inline-block;font-family:monospace;font-size:11px;background:#f1f1f1;border-radius:3px;padding:1px 5px;margin:2px 3px 0 0}
.rel{margin:4px 0;padding:5px 7px;border-radius:4px;background:#fafafa;border:1px solid #eee}
.rel .p{font-weight:bold;color:#c0392b}.rel .c{color:#2c6}.rel .src{font-family:monospace;font-size:10px;color:#777;display:block;margin-top:2px}
.swatch{display:inline-block;width:11px;height:11px;border-radius:2px;vertical-align:middle;margin-right:5px}</style></head>
<body><div id='bar'>
<b>BioBTree KG schema</b>
<button id='bG' class='on' onclick="view('g')">Graph</button>
<button id='bM' onclick="view('m')">Matrix</button>
<span id='gc'>| layout:
<button onclick="lay('dagre-LR')">Layered&rarr;</button>
<button onclick="lay('concentric')">Hubs</button>
<button onclick="lay('cose')">Force</button>
| edges:
<button id='eC' class='on' onclick="emode('click')">on click</button>
<button id='eA' onclick="emode('all')">show all</button>
<span style='color:#888'>&nbsp;click a node to reveal connections + datasets</span></span>
</div>
<div id='main'><div id='left'><div id='cy'></div><div id='matrix'></div></div>
<div id='side'><h2>Details</h2><div class='hint'>Click a node (or a matrix cell) to see the BioBTree datasets and relationships behind it.</div></div></div>
<script>
var D=__PAYLOAD__;
var cy=cytoscape({container:document.getElementById('cy'),elements:{nodes:D.nodes,edges:D.edges},
 style:[
  {selector:'node',style:{'background-color':'data(color)','label':'data(label)','font-size':11,'text-valign':'center','color':'#111','text-outline-color':'#fff','text-outline-width':2,'width':40,'height':40}},
  {selector:'edge',style:{'label':'data(label)','font-size':8,'color':'#555','curve-style':'bezier','target-arrow-shape':'triangle','line-color':'#bbb','target-arrow-color':'#bbb','width':1.2,'text-rotation':'autorotate','text-background-color':'#fff','text-background-opacity':0.85}},
  {selector:'edge.hidden',style:{'display':'none'}},
  {selector:'.faded',style:{'opacity':0.12}},
  {selector:'.hi',style:{'line-color':'#e15759','target-arrow-color':'#e15759','width':2.6,'color':'#900'}}
 ]});
var EMODE='click';
function lay(name){var o={name:'cose'};
 if(name=='dagre-LR')o={name:'dagre',rankDir:'LR',nodeSep:38,rankSep:150};
 if(name=='concentric')o={name:'concentric',concentric:function(n){return n.degree()},levelWidth:function(){return 2},minNodeSpacing:45};
 cy.layout(o).run();}
function emode(m){EMODE=m;document.getElementById('eC').className=m=='click'?'on':'';document.getElementById('eA').className=m=='all'?'on':'';
 if(m=='all'){cy.edges().removeClass('hidden');cy.elements().removeClass('faded hi');}
 else{cy.edges().addClass('hidden');cy.elements().removeClass('faded hi');}}
cy.on('tap','node',function(e){var n=e.target;
 if(EMODE=='click'){cy.edges().addClass('hidden');cy.elements().removeClass('faded hi');
  var ce=n.connectedEdges();ce.removeClass('hidden').addClass('hi');
  cy.elements().addClass('faded');n.closedNeighborhood().removeClass('faded');}
 showNode(n.id());});
cy.on('tap',function(e){if(e.target===cy){if(EMODE=='click'){cy.edges().addClass('hidden');cy.elements().removeClass('faded hi');}resetPanel();}});
function esc(s){return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');}
function dsChips(list){return list.map(function(d){return "<span class='ds'>"+esc(d)+"</span>";}).join('');}
function resetPanel(){document.getElementById('side').innerHTML="<h2>Details</h2><div class='hint'>Click a node (or a matrix cell) to see the BioBTree datasets and relationships behind it.</div>";}
function showNode(cat){var col=D.colors[cat]||'#888';
 var nd=(D.nodeDatasets[cat]||[]);
 var out=[],inc=[];
 D.edges.forEach(function(e){var d=e.data;if(d.source==cat)out.push(d);if(d.target==cat)inc.push(d);});
 var h="<h2><span class='swatch' style='background:"+col+"'></span>"+esc(cat)+"</h2>";
 h+="<h3>Node datasets ("+nd.length+")</h3>";
 h+=nd.length?"<div>"+nd.map(function(x){return "<span class='ds' title='CURIE prefix: "+esc(x.prefix)+"'>"+esc(x.ds)+"</span>";}).join('')+"</div>"
            :"<div class='hint'>No primary node dataset — appears only as an edge endpoint / stub.</div>";
 function relBlock(d,dir){var other=dir=='out'?d.target:d.source;var arrow=dir=='out'?'&rarr;':'&larr;';
  return "<div class='rel'><span class='p'>"+esc(d.label)+"</span> "+arrow+" <span class='c'>"+esc(other)+"</span>"
   +"<span class='src'>"+esc(d.datasets.join(', '))+"</span></div>";}
 h+="<h3>Outgoing ("+out.length+")</h3>"+(out.length?out.map(function(d){return relBlock(d,'out');}).join(''):"<div class='hint'>none</div>");
 h+="<h3>Incoming ("+inc.length+")</h3>"+(inc.length?inc.map(function(d){return relBlock(d,'in');}).join(''):"<div class='hint'>none</div>");
 document.getElementById('side').innerHTML=h;}
function showCell(s,o){var es=D.edges.filter(function(e){return e.data.source==s&&e.data.target==o;});
 var h="<h2>"+esc(s)+" &rarr; "+esc(o)+"</h2><h3>Relationships ("+es.length+")</h3>";
 h+=es.map(function(e){return "<div class='rel'><span class='p'>"+esc(e.data.label)+"</span><span class='src'>"+esc(e.data.datasets.join(', '))+"</span></div>";}).join('');
 document.getElementById('side').innerHTML=h;}
function view(v){var g=v=='g';document.getElementById('cy').style.display=g?'block':'none';
 document.getElementById('matrix').style.display=g?'none':'block';
 document.getElementById('gc').style.display=g?'inline':'none';
 document.getElementById('bG').className=g?'on':'';document.getElementById('bM').className=g?'':'on';
 if(!g&&!document.getElementById('matrix').dataset.built){buildMatrix();}}
function buildMatrix(){var m={};D.edges.forEach(function(e){var k=e.data.source+'>'+e.data.target;(m[k]=m[k]||[]).push(e.data.label);});
 var c=D.cats,h='<table><tr><th></th>';c.forEach(function(o){h+="<th class='rot'><div>"+o+"</div></th>";});h+='</tr>';
 c.forEach(function(s){h+="<td class='rh' style='color:"+D.colors[s]+"'>"+s+"</td>";
  c.forEach(function(o){var v=m[s+'>'+o];if(v){h+="<td class='cell' onclick=\"showCell('"+s+"','"+o+"')\" title='"+s+' &rarr; '+o+":\n"+v.join('\n')+"' style='background:"+D.colors[s]+"33'>"+(v.length>1?v.length:'&bull;')+"</td>";}else{h+="<td class='muted'></td>";}});h+='</tr>';});
 h+='</table>';document.getElementById('matrix').innerHTML=h;document.getElementById('matrix').dataset.built=1;}
lay('dagre-LR');emode('click');
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
    ap.add_argument("--out", default=None, help="pyvis HTML output path")
    ap.add_argument("--mermaid", default=None, help="Mermaid HTML output path (cleaner)")
    ap.add_argument("--cytoscape", default=None, help="Cytoscape.js interactive HTML")
    ap.add_argument("--explorer", default=None,
                    help="Combined Graph (edges-on-click) + Matrix explorer HTML")
    ap.add_argument("--print", action="store_true", dest="show")
    a = ap.parse_args()
    cats = CategoryMap.load(a.categories)
    preds = PredicateMap.load(a.predicates)
    triples = schema_triples(cats, preds)
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
        render_explorer(triples, a.explorer, cats)
        print(f"wrote {a.explorer}: {len({c for (s,_,o) in triples for c in (s,o)})} node types, {len(triples)} edges")


if __name__ == "__main__":
    main()
