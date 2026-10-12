#!/usr/bin/env python3
"""
Prepare SpliceAI raw + masked MANE VCFs for biobtree's raw/masked merge-join.

Background: the Go merge-join (src/update/spliceai.go) walks raw + masked in
lockstep by (chrRank,pos), so BOTH files must be sorted the SAME way. The
published Ensembl MANE VCFs are NOT co-sorted (chr18 is dumped at the end, and
raw/masked disagree there: raw ...X,Y,18 vs masked ...X,18,Y), so a direct merge
silently drops chr18's masked deltas. The files are also huge (~27GB gz each,
~200GB uncompressed) and fragile to stream for hours off FTP.

Strategy — FILTER FIRST, then co-sort the small result:
  1. download both .vcf.gz locally (curl -C -, resumable);
  2. RAW: keep only rows whose SpliceAI max delta >= 0.2 (exactly what the Go
     parser stores), prepend the biobtree chrRank, sort by (chrRank,pos), emit
     a small co-sorted VCF + the set of kept variant keys (chr:pos:ref:alt);
  3. MASKED: keep only rows whose key is in the RAW-kept set (NOT by masked's own
     score — a kept variant can have masked==0, which is the negative-evidence
     the Variant team wants), co-sort the same way.
Both outputs are small and identically sorted, so the Go merge is correct and
fast, and reads local files (no mid-stream FTP failure).

Output:
  raw_data/spliceai/spliceai_raw.sorted.vcf.gz
  raw_data/spliceai/spliceai_masked.sorted.vcf.gz
Run from the repo root. Completed steps are skipped on re-run.
"""
import os
import sys
import subprocess

BASE = "https://ftp.ensembl.org/pub/current_variation/SpliceAI"
OUT_DIR = "raw_data/spliceai"
TMP_DIR = os.path.join(OUT_DIR, "sorttmp")
DL = {
    "raw": "spliceai_scores.raw.snv.ensembl_mane_v1.4.grch38.vcf.gz",
    "masked": "spliceai_scores.masked.snv.ensembl_mane_v1.4.grch38.vcf.gz",
}
RAW_SORTED = os.path.join(OUT_DIR, "spliceai_raw.sorted.vcf.gz")
MASKED_SORTED = os.path.join(OUT_DIR, "spliceai_masked.sorted.vcf.gz")
RAW_KEYS = os.path.join(OUT_DIR, "spliceai_raw.keys")

THRESHOLD = "0.2"

# chrRank prefix matching chrRank() in spliceai.go (1..22, X=23, Y=24, M/MT=25,
# other=1000). Shared awk fragment; callers set what to do with $0.
RANK = r'''
function rank(ch,  c){ c=ch; sub(/^chr/,"",c); sub(/^CHR/,"",c);
  if(c=="X")return 23; if(c=="Y")return 24; if(c=="M"||c=="MT")return 25;
  if(c ~ /^[0-9]+$/)return c+0; return 1000 }
'''

# RAW filter: keep rows whose SpliceAI max(DS_AG,DS_AL,DS_DG,DS_DL) >= THRESHOLD,
# prepend rank. SpliceAI INFO = ...;SpliceAI=ALLELE|SYM|DS_AG|DS_AL|DS_DG|DS_DL|...
AWK_RAW = RANK + r'''
/^#/ { next }
{
  n=split($8,parts,";"); spl="";
  for(i=1;i<=n;i++) if(substr(parts[i],1,9)=="SpliceAI="){ spl=substr(parts[i],10); break }
  if(spl=="") next
  ci=index(spl,","); if(ci>0) spl=substr(spl,1,ci-1)
  m=split(spl,b,"|"); if(m<6) next
  ds=b[3]+0; if(b[4]+0>ds)ds=b[4]+0; if(b[5]+0>ds)ds=b[5]+0; if(b[6]+0>ds)ds=b[6]+0
  if(ds < TH) next
  print rank($1) "\t" $0
}
'''

# MASKED filter: keep rows whose chr:pos:ref:alt is in the raw-kept key set.
AWK_MASKED = RANK + r'''
FNR==NR { keep[$0]=1; next }
/^#/ { next }
{ key=$1":"$2":"$4":"$5; if(key in keep) print rank($1) "\t" $0 }
'''


def sh(cmd, **kw):
    return subprocess.run(cmd, check=True, env=dict(os.environ, LC_ALL="C"), **kw)


def download(name):
    dst = os.path.join(OUT_DIR, name)
    print(f"  download/resume {name} ...", flush=True)
    sh(["curl", "-fL", "-C", "-", "-o", dst, f"{BASE}/{name}"])
    return dst


def pipe_sort_gz(gen_argv, gen_stdin, out_gz):
    """Run gen (awk pipeline) -> sort(rank,pos) -> cut -> gzip -> out_gz."""
    gen = subprocess.Popen(gen_argv, stdin=gen_stdin, stdout=subprocess.PIPE,
                           env=dict(os.environ, LC_ALL="C"))
    if gen_stdin is not None:
        gen_stdin.close()
    srt = subprocess.Popen(["sort", "-t", "\t", "-k1,1n", "-k3,3n", "-S", "4G", "-T", TMP_DIR],
                           stdin=gen.stdout, stdout=subprocess.PIPE, env=dict(os.environ, LC_ALL="C"))
    gen.stdout.close()
    cut = subprocess.Popen(["cut", "-f", "2-"], stdin=srt.stdout, stdout=subprocess.PIPE,
                           env=dict(os.environ, LC_ALL="C"))
    srt.stdout.close()
    with open(out_gz + ".part", "wb") as f:
        gz = subprocess.Popen(["gzip"], stdin=cut.stdout, stdout=f)
        cut.stdout.close()
        rc = gz.wait()
    for p in (gen, srt, cut):
        if p.wait() != 0:
            raise SystemExit(f"pipeline step failed: {p.args}")
    if rc != 0:
        raise SystemExit("gzip failed")
    os.rename(out_gz + ".part", out_gz)


def build_raw():
    if os.path.exists(RAW_SORTED) and os.path.exists(RAW_KEYS):
        print("  raw sorted + keys present, skip", flush=True)
        return
    src = download(DL["raw"])
    print(f"  filtering RAW (>= {THRESHOLD}) + co-sort ...", flush=True)
    zc = subprocess.Popen(["zcat", src], stdout=subprocess.PIPE, env=dict(os.environ, LC_ALL="C"))
    pipe_sort_gz(["awk", "-F", "\t", "-v", f"TH={THRESHOLD}", AWK_RAW], zc.stdout, RAW_SORTED)
    if zc.wait() != 0:
        raise SystemExit("zcat raw failed")
    print("  extracting raw-kept keys ...", flush=True)
    with open(RAW_KEYS + ".part", "w") as f:
        z = subprocess.Popen(["zcat", RAW_SORTED], stdout=subprocess.PIPE, env=dict(os.environ, LC_ALL="C"))
        a = subprocess.Popen(["awk", "-F", "\t", '!/^#/{print $1":"$2":"$4":"$5}'],
                             stdin=z.stdout, stdout=f, env=dict(os.environ, LC_ALL="C"))
        z.stdout.close(); a.wait(); z.wait()
    os.rename(RAW_KEYS + ".part", RAW_KEYS)
    n = sum(1 for _ in open(RAW_KEYS))
    print(f"  raw kept {n} variants -> {RAW_SORTED}", flush=True)


def build_masked():
    if os.path.exists(MASKED_SORTED):
        print("  masked sorted present, skip", flush=True)
        return
    src = download(DL["masked"])
    print("  filtering MASKED to raw-kept keys + co-sort ...", flush=True)
    # awk reads RAW_KEYS first (FNR==NR), then masked from zcat on stdin.
    zc = subprocess.Popen(["zcat", src], stdout=subprocess.PIPE, env=dict(os.environ, LC_ALL="C"))
    gen = subprocess.Popen(["awk", "-F", "\t", AWK_MASKED, RAW_KEYS, "-"],
                           stdin=zc.stdout, stdout=subprocess.PIPE, env=dict(os.environ, LC_ALL="C"))
    zc.stdout.close()
    srt = subprocess.Popen(["sort", "-t", "\t", "-k1,1n", "-k3,3n", "-S", "4G", "-T", TMP_DIR],
                           stdin=gen.stdout, stdout=subprocess.PIPE, env=dict(os.environ, LC_ALL="C"))
    gen.stdout.close()
    cut = subprocess.Popen(["cut", "-f", "2-"], stdin=srt.stdout, stdout=subprocess.PIPE,
                           env=dict(os.environ, LC_ALL="C"))
    srt.stdout.close()
    with open(MASKED_SORTED + ".part", "wb") as f:
        gz = subprocess.Popen(["gzip"], stdin=cut.stdout, stdout=f); cut.stdout.close(); rc = gz.wait()
    for p in (zc, gen, srt, cut):
        p.wait()
    if rc != 0:
        raise SystemExit("masked gzip failed")
    os.rename(MASKED_SORTED + ".part", MASKED_SORTED)
    print(f"  done -> {MASKED_SORTED}", flush=True)


def main():
    os.makedirs(OUT_DIR, exist_ok=True)
    os.makedirs(TMP_DIR, exist_ok=True)
    only = sys.argv[1] if len(sys.argv) > 1 else ""
    if only in ("", "raw"):
        build_raw()
    if only in ("", "masked"):
        build_masked()
    print("spliceai prepare: done", flush=True)


if __name__ == "__main__":
    sys.exit(main())
