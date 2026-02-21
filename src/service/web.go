package service

import (
	"biobtree/configs"
	"biobtree/query"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" // Register pprof handlers
	"os/exec"
	"runtime"
	"strings"

	"github.com/NYTimes/gziphandler"
	"github.com/pquerna/ffjson/ffjson"
)

var config *configs.Conf

const spacestr = " "

// pageKeySep separates the root key from the dataset+page suffix in page keys.
// Page key format: rootKey + \x00 + datasetKey(2 chars) + pageIndex(variable)
// Example: "TOXIN\x00AAC" where "AA" is dataset, "C" is page index
// MUST match the format used in generate/mergeg.go during database creation.
const pageKeySep = "\x00"

type Web struct {
	service *Service
	metaRes []byte
}

func (web *Web) Start(c *configs.Conf, nowebpopup bool, prodMode bool) {

	config = c

	s := &Service{}
	s.init()

	web.service = s

	rpc := biobtreegrpc{
		service: s,
	}
	rpc.Start(prodMode)

	//setup rest ws
	web.metaRes = []byte(s.metajson())

	searchGz := gziphandler.GzipHandler(http.HandlerFunc(web.search))
	metaGz := gziphandler.GzipHandler(http.HandlerFunc(web.meta))
	searchEntryGz := gziphandler.GzipHandler(http.HandlerFunc(web.entry))
	mapFilterGz := gziphandler.GzipHandler(http.HandlerFunc(web.mapFilter))

	http.Handle("/ws/", searchGz)
	http.Handle("/ws/meta", metaGz)
	http.Handle("/ws/meta/", metaGz)
	http.Handle("/ws/entry/", searchEntryGz)
	http.Handle("/ws/map/", mapFilterGz)

	//web ui
	fs := http.FileServer(http.Dir("website"))
	http.Handle("/ui/", http.StripPrefix("/ui/", fs))

	// genomes
	if _, ok := config.Appconf["disableGenomes"]; !ok {
		fsgenomes := http.FileServer(http.Dir("ensembl"))
		http.Handle("/genomes/", http.StripPrefix("/genomes/", fsgenomes))
	}

	//start web server with rest endpoints and ui
	var port string
	if prodMode {
		// Production mode: use prodHttpPort config
		if _, ok := config.Appconf["prodHttpPort"]; ok {
			port = config.Appconf["prodHttpPort"]
		} else {
			log.Fatal("prodHttpPort must be configured in application.param.json when using --prod flag")
		}
	} else {
		// Normal mode: use httpPort config
		if _, ok := config.Appconf["httpPort"]; ok {
			port = config.Appconf["httpPort"]
		} else {
			log.Fatal("httpPort must be configured in application.param.json")
		}
	}

	if !nowebpopup {

		url := "http://localhost:" + port + "/ui"
		switch runtime.GOOS {
		case "linux":
			exec.Command("xdg-open", url).Start()
		case "windows":
			exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
		case "darwin":
			exec.Command("open", url).Start()
		}

	}

	uiURL := "localhost:" + port + "/ui"
	log.Println("Web UI url->", uiURL)
	log.Println("biobtree web is running...")
	log.Fatal(http.ListenAndServe(":"+port, nil))

}

func (web *Web) checkRequest(r *http.Request) error {

	switch r.Method {
	case "GET":
		return nil
	default:
		return fmt.Errorf("Only GET supported")
	}

}

func (web *Web) meta(w http.ResponseWriter, r *http.Request) {

	w.Header().Add("content-type", "application/json")
	w.Header().Add("Access-Control-Allow-Origin", "*")

	w.Write(web.metaRes)

}

func (web *Web) entry(w http.ResponseWriter, r *http.Request) {

	w.Header().Add("content-type", "application/json")
	w.Header().Add("Access-Control-Allow-Origin", "*")

	var buf strings.Builder

	err := web.checkRequest(r)

	if err != nil {
		buf.WriteString("[")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		buf.WriteString("]")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(buf.String()))
		return
	}

	ids, ok := r.URL.Query()["i"]

	if !ok || len(ids[0]) < 1 {
		err := fmt.Errorf("i param is missing")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(buf.String()))
		return
	}

	dataset, ok := r.URL.Query()["s"]

	if !ok || len(dataset[0]) < 1 {
		err := fmt.Errorf("s param is missing")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(buf.String()))
		return
	}

	src, ok := config.DataconfIDStringToInt[dataset[0]]
	if !ok {
		err := fmt.Errorf("invalid s param")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(buf.String()))
		return
	}

	// Always use lite mode - returns full attributes but compact xref counts
	// Each dataset's entries can be retrieved using map query, e.g.:
	//   /ws/map/?i=P04637&m=>>uniprot>>reactome  (get reactome xrefs)
	//   /ws/map/?i=P04637&m=>>uniprot>>go        (get GO xrefs)
	liteResult, err := web.service.entryLite(ids[0], src)
	if err != nil {
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(buf.String()))
		return
	}
	jb, _ := ffjson.Marshal(liteResult)
	w.Write(jb)
	return

	// Full mode commented out - can be re-enabled if needed
	// Full mode returns all xref entries which can be very large
	// r1, err := web.service.LookupByDataset(ids[0], src)
	//
	// if err != nil {
	// 	buf.WriteString("[")
	// 	errStr := errString{Err: err.Error()}
	// 	jb, _ := ffjson.Marshal(errStr)
	// 	buf.WriteString(string(jb))
	// 	buf.WriteString("]")
	// 	w.WriteHeader(http.StatusBadRequest)
	// 	w.Write([]byte(buf.String()))
	// 	return
	// }
	//
	// buf.WriteString("[")
	// jb, _ := ffjson.Marshal(r1)
	// buf.WriteString(string(jb))
	// buf.WriteString("]")
	// w.Write([]byte(buf.String()))
	// return

}

func (web *Web) search(w http.ResponseWriter, r *http.Request) {

	w.Header().Add("content-type", "application/json")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	var buf strings.Builder

	err := web.checkRequest(r)

	if err != nil {
		buf.WriteString("[")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		buf.WriteString("]")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(buf.String()))
		return
	}

	var ids []string

	qids, ok := r.URL.Query()["i"]
	if !ok || len(qids[0]) < 1 {
		err := fmt.Errorf("i param is missing")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(buf.String()))
		return
	}
	if strings.HasPrefix(qids[0], "alias:") {
		if len(qids[0][6:]) <= 0 {
			err := fmt.Errorf("alias value is missing")
			errStr := errString{Err: err.Error()}
			jb, _ := ffjson.Marshal(errStr)
			buf.WriteString(string(jb))
			w.Write([]byte(buf.String()))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ids, err = web.service.aliasIDs(qids[0][6:])
		if err != nil {
			errStr := errString{Err: err.Error()}
			jb, _ := ffjson.Marshal(errStr)
			buf.WriteString(string(jb))
			w.Write([]byte(buf.String()))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	} else {
		ids = strings.Split(qids[0], ",")
		for i := 0; i < len(ids); i++ {
			ids[i] = strings.TrimSpace(ids[i])
		}
	}

	pages, ok := r.URL.Query()["p"]
	var page string
	if ok && len(pages[0]) > 0 {
		page = pages[0]
	}

	var filterq *query.Query

	filter, ok := r.URL.Query()["f"]
	if ok && len(filter[0]) > 0 {
		filterq = &query.Query{}
		filterq.Filter = filter[0]
	}

	var datasetFilters []uint32
	srcStr, ok := r.URL.Query()["s"]
	if ok && len(srcStr[0]) > 0 {
		// Parse comma-separated datasets: s=uniprot,ensembl,hgnc
		for _, ds := range strings.Split(srcStr[0], ",") {
			ds = strings.TrimSpace(ds)
			if ds == "" {
				continue
			}
			if id, ok := config.DataconfIDStringToInt[ds]; ok {
				datasetFilters = append(datasetFilters, id)
			} else {
				err := fmt.Errorf("invalid dataset in s param: %s", ds)
				errStr := errString{Err: err.Error()}
				jb, _ := ffjson.Marshal(errStr)
				buf.WriteString(string(jb))
				w.Write([]byte(buf.String()))
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
	}

	mode := parseMode(r)
	_, url := r.URL.Query()["u"]

	// Get dataset filter string for query echo
	var datasetFilter string
	if len(srcStr) > 0 && len(srcStr[0]) > 0 {
		datasetFilter = srcStr[0]
	}

	if mode == "lite" {
		// Lite mode - LLM-friendly pipe-delimited format with names
		res, err := web.service.searchLite(ids, datasetFilters, page, datasetFilter)
		if err != nil {
			errStr := errString{Err: err.Error()}
			jb, _ := ffjson.Marshal(errStr)
			buf.WriteString(string(jb))
			w.Write([]byte(buf.String()))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		jb, _ := ffjson.Marshal(res)
		buf.WriteString(string(jb))
	} else {
		// Full mode - complete response with attributes
		detail := mode == "full"
		res, err := web.service.Search(ids, datasetFilters, page, filterq, detail, url)

		if err != nil {
			buf.WriteString("[")
			errStr := errString{Err: err.Error()}
			jb, _ := ffjson.Marshal(errStr)
			buf.WriteString(string(jb))
			buf.WriteString("]")
			w.Write([]byte(buf.String()))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Enrich with query echo and stats for full mode
		rawQuery := qids[0]
		if datasetFilter != "" {
			rawQuery += " s=" + datasetFilter
		}
		EnrichResultFull(res, ids, datasetFilter, rawQuery)

		jb, _ := ffjson.Marshal(res)
		buf.WriteString(string(jb))
	}

	/**
		buf.WriteString("[")
		for i, b := range res {
			//jb, _ := json.Marshal(*b)
			jb, _ := ffjson.Marshal(*b)
			buf.WriteString(string(jb))
			if len(res)-1 != i {
				buf.WriteString(",")
			}
		}
		buf.WriteString("]")
	***/

	w.Write([]byte(buf.String()))

}

func (web *Web) mapFilter(w http.ResponseWriter, r *http.Request) {

	w.Header().Add("content-type", "application/json")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	var buf strings.Builder
	var err error

	err = web.checkRequest(r)

	if err != nil {
		buf.WriteString("[")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		buf.WriteString("]")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(buf.String()))
		return
	}

	var ids []string

	qids, ok := r.URL.Query()["i"]
	if !ok || len(qids[0]) < 1 {
		err := fmt.Errorf("i param is missing")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(buf.String()))
		return
	}
	if strings.HasPrefix(qids[0], "alias:") {
		if len(qids[0][6:]) <= 0 {
			err := fmt.Errorf("alias value is missing")
			errStr := errString{Err: err.Error()}
			jb, _ := ffjson.Marshal(errStr)
			buf.WriteString(string(jb))
			w.Write([]byte(buf.String()))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ids, err = web.service.aliasIDs(qids[0][6:])
		if err != nil {
			errStr := errString{Err: err.Error()}
			jb, _ := ffjson.Marshal(errStr)
			buf.WriteString(string(jb))
			w.Write([]byte(buf.String()))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	} else {
		ids = strings.Split(qids[0], ",")
		for i := 0; i < len(ids); i++ {
			ids[i] = strings.TrimSpace(ids[i])
		}
	}

	mapfil, ok := r.URL.Query()["m"]
	if !ok || len(mapfil[0]) < 1 {
		buf.WriteString("[")
		buf.WriteString(`{"Err":"m parameter is required"}`)
		buf.WriteString("]")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(buf.String()))
		return
	}

	pages, ok := r.URL.Query()["p"]
	var page string
	if ok && len(pages[0]) > 0 {
		page = pages[0]
	}

	mode := parseMode(r)

	if mode == "lite" {
		// Lite mode - LLM-friendly grouped format with names
		res, err := web.service.MapFilterLite(ids, mapfil[0], page)
		if err != nil {
			errStr := errString{Err: err.Error()}
			jb, _ := ffjson.Marshal(errStr)
			buf.WriteString(string(jb))
			w.Write([]byte(buf.String()))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		jb, _ := ffjson.Marshal(res)
		buf.WriteString(string(jb))
	} else {
		// Full mode - complete response with attributes
		res, err := web.service.MapFilter(ids, mapfil[0], page)
		if err != nil {
			errStr := errString{Err: err.Error()}
			jb, _ := ffjson.Marshal(errStr)
			buf.WriteString(string(jb))
			w.Write([]byte(buf.String()))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Enrich with query echo and stats for full mode
		rawQuery := qids[0] + " m=" + mapfil[0]
		EnrichMapFilterResultFull(res, ids, mapfil[0], rawQuery)

		jb, _ := ffjson.Marshal(res)
		buf.WriteString(string(jb))
	}

	w.Write([]byte(buf.String()))

}

type errString struct {
	Err string
}

// parseMode extracts and validates the mode parameter from request
// Returns "lite", "full", or "compact". Default is "full" for backward compatibility.
func parseMode(r *http.Request) string {
	modes, ok := r.URL.Query()["mode"]
	if ok && len(modes[0]) > 0 {
		mode := strings.ToLower(modes[0])
		if mode == "lite" || mode == "full" {
			return mode
		}
	}
	// Backward compatibility: d=1 means full (detail mode)
	_, hasDetail := r.URL.Query()["d"]
	if hasDetail {
		return "full"
	}
	return "full" // Default to full for backward compatibility
}
