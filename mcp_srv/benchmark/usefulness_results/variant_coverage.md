# Variant annotation-coverage matrix (30 Atlas variants)

| variant | clinvar | gnomad_variant | alphamissense | revel | spliceai | conservation | saprot | n |
|---|--|--|--|--|--|--|--|--|
| pten:ala120thr | Y | · | Y | Y | · | Y | Y | 5 |
| pten:ala121glu | Y | · | Y | Y | · | Y | Y | 5 |
| pten:ala121pro | Y | · | Y | Y | · | Y | Y | 5 |
| pten:ala121ser | Y | · | Y | Y | · | Y | Y | 5 |
| pten:ala121thr | Y | · | Y | Y | · | Y | Y | 5 |
| pten:ala126asp | Y | · | Y | Y | · | Y | Y | 5 |
| asxl1:ala1091thr | Y | · | Y | Y | · | Y | Y | 5 |
| asxl1:ala1482thr | Y | · | Y | Y | · | Y | Y | 5 |
| asxl1:ala14val | Y | · | Y | Y | Y | Y | Y | 6 |
| asxl1:ala195thr | Y | Y | Y | Y | · | Y | Y | 6 |
| asxl1:ala215thr | Y | Y | Y | Y | · | Y | Y | 6 |
| asxl1:ala530val | Y | Y | Y | Y | · | Y | Y | 6 |
| acta1:ala116pro | Y | · | Y | Y | · | Y | Y | 5 |
| acta1:ala116ser | Y | · | Y | Y | · | Y | Y | 5 |
| acta1:ala116thr | Y | · | Y | Y | · | Y | Y | 5 |
| acta1:ala116val | Y | · | Y | Y | · | Y | Y | 5 |
| acta1:ala140asp | Y | · | Y | Y | · | Y | Y | 5 |
| acta1:ala140gly | Y | · | Y | Y | · | Y | Y | 5 |
| naa10:ala193pro | Y | Y | Y | Y | · | Y | Y | 6 |
| naa10:ala76thr | Y | · | Y | Y | · | Y | Y | 5 |
| naa10:ala87ser | Y | · | Y | Y | · | Y | Y | 5 |
| naa10:arg116gln | Y | · | Y | Y | · | Y | Y | 5 |
| naa10:arg116trp | Y | · | Y | Y | · | Y | Y | 5 |
| naa10:arg149gly | Y | · | Y | Y | · | Y | Y | 5 |
| rpl10:ala151val | Y | · | Y | Y | · | Y | Y | 5 |
| rpl10:ala64val | Y | · | Y | Y | · | Y | Y | 5 |
| rpl10:arg189trp | Y | Y | Y | Y | · | Y | Y | 6 |
| rpl10:arg32leu | Y | · | Y | Y | · | Y | Y | 5 |
| rpl10:arg3his | Y | · | Y | Y | · | Y | Y | 5 |
| rpl10:gly161asp | Y | · | Y | Y | · | Y | Y | 5 |

**Per-source coverage (of 30 variants):**
- clinvar: 30/30 (100%)
- gnomad_variant: 5/30 (17%)
- alphamissense: 30/30 (100%)
- revel: 30/30 (100%)
- spliceai: 1/30 (3%)
- conservation: 30/30 (100%)
- saprot: 30/30 (100%)

**Multi-source depth:** mean 5.2 sources/variant.
- covered by >=3 sources: 30/30 (100%)
- covered by >=4 sources: 30/30 (100%)
- covered by >=5 sources: 30/30 (100%)
- covered by >=6 sources: 6/30 (20%)

_One biobtree query surfaces all of the above per variant; VarSome/Franklin/OpenCRAVAT each expose a subset, and none co-serve MaveDB + an owned unsupervised PLM (SaProt)._
