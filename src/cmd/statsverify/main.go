// statsverify recomputes per-dataset node/edge counts from the sorted index
// files and compares them with dataset_state.json.
//
// The counts mirror what the generate phase keeps: duplicate xrefs are removed
// per key (the merge dedups by key + target dataset + value), and only the
// newest sorted file is used when a dataset has stale files from older chunk
// runs (those are flagged as multi_file).
//
// Usage (from the repo root):
//
//	go run ./cmd/statsverify -index out_prod/main/index -state out_prod/dataset_state.json -conf conf
//
// Optional:
//
//	-datasets hgnc,entrez,go   only these datasets
//	-workers 8
//
// Output is CSV on stdout; a summary goes to stderr.
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type buildInfo struct {
	ForwardEdges map[string]int64 `json:"forward_edges"`
	ReverseEdges map[string]int64 `json:"reverse_edges"`
	Properties   int64            `json:"properties"`
	XrefEdges    int64            `json:"xref_edges"`
	IndexEdges   int64            `json:"index_edges"`
	TotalEdges   int64            `json:"total_edges"`
}

type stateFile struct {
	Datasets map[string]buildInfo `json:"datasets"`
}

type result struct {
	dataset     string
	stateNodes  int64
	stateOut    int64
	stateIndex  int64
	actualNodes int64
	actualOut   int64
	actualIndex int64
	edgesIn     int64
	multiFile   bool
	hasFiles    bool
}

type fileCounts struct {
	nodes    int64
	edges    int64
	byTarget map[string]int64
}

func main() {
	indexFlag := flag.String("index", "out_prod/main/index", "comma-separated federation index directories")
	statePath := flag.String("state", "out_prod/dataset_state.json", "dataset_state.json")
	confDir := flag.String("conf", "conf", "configuration directory")
	datasetList := flag.String("datasets", "", "comma-separated dataset names (default: all in state)")
	workers := flag.Int("workers", runtime.NumCPU(), "parallel dataset workers")
	flag.Parse()

	state := stateFile{}
	data, err := os.ReadFile(*statePath)
	check(err, "reading state")
	check(json.Unmarshal(data, &state), "parsing state")

	idToName, err := loadDatasetIDs(*confDir)
	check(err, "loading dataset config")

	indexDirs := make([]string, 0, 4)
	for _, d := range strings.Split(*indexFlag, ",") {
		if d = strings.TrimSpace(d); d != "" {
			indexDirs = append(indexDirs, d)
		}
	}

	names := make([]string, 0, len(state.Datasets))
	if *datasetList != "" {
		for _, n := range strings.Split(*datasetList, ",") {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}
	} else {
		for n := range state.Datasets {
			names = append(names, n)
		}
	}
	sort.Strings(names)

	jobs := make(chan string)
	results := make(chan result, len(names))
	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for name := range jobs {
				results <- verifyDataset(indexDirs, name, idToName, state.Datasets[name])
			}
		}()
	}
	go func() {
		for _, n := range names {
			jobs <- n
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	rows := make([]result, 0, len(names))
	for r := range results {
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].dataset < rows[j].dataset })

	w := csv.NewWriter(os.Stdout)
	_ = w.Write([]string{
		"dataset", "multi_file",
		"nodes_state", "nodes_actual", "nodes_delta",
		"edges_out_state", "edges_out_actual", "edges_out_delta",
		"index_edges_state", "index_edges_actual", "index_edges_delta",
		"edges_in_actual",
	})
	checked, skipped, mismatches, multi := 0, 0, 0, 0
	for _, r := range rows {
		if !r.hasFiles {
			skipped++
			continue
		}
		checked++
		if r.multiFile {
			multi++
		}
		if r.actualNodes != r.stateNodes || r.actualOut != r.stateOut || r.actualIndex != r.stateIndex {
			mismatches++
		}
		mf := "no"
		if r.multiFile {
			mf = "yes"
		}
		_ = w.Write([]string{
			r.dataset, mf,
			strconv.FormatInt(r.stateNodes, 10), strconv.FormatInt(r.actualNodes, 10), strconv.FormatInt(r.actualNodes-r.stateNodes, 10),
			strconv.FormatInt(r.stateOut, 10), strconv.FormatInt(r.actualOut, 10), strconv.FormatInt(r.actualOut-r.stateOut, 10),
			strconv.FormatInt(r.stateIndex, 10), strconv.FormatInt(r.actualIndex, 10), strconv.FormatInt(r.actualIndex-r.stateIndex, 10),
			strconv.FormatInt(inboundAccumulator[r.dataset], 10),
		})
	}
	w.Flush()
	check(w.Error(), "writing csv")

	fmt.Fprintf(os.Stderr, "\n%d dataset(s) checked, %d with mismatches, %d multi-file (newest used), %d skipped (no index files)\n",
		checked, mismatches, multi, skipped)
}

var (
	inboundMu          sync.Mutex
	inboundAccumulator = map[string]int64{}
)

func verifyDataset(indexDirs []string, dataset string, idToName map[string]string, st buildInfo) result {
	r := result{
		dataset:    dataset,
		stateNodes: st.Properties,
		stateOut:   st.XrefEdges,
		stateIndex: st.IndexEdges,
	}
	if r.stateOut == 0 && st.TotalEdges > 0 {
		var props int64
		if v, ok := st.ForwardEdges["entry"]; ok {
			props = v
		}
		var fwd int64
		for _, v := range st.ForwardEdges {
			fwd += v
		}
		r.stateNodes = props
		r.stateOut = fwd - props
	}
	if r.stateIndex == 0 {
		if v, ok := st.ReverseEdges["textsearch"]; ok {
			r.stateIndex = v
		}
	}

	var forwardFiles, searchFiles []string
	for _, indexDir := range indexDirs {
		f, _ := filepath.Glob(filepath.Join(indexDir, dataset+"_sorted*.index.gz"))
		forwardFiles = append(forwardFiles, f...)
		s, _ := filepath.Glob(filepath.Join(indexDir, "textsearch_"+dataset+"_sorted*.index.gz"))
		searchFiles = append(searchFiles, s...)
	}
	if len(forwardFiles) == 0 {
		return r
	}
	r.hasFiles = true

	// Stale chunk runs can leave several files for one dataset. The newest is
	// the most recent complete output; using it avoids double counting.
	r.multiFile = len(forwardFiles) > 1
	fwdFile := newestFile(forwardFiles)
	fc := analyzeForwardFile(fwdFile)
	r.actualNodes = fc.nodes
	r.actualOut = fc.edges
	for target, n := range fc.byTarget {
		name := idToName[target]
		if name == "" {
			name = target
		}
		inboundMu.Lock()
		inboundAccumulator[name] += n
		inboundMu.Unlock()
	}

	if len(searchFiles) > 0 {
		r.actualIndex = analyzeSearchFile(newestFile(searchFiles))
	}
	return r
}

// analyzeForwardFile counts unique property keys (nodes) and unique xrefs
// (per key + target dataset + value), matching the merge's dedup rule.
func analyzeForwardFile(path string) fileCounts {
	fc := fileCounts{byTarget: make(map[string]int64)}
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %s: %v\n", path, err)
		return fc
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %s: %v\n", path, err)
		return fc
	}
	defer gz.Close()

	sc := bufio.NewScanner(gz)
	sc.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)

	curKey := ""
	seen := make(map[string]bool)
	hasProp := false
	flush := func() {
		if curKey != "" && hasProp {
			fc.nodes++
		}
	}
	for sc.Scan() {
		key, value, target, ok := fields(sc.Text())
		if !ok {
			continue
		}
		if key != curKey {
			flush()
			curKey = key
			seen = make(map[string]bool)
			hasProp = false
		}
		if target == "-1" {
			hasProp = true
			continue
		}
		dedup := target + "\x00" + value
		if seen[dedup] {
			continue
		}
		seen[dedup] = true
		fc.edges++
		fc.byTarget[target]++
	}
	flush()
	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %s: %v\n", path, err)
	}
	return fc
}

// analyzeSearchFile counts unique (key, value) rows, matching merge dedup.
func analyzeSearchFile(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0
	}
	defer gz.Close()

	sc := bufio.NewScanner(gz)
	sc.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)
	curKey := ""
	seen := make(map[string]bool)
	var n int64
	for sc.Scan() {
		key, value, _, ok := fields(sc.Text())
		if !ok {
			continue
		}
		if key != curKey {
			curKey = key
			seen = make(map[string]bool)
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		n++
	}
	return n
}

// fields returns key (field 0), value (field 2) and target dataset id
// (field 3) without copying attribute payloads.
func fields(line string) (key, value, target string, ok bool) {
	t1 := strings.IndexByte(line, '\t')
	if t1 < 0 {
		return
	}
	rest := line[t1+1:]
	t2 := strings.IndexByte(rest, '\t')
	if t2 < 0 {
		return
	}
	rest = rest[t2+1:]
	t3 := strings.IndexByte(rest, '\t')
	if t3 < 0 {
		return
	}
	value = rest[:t3]
	rest = rest[t3+1:]
	if t4 := strings.IndexByte(rest, '\t'); t4 >= 0 {
		target = rest[:t4]
	} else {
		target = rest
	}
	key = line[:t1]
	ok = true
	return
}

func newestFile(files []string) string {
	best := files[0]
	var bestTime int64
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		if t := st.ModTime().UnixNano(); t > bestTime {
			bestTime = t
			best = f
		}
	}
	return best
}

// loadDatasetIDs reads every *.dataset.json and returns dataset-id -> name.
func loadDatasetIDs(confDir string) (map[string]string, error) {
	files, err := filepath.Glob(filepath.Join(confDir, "*.dataset.json"))
	if err != nil {
		return nil, err
	}
	ids := make(map[string]string)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var ds map[string]map[string]interface{}
		if err := json.Unmarshal(data, &ds); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		for name, props := range ds {
			id, _ := props["id"].(string)
			if id != "" {
				ids[id] = name
			}
		}
	}
	return ids, nil
}

func check(err error, what string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error %s: %v\n", what, err)
		os.Exit(1)
	}
}
