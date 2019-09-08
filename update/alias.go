package update

import (
	"biobtree/conf"
	"biobtree/db"
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	json "encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"

	"github.com/bmatsuo/lmdb-go/lmdb"
)

type idFileInfo struct {
	Path   string `json:"path"`
	Total  uint64 `json:"total"`
	Size   uint64 `json:"size"`
	Source string `json:"source"`
}

var idMeta = map[string]idFileInfo{}

type Alias struct {
	key                 string
	source              string
	dataChan            *chan string
	chunk               []string
	chunkIndex          int
	totalid             uint64
	size                uint64
	path                string
	collectionIndex     int
	collectionPath      string
	currentDirFileCount int64
	dirMaxFileLimit     int64
}

// this implemented for individual dataset to generate all their ids set as an alias but at the moment it is not active
// since more proper way is going through organism and mapped and filter it to the desired datasets
// but with this fetature user can still create aliases for its specific usecases.
func (a *Alias) newAlias(key string) error {

	var err error
	if _, ok := idMeta[key]; ok {
		err := fmt.Errorf("Id export with same name ->" + key + " already started. This one skipped check its uniqness")
		return err
	}

	if len(a.chunk) == 0 {

		var aliasMaxIDSize int64 = 2000000
		if _, ok := config.Appconf["aliasMaxIDSize"]; ok {
			aliasMaxIDSize, err = strconv.ParseInt(config.Appconf["aliasMaxIDSize"], 10, 64)
			if err != nil {
				return err
			}
		}
		a.chunk = make([]string, aliasMaxIDSize)

		var dirMaxFileLimit int64 = 3000
		if _, ok := config.Appconf["aliasMaxFileLimitInDirectory"]; ok {
			dirMaxFileLimit, err = strconv.ParseInt(config.Appconf["aliasMaxFileLimitInDirectory"], 10, 64)
			if err != nil {
				return err
			}
		}
		a.dirMaxFileLimit = dirMaxFileLimit

	}

	if a.chunkIndex > 0 { // close previous one
		a.close()
	}

	a.key = key

	idMeta[key] = idFileInfo{}
	return nil

}

func (a *Alias) addID(id string) {

	// check max id size

	a.chunk[a.chunkIndex] = id
	a.chunkIndex++
	a.size += uint64(len(id))
	a.totalid++

}

func (a *Alias) close() {

	if a.chunkIndex > 0 {
		if a.currentDirFileCount == a.dirMaxFileLimit {

			a.collectionIndex++
			a.currentDirFileCount = 0
			a.collectionPath = config.Appconf["idDir"] + "/" + a.source + "_collection" + strconv.Itoa(a.collectionIndex)
			err := os.Mkdir(filepath.FromSlash(a.collectionPath), 0700)

			if err != nil {
				panic("Folder could not created ->" + filepath.FromSlash(config.Appconf["idDir"]+"/"+filepath.FromSlash(a.collectionPath)))
			}

		}
		a.currentDirFileCount++

		var idgenFilePath string

		if a.collectionPath != "" {
			idgenFilePath = a.collectionPath + "/" + a.source + "_" + a.key + ".gz"
		} else {
			idgenFilePath = config.Appconf["idDir"] + "/" + a.source + "_" + a.key + ".gz"
		}

		f, err := os.OpenFile(filepath.FromSlash(idgenFilePath), os.O_RDWR|os.O_CREATE, 0700)
		if err != nil {
			panic(err)
		}

		gw, err := gzip.NewWriterLevel(f, gzip.BestSpeed)

		sort.Strings(a.chunk[:a.chunkIndex])

		for i := 0; i < a.chunkIndex; i++ {
			if i == 0 || a.chunk[i] != a.chunk[i-1] {
				gw.Write([]byte(a.chunk[i]))
				gw.Write([]byte(newline))
			} else {
				a.totalid--
			}
		}

		gw.Close()
		f.Close()

		a.chunkIndex = 0

		fileInfo := idMeta[a.key]
		fileInfo.Total = a.totalid
		fileInfo.Size = a.size
		fileInfo.Source = a.source
		fileInfo.Path = idgenFilePath
		idMeta[a.key] = fileInfo

		gw.Close()
	}

}

// Merge runs at generate phase to write all alias in files to lmdb
func (a *Alias) Merge(conf *conf.Conf) {

	config = conf

	files, err := ioutil.ReadDir(config.Appconf["idDir"])
	check(err)

	allAliasConf := map[string]interface{}{}

	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {

			f, err := ioutil.ReadFile(filepath.FromSlash(config.Appconf["idDir"] + "/" + f.Name()))
			if err != nil {
				fmt.Printf("Error: %v", err)
				os.Exit(1)
			}

			if err := json.Unmarshal(f, &allAliasConf); err != nil {
				panic(err)
			}

		}
	}

	var totalSize float64

	for _, v := range allAliasConf {
		a := v.(map[string]interface{})
		totalSize += a["size"].(float64)
	}

	//fmt.Println(int64(totalSize))

	err = os.RemoveAll(filepath.FromSlash(config.Appconf["aliasDbDir"]))

	if err != nil {
		log.Fatal("Error cleaning the out dir check you have right permission")
		panic(err)
	}
	err = os.Mkdir(filepath.FromSlash(config.Appconf["aliasDbDir"]), 0700)
	if err != nil {
		log.Fatal("Error creating dir", config.Appconf["aliasDbDir"], "check you have right permission ")
		panic(err)
	}

	db := db.DB{}
	l, dbi := db.OpenAliasDB(true, int64(totalSize), config.Appconf)

	for k, v := range allAliasConf {

		a := v.(map[string]interface{})
		path := a["path"].(string)
		total := int(a["total"].(float64))

		ids := make([]string, total)

		file, err := os.Open(filepath.FromSlash(path))
		gz, err := gzip.NewReader(file)
		if err != nil {
			err := fmt.Errorf("Getting alias file failed path ->" + path)
			panic(err)
		}

		scanner := bufio.NewScanner(gz)
		var ind int
		for scanner.Scan() {
			if ind == total {
				err := fmt.Errorf("Total id count and actual content does not match for path ->" + path)
				panic(err)
			}
			ids[ind] = scanner.Text()
			ind++
		}

		if err := scanner.Err(); err != nil {
			err := fmt.Errorf("Getting alias ids failed path ->" + path)
			panic(err)
		}

		if ind == 0 {
			err := fmt.Errorf("Empty alias content not allowed path ->" + path)
			panic(err)
		}
		err = l.Update(func(txn *lmdb.Txn) (err error) {
			i := 0
			for i = 0; i < 20; i++ {
				txn.Put(dbi, []byte(strconv.Itoa(i)), []byte("test"+strconv.Itoa(i)), lmdb.Create)
			}

			var aa = pbuf.Alias{}
			aa.Identifiers = ids

			data, err := proto.Marshal(&aa)
			if err != nil {
				panic(err)
			}

			txn.Put(dbi, []byte(k), data, lmdb.Create)

			return err
		})

		if err != nil {
			panic(err)
		}

	}

	aliasStats := make(map[string]interface{})
	aliasStats["datasize"] = totalSize
	aliasStats["aliasSize"] = len(allAliasConf)
	data, err := json.Marshal(aliasStats)
	if err != nil {
		fmt.Println("Error while writing merge metadata")
	}

	ioutil.WriteFile(filepath.FromSlash(config.Appconf["aliasDbDir"]+"/alias.meta.json"), data, 0770)

}
