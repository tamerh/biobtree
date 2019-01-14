package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"./pbuf"
)

type web struct {
	service service
}

func (web *web) start() {

	s := service{}
	s.init()

	web.service = s

	// start grpc
	rpc := biobtreegrpc{
		service: s,
	}
	rpc.Start()

	//start web server with rest endpoints and ui
	http.HandleFunc("/ws/", web.search)
	http.HandleFunc("/ws/page/", web.searchPage)
	http.HandleFunc("/ws/filter/", web.searchFilter)
	http.HandleFunc("/bulk/", web.bulkSearch)
	http.HandleFunc("/ws/meta/", web.meta)
	fs := http.FileServer(http.Dir("webui"))
	http.Handle("/ui/", http.StripPrefix("/ui/", fs))

	var port string
	if _, ok := appconf["httpPort"]; ok {
		port = appconf["httpPort"]
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

func (web *web) meta(w http.ResponseWriter, r *http.Request) {

	w.Header().Add("content-type", "application/json")
	w.Header().Add("Access-Control-Allow-Origin", "*")

	var b strings.Builder
	b.WriteString("{")
	keymap := map[string]bool{}
	for k := range dataconf {
		id := dataconf[k]["id"]
		if _, ok := keymap[id]; !ok {
			b.WriteString("\"" + id + "\":{")

			if len(dataconf[k]["name"]) > 0 {
				b.WriteString("\"name\":\"" + dataconf[k]["name"] + "\",")
			} else {
				b.WriteString("\"name\":\"" + k + "\",")
			}

			b.WriteString("\"url\":\"" + dataconf[k]["url"] + "\"},")
			keymap[id] = true
		}
	}
	s := b.String()
	s = s[:len(s)-1]
	s = s + "}"

	w.Write([]byte(s))

}
func (web *web) bulkSearch(w http.ResponseWriter, r *http.Request) {

	r.ParseMultipartForm(32 << 20)
	file, _, err := r.FormFile("file")
	if err != nil {
		fmt.Println(err)
		return
	}

	var results []pbuf.Result

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {

		t := strings.ToUpper(strings.TrimSpace(scanner.Text()))
		tarr := []string{t}
		r1 := web.service.search(tarr)

		for _, res := range r1 {
			results = append(results, *res)
		}

	}
	var buf strings.Builder
	buf.WriteString("[")
	for i, b := range results {
		jb, _ := json.Marshal(b)
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
func (web *web) searchFilter(w http.ResponseWriter, r *http.Request) {

	w.Header().Add("content-type", "application/json")
	w.Header().Add("Access-Control-Allow-Origin", "*")

	id := r.URL.Query()["ids"][0]

	fids, ok := r.URL.Query()["filters"]

	if !ok || len(fids[0]) < 1 {
		log.Println("Url Param 'filters' is missing")
		return
	}

	fils := strings.TrimSuffix(fids[0], ",")

	var filters []uint32
	for _, filterstr := range strings.Split(fils, ",") {
		filterint, _ := strconv.Atoi(filterstr)
		filters = append(filters, uint32(filterint))
	}

	srcint, _ := strconv.Atoi(r.URL.Query()["src"][0])
	src := uint32(srcint)

	pageInd := 0
	if len(r.URL.Query()["last_filtered_page"]) > 0 {
		pageInd, _ = strconv.Atoi(r.URL.Query()["last_filtered_page"][0])
		//lastPage = r.URL.Query()["last_filtered_page"][0]
	}

	filteredRes := web.service.filter(id, src, filters, pageInd)

	var buf strings.Builder
	buf.WriteString("[")
	jb, _ := json.Marshal(filteredRes)
	buf.WriteString(string(jb))
	buf.WriteString("]")
	w.Write([]byte(buf.String()))
	return

}

func (web *web) searchPage(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query()["ids"][0]
	src, _ := strconv.Atoi(r.URL.Query()["src"][0])
	page, _ := strconv.Atoi(r.URL.Query()["page"][0])
	t, _ := strconv.Atoi(r.URL.Query()["total"][0])

	r1 := web.service.page(id, src, page, t)
	var buf strings.Builder
	buf.WriteString("[")
	jb, _ := json.Marshal(r1)
	buf.WriteString(string(jb))
	buf.WriteString("]")
	w.Header().Add("content-type", "application/json")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Write([]byte(buf.String()))
	return

}

func (web *web) search(w http.ResponseWriter, r *http.Request) {
	qids, ok := r.URL.Query()["ids"]
	if !ok || len(qids[0]) < 1 {
		//log.Println("Url Param 'ids' is missing")
		return
	}

	ids := strings.Split(qids[0], ",")

	res := web.service.search(ids)

	w.Header().Add("content-type", "application/json")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	var buf strings.Builder
	buf.WriteString("[")
	for i, b := range res {
		jb, _ := json.Marshal(*b)
		buf.WriteString(string(jb))
		if len(res)-1 != i {
			buf.WriteString(",")
		}
	}
	buf.WriteString("]")
	w.Write([]byte(buf.String()))

}
