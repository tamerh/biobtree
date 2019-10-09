package service

import (
	"biobtree/conf"
	"biobtree/query"
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/NYTimes/gziphandler"

	"biobtree/pbuf"

	"github.com/pquerna/ffjson/ffjson"
)

var config *conf.Conf

const spacestr = " "

type Web struct {
	service service
	metaRes []byte
}

func (web *Web) Start(c *conf.Conf) {

	config = c

	s := service{}
	s.init()

	web.service = s

	// start grpc
	rpc := biobtreegrpc{
		service: s,
	}
	rpc.Start()

	web.initMeta()

	//setup ws and static points
	searchGz := gziphandler.GzipHandler(http.HandlerFunc(web.search))
	searchEntryGz := gziphandler.GzipHandler(http.HandlerFunc(web.entry))
	mapFilterGz := gziphandler.GzipHandler(http.HandlerFunc(web.mapFilter))
	searchPageGz := gziphandler.GzipHandler(http.HandlerFunc(web.searchPage))
	searchFilterGz := gziphandler.GzipHandler(http.HandlerFunc(web.searchFilter))
	bulkSearchGz := gziphandler.GzipHandler(http.HandlerFunc(web.bulkSearch))
	metaGz := gziphandler.GzipHandler(http.HandlerFunc(web.meta))
	http.Handle("/ws/", searchGz)
	http.Handle("/ws/entry/", searchEntryGz)
	http.Handle("/ws/map/", mapFilterGz)
	http.Handle("/ws/page/", searchPageGz)
	http.Handle("/ws/filter/", searchFilterGz)
	http.Handle("/bulk/", bulkSearchGz)
	http.Handle("/ws/meta/", metaGz)
	fs := http.FileServer(http.Dir("website"))
	http.Handle("/ui/", http.StripPrefix("/ui/", fs))

	//start web server with rest endpoints and ui
	var port string
	if _, ok := config.Appconf["httpPort"]; ok {
		port = config.Appconf["httpPort"]
	} else {
		port = "8888"
	}

	url := "http://localhost:" + port + "/ui"
	switch runtime.GOOS {
	case "linux":
		exec.Command("xdg-open", url).Start()
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		exec.Command("open", url).Start()
		//	default:
		//	err = fmt.Errorf("unsupported platform")
	}
	log.Println("REST started at port->", port)
	uiURL := "localhost:" + port + "/ui"
	log.Println("Web interface url->", uiURL)
	fmt.Println("biobtree web is running...")
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

func (web *Web) initMeta() {

	var b strings.Builder
	b.WriteString("{")
	keymap := map[string]bool{}
	optionalFields := []string{"bacteriaUrl", "fungiUrl", "metazoaUrl", "plantsUrl", "protistsUrl"}
	for k := range config.Dataconf {
		if config.Dataconf[k]["_alias"] == "" { // not send the alias
			id := config.Dataconf[k]["id"]
			if _, ok := keymap[id]; !ok {
				b.WriteString(`"` + id + `":{`)

				if len(config.Dataconf[k]["name"]) > 0 {
					b.WriteString(`"name":"` + config.Dataconf[k]["name"] + `",`)
				} else {
					b.WriteString(`"name":"` + k + `",`)
				}

				if len(config.Dataconf[k]["linkdataset"]) > 0 {
					b.WriteString(`"linkdataset":"` + config.Dataconf[k]["linkdataset"] + `",`)
				}

				b.WriteString(`"id":"` + k + `",`)

				b.WriteString(`"url":"` + config.Dataconf[k]["url"] + `"`)

				for _, field := range optionalFields {
					if _, ok := config.Dataconf[k][field]; ok {
						b.WriteString(`,`)
						b.WriteString(`"` + field + `":"` + config.Dataconf[k][field] + `"`)
					}
				}
				b.WriteString(`},`)

				keymap[id] = true
			}
		}
	}
	s2 := b.String()
	s2 = s2[:len(s2)-1]
	s2 = s2 + "}"
	web.metaRes = []byte(s2)
}

func (web *Web) meta(w http.ResponseWriter, r *http.Request) {

	w.Header().Add("content-type", "application/json")
	w.Header().Add("Access-Control-Allow-Origin", "*")

	w.Write(web.metaRes)

}

func (web *Web) bulkSearch(w http.ResponseWriter, r *http.Request) {

	r.ParseMultipartForm(32 << 20)
	file, _, err := r.FormFile("file")
	if err != nil {
		fmt.Println(err)
		return
	}

	var results []pbuf.Result
	var buf strings.Builder
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {

		t := strings.ToUpper(strings.TrimSpace(scanner.Text()))
		tarr := []string{t}
		r1, err := web.service.search(tarr, 0, "", nil) // todo

		if err != nil {
			buf.WriteString("[")
			errStr := errString{Err: err.Error()}
			jb, _ := ffjson.Marshal(errStr)
			buf.WriteString(string(jb))
			buf.WriteString("]")
			w.Write([]byte(buf.String()))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		//TODO	for _, res := range r1 {
		results = append(results, *r1)
		//		}

	}

	buf.WriteString("[")
	for i, b := range results {
		//jb, _ := json.Marshal(b)
		jb, _ := ffjson.Marshal(b)
		buf.WriteString(string(jb))
		if len(results)-1 != i {
			buf.WriteString(",")
		}
	}
	buf.WriteString("]")

	result := []byte(buf.String())
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-length", strconv.Itoa(len(result)))
	w.Header().Set("Content-Disposition", "attachment; filename=test.txt")
	w.Write(result)
	//io.Copy(w, bytes.NewBufferString(buf.String()))

	return

}

func (web *Web) searchFilter(w http.ResponseWriter, r *http.Request) {

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
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ids, ok := r.URL.Query()["i"]

	if !ok || len(ids[0]) < 1 {
		err := fmt.Errorf("i param is missing")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	fids, ok := r.URL.Query()["f"]

	if !ok || len(fids[0]) < 1 {
		err := fmt.Errorf("f param is missing")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	fils := strings.TrimSuffix(fids[0], ",")

	var filters []uint32
	for _, filterstr := range strings.Split(fils, ",") {

		filtersrc, ok := config.DataconfIDStringToInt[filterstr]
		if !ok {
			err := fmt.Errorf("invalid s param")
			errStr := errString{Err: err.Error()}
			jb, _ := ffjson.Marshal(errStr)
			buf.WriteString(string(jb))
			w.Write([]byte(buf.String()))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		filters = append(filters, uint32(filtersrc))

	}

	src, ok := config.DataconfIDStringToInt[r.URL.Query()["s"][0]]
	if !ok {
		err := fmt.Errorf("invalid s param")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	pageInd := 0
	if len(r.URL.Query()["p"]) > 0 {
		pageInd, _ = strconv.Atoi(r.URL.Query()["p"][0])
	}

	filteredRes, err := web.service.filter(ids[0], src, filters, pageInd)

	if err != nil {
		buf.WriteString("[")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		buf.WriteString("]")
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	buf.WriteString("[")
	jb, _ := ffjson.Marshal(filteredRes)
	buf.WriteString(string(jb))
	buf.WriteString("]")
	w.Write([]byte(buf.String()))
	return

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
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ids, ok := r.URL.Query()["i"]

	if !ok || len(ids[0]) < 1 {
		err := fmt.Errorf("i param is missing")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	dataset, ok := r.URL.Query()["s"]

	if !ok || len(dataset[0]) < 1 {
		err := fmt.Errorf("s param is missing")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	src, ok := config.DataconfIDStringToInt[dataset[0]]
	if !ok {
		err := fmt.Errorf("invalid s param")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	r1, err := web.service.getLmdbResult2(strings.ToUpper(ids[0]), src)

	if err != nil {
		buf.WriteString("[")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		buf.WriteString("]")
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	buf.WriteString("[")
	//jb, _ := json.Marshal(r1)
	jb, _ := ffjson.Marshal(r1)
	buf.WriteString(string(jb))
	buf.WriteString("]")
	w.Write([]byte(buf.String()))
	return

}

func (web *Web) searchPage(w http.ResponseWriter, r *http.Request) {

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
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	id := r.URL.Query()["i"][0]

	src, ok := config.DataconfIDStringToInt[r.URL.Query()["s"][0]]
	if !ok {
		err := fmt.Errorf("invalid s param")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query()["p"][0])
	t, _ := strconv.Atoi(r.URL.Query()["t"][0])

	r1, err := web.service.page(id, int(src), page, t)

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

	buf.WriteString("[")
	//jb, _ := json.Marshal(r1)
	jb, _ := ffjson.Marshal(r1)
	buf.WriteString(string(jb))
	buf.WriteString("]")
	w.Write([]byte(buf.String()))
	return

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
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var ids []string

	qids, ok := r.URL.Query()["i"]
	if !ok || len(qids[0]) < 1 {
		err := fmt.Errorf("i param is missing")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
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
	} else {
		ids = strings.Split(qids[0], ",")
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

	var src uint32
	srcStr, ok := r.URL.Query()["s"]
	if ok && len(srcStr[0]) > 0 {

		src, ok = config.DataconfIDStringToInt[srcStr[0]]
		if !ok {
			err := fmt.Errorf("invalid s param")
			errStr := errString{Err: err.Error()}
			jb, _ := ffjson.Marshal(errStr)
			buf.WriteString(string(jb))
			w.Write([]byte(buf.String()))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

	}

	res, err := web.service.search(ids, src, page, filterq)

	if err != nil {
		buf.WriteString("[")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		buf.WriteString("]")
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	} else if len(res.Results) == 0 {
		err := fmt.Errorf("No result found")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.Write([]byte(buf.String()))
		return
	}

	jb, _ := ffjson.Marshal(res)
	buf.WriteString(string(jb))

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
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var ids []string

	qids, ok := r.URL.Query()["i"]
	if !ok || len(qids[0]) < 1 {
		err := fmt.Errorf("i param is missing")
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
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
	} else {
		ids = strings.Split(qids[0], ",")
	}

	mapfil, ok := r.URL.Query()["m"]
	if !ok || len(mapfil[0]) < 1 {
		buf.WriteString("[")
		buf.WriteString(`{"Err":"m parameter is required"}`)
		buf.WriteString("]")
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var res *pbuf.MapFilterResult

	var src uint32
	srcStr, ok := r.URL.Query()["s"]
	if ok && len(srcStr[0]) > 0 {

		src, ok = config.DataconfIDStringToInt[srcStr[0]]
		if !ok {
			err := fmt.Errorf("invalid s param")
			errStr := errString{Err: err.Error()}
			jb, _ := ffjson.Marshal(errStr)
			buf.WriteString(string(jb))
			w.Write([]byte(buf.String()))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

	}

	pages, ok := r.URL.Query()["p"]
	var page string
	if ok && len(pages[0]) > 0 {
		page = pages[0]
	}

	res, err = web.service.mapFilter(ids, src, mapfil[0], page)
	if err != nil {
		errStr := errString{Err: err.Error()}
		jb, _ := ffjson.Marshal(errStr)
		buf.WriteString(string(jb))
		w.Write([]byte(buf.String()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	//buf.WriteString("[")
	jb, _ := ffjson.Marshal(res)
	buf.WriteString(string(jb))
	//buf.WriteString("]")
	w.Write([]byte(buf.String()))

}

type errString struct {
	Err string
}
