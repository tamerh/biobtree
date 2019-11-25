package configs

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func (c *Conf) toLowerCaseAndNumbered(start int, datasetfile string) {

	var b strings.Builder
	b.WriteString("{")

	var ids []string

	for k := range c.Dataconf {
		ids = append(ids, k)
	}

	sort.Strings(ids)

	for _, k := range ids {

		v := c.Dataconf[k]

		//id := c.Dataconf[k]["id"]

		lowerID := strings.ToLower(k)

		b.WriteString("\"" + lowerID + "\":{")

		if _, ok := v["aliases"]; ok {
			fmt.Println(k, " dataset has aliases update manually")
		}

		if lowerID != k {
			b.WriteString("\"aliases\":\"" + k + "\",")
		}
		if len(c.Dataconf[k]["name"]) > 0 {
			b.WriteString("\"name\":\"" + c.Dataconf[k]["name"] + "\",")
		} else {
			b.WriteString("\"name\":\"" + k + "\",")
		}

		b.WriteString("\"id\":\"" + strconv.Itoa(start) + "\",")

		b.WriteString("\"url\":\"" + c.Dataconf[k]["url"] + "\"},")

		start++

	}
	s := b.String()
	s = s[:len(s)-1]
	s = s + "}"
	ioutil.WriteFile("conf/new"+datasetfile, []byte(s), 0700)

}

func (c *Conf) reNumber(start int, datasetfile string) {

	var b strings.Builder
	b.WriteString("{")

	var ids []string

	for k := range c.Dataconf {
		ids = append(ids, k)
	}

	sort.Strings(ids)

	for _, k := range ids {

		v := c.Dataconf[k]

		b.WriteString("\"" + k + "\":{")

		if _, ok := v["aliases"]; ok {
			b.WriteString("\"aliases\":\"" + v["aliases"] + "\",")
		}

		if len(v["name"]) > 0 {
			b.WriteString("\"name\":\"" + v["name"] + "\",")
		} else {
			b.WriteString("\"name\":\"" + k + "\",")
		}

		if len(v["hasFilter"]) > 0 {
			b.WriteString("\"hasFilter\":\"" + v["hasFilter"] + "\",")
		}

		b.WriteString("\"id\":\"" + strconv.Itoa(start) + "\",")

		b.WriteString("\"url\":\"" + v["url"] + "\"},")

		start++

	}
	s := b.String()
	s = s[:len(s)-1]
	s = s + "}"
	ioutil.WriteFile("conf/new"+datasetfile, []byte(s), 0700)

}

func (c *Conf) createReverseConf() {

	os.Remove("conf/reverseconf.json")

	var b strings.Builder
	b.WriteString("{")
	keymap := map[string]bool{}
	for k := range c.Dataconf {
		id := c.Dataconf[k]["id"]
		if _, ok := keymap[id]; !ok {
			b.WriteString("\"" + id + "\":{")

			if len(c.Dataconf[k]["name"]) > 0 {
				b.WriteString("\"name\":\"" + c.Dataconf[k]["name"] + "\",")
			} else {
				b.WriteString("\"name\":\"" + k + "\",")
			}

			b.WriteString("\"url\":\"" + c.Dataconf[k]["url"] + "\"},")
			keymap[id] = true
		}
	}
	s := b.String()
	s = s[:len(s)-1]
	s = s + "}"
	ioutil.WriteFile("conf/reverseconf.json", []byte(s), 0700)

}

func (c *Conf) CleanOutDirs(cleanCaches bool) {

	if cleanCaches {

		err := os.RemoveAll(filepath.FromSlash(c.Appconf["outDir"]))

		if err != nil {
			log.Fatal("Error cleaning the out dir check you have right permission")
		}
		err = os.Mkdir(filepath.FromSlash(c.Appconf["outDir"]), 0700)
		if err != nil {
			log.Fatal("Error creating dir", c.Appconf["outDir"], "check you have right permission ")
		}
		err = os.Mkdir(filepath.FromSlash(c.Appconf["indexDir"]), 0700)
		if err != nil {
			log.Fatal("Error creating dir", c.Appconf["indexDir"], "check you have right permission ")
		}
		err = os.Mkdir(filepath.FromSlash(c.Appconf["dbDir"]), 0700)
		if err != nil {
			log.Fatal("Error creating dir", c.Appconf["dbDir"], "check you have right permission ")
		}

		err = os.Mkdir(filepath.FromSlash(c.Appconf["idDir"]), 0700)
		if err != nil {
			log.Fatal("Error creating dir", c.Appconf["dbDir"], "check you have right permission ")
		}

	} else {

		err := os.RemoveAll(filepath.FromSlash(c.Appconf["dbDir"]))

		if err != nil {
			log.Fatal("Error cleaning the db dir check you have right permission")
		}

		err = os.Mkdir(filepath.FromSlash(c.Appconf["dbDir"]), 0700)
		if err != nil {
			log.Fatal("Error creating dir", c.Appconf["dbDir"], "check you have right permission ")
		}

		err = os.RemoveAll(filepath.FromSlash(c.Appconf["idDir"]))
		if err != nil {
			log.Fatal("Error cleaning the idir dir check you have right permission")
		}

		err = os.Mkdir(filepath.FromSlash(c.Appconf["idDir"]), 0700)
		if err != nil {
			log.Fatal("Error creating dir", c.Appconf["idDir"], "check you have right permission ")
		}

		c.CleanNonCacheFiles()

	}

}

func (c *Conf) CleanNonCacheFiles() {

	// delete files which are not cache
	files, err := ioutil.ReadDir(filepath.FromSlash(c.Appconf["indexDir"]))

	if err != nil {
		return
	}

	for _, f := range files {

		if !strings.Contains(f.Name(), "cache") {

			err := os.Remove(filepath.FromSlash(c.Appconf["indexDir"] + "/" + f.Name()))
			if err != nil {
				log.Fatal(err)
			}

		}

	}

}

func (c *Conf) CleanCacheFiles() {

	files, err := ioutil.ReadDir(filepath.FromSlash(c.Appconf["indexDir"]))

	if err != nil {
		return
	}

	for _, f := range files {

		if strings.Contains(f.Name(), "cache") {

			err := os.Remove(filepath.FromSlash(c.Appconf["indexDir"] + "/" + f.Name()))
			if err != nil {
				log.Fatal(err)
			}

		}

	}

}

func (c *Conf) HasCacheFiles() bool {

	files, err := ioutil.ReadDir(filepath.FromSlash(c.Appconf["indexDir"]))

	if err != nil {
		return false
	}

	for _, f := range files {

		if strings.Contains(f.Name(), "cache") {
			return true
		}

	}

	return false

}
