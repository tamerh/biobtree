package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

type GFF3 struct {
	SeqRegion string
	Source    string
	Type      string
	Start     int
	End       int
	Score     float64
	Strand    byte
	Phase     int
	Attrs     map[string]string
}

func main() {

	f, err := os.Open("/Users/tgur/Downloads/Homo_sapiens.GRCh38.98.gff3")

	if err != nil {
		panic(err)
	}

	biotypes := map[string]bool{}
	types := map[string]bool{}
	sources := map[string]bool{}

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		l := scanner.Text()

		if l[0] == '#' {
			continue
		}

		fields := strings.Split(string(l), "\t")
		if len(fields) != 9 {
			// comment lines should have already been dealt with,
			// so this is a malformed record
			log.Fatalln("Malformed record: ", string(l))
		}

		gff := GFF3{}
		gff.SeqRegion = fields[0]
		gff.Source = fields[1]
		gff.Type = fields[2]
		gff.Start, _ = strconv.Atoi(fields[3])
		gff.End, _ = strconv.Atoi(fields[4])
		gff.Score, _ = strconv.ParseFloat(fields[5], 64)
		gff.Strand = fields[6][0] // one byte char: +, -, ., or ?
		gff.Phase, _ = strconv.Atoi(fields[7])
		gff.Attrs = map[string]string{}

		var eqIndex int
		attrs := fields[8]
		for i := strings.Index(attrs, ";"); i > 0; i = strings.Index(attrs, ";") {
			eqIndex = strings.Index(attrs[:i], "=")
			gff.Attrs[attrs[:i][:eqIndex]] = attrs[:i][eqIndex+1:]
			attrs = attrs[i+1:]
		}

		eqIndex = strings.Index(attrs, "=")
		gff.Attrs[attrs[:eqIndex]] = attrs[eqIndex+1:]

		if _, ok := types[gff.Type]; !ok {
			types[gff.Type] = true
		}

		if _, ok := sources[gff.Source]; !ok {
			sources[gff.Source] = true
		}

		if _, ok1 := gff.Attrs["biotype"]; ok1 {
			if _, ok := biotypes[gff.Attrs["biotype"]]; !ok {
				biotypes[gff.Attrs["biotype"]] = true
			}
		}

	}

	for k := range biotypes {
		fmt.Println(k)
	}
}
