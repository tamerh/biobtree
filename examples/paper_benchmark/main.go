package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"../../src/pbuf"
	"google.golang.org/grpc"
)

const newlinebyte = byte('\n')
const tab string = "\t"

// this variable needs to be set to generate sample data with genSampleData()
const indexdir string = ""

// change this for remote biobtree
const grpcEndpoint = "localhost:7777"

func main() {
	//genSampleData()
	sendRequests()
}

func sendRequests() {

	var conn *grpc.ClientConn
	conn, err := grpc.Dial(grpcEndpoint, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %s", err)
	}
	defer conn.Close()
	c := pbuf.NewBiobtreeServiceClient(conn)

	file, err := os.Open("sample100K.data")
	if err != nil {
		log.Fatal(err)
	}
	maxEmptyResult := 10
	emptyResultIndex := 0
	defer file.Close()

	// wait a bit for setup monitoring
	//time.Sleep(15 * time.Second)

	scanner := bufio.NewScanner(file)
	var totalEleapseTime int64
	for scanner.Scan() {

		start := time.Now().UnixNano()

		response, err := c.Get(context.Background(), &pbuf.BiobtreeGetRequest{Keywords: []string{scanner.Text()}})

		end := time.Now().UnixNano()

		if len(response.Results) == 0 && emptyResultIndex > maxEmptyResult {
			panic("Process stopped. Too many empty results for key->" + scanner.Text())
		} else if len(response.Results) == 0 {
			emptyResultIndex++
		}
		//fmt.Println(response)

		if err != nil {
			panic("Process stopped.")
		}

		totalEleapseTime = totalEleapseTime + (end - start)

	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
		panic("Process stopped.")
	}

	fmt.Println("Process completed. Total duration in nonoseconds -> ", totalEleapseTime)

}

/**
Generates sample data with reservoir sampling
*/
func genSampleData() {

	var samples []string
	sampleSize := 100000

	files, err := ioutil.ReadDir(indexdir)
	if err != nil {
		log.Fatal(err)
	}

	rand.Seed(time.Now().UnixNano())
	totalkey := 0
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".gz") {

			file, err := os.Open(filepath.FromSlash(indexdir + f.Name()))
			gz, err := gzip.NewReader(file)
			if err == io.EOF { //zero file
				continue
			}
			br := bufio.NewReaderSize(gz, 65536)

			for {
				line, err := br.ReadString(newlinebyte)

				if err == io.EOF {
					break
				}
				r := strings.Split(line, tab)

				// sampling algorithm
				if len(r) > 0 {
					if len(samples) < sampleSize {
						samples = append(samples, r[0])
					} else {
						random := rand.Intn(totalkey)
						if random < sampleSize {
							samples[random] = r[0]
						}
					}
				}
				totalkey++
			}
		}
	}
	var b strings.Builder
	index := 0
	for _, s := range samples {
		b.WriteString(s)
		if index != len(s)-1 {
			b.WriteString("\n")
		}
		index++
	}

	ioutil.WriteFile(filepath.FromSlash("sample100K.data"), []byte(b.String()), 0700)

}
