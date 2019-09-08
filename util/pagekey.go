package util

import (
	"math"
	"strconv"
)

type Pagekey struct {
	seqCache     map[string]string
	keyOffsets   map[int]int
	maxCacheSize int
	base26Power2 int
	base26Power3 int
	base26Power4 int
	base26Power5 int
}

func (p *Pagekey) Init() {

	p.seqCache = map[string]string{}
	p.keyOffsets = map[int]int{}
	p.maxCacheSize = 50000
	p.base26Power2 = 676
	p.base26Power3 = 17576
	p.base26Power4 = 456976
	p.base26Power5 = 11881376

	p.keyOffsets[1] = 1
	p.keyOffsets[2] = 27
	p.keyOffsets[3] = p.base26Power2 + 27
	p.keyOffsets[4] = p.base26Power2 + p.base26Power3 + 27
	p.keyOffsets[5] = p.base26Power2 + p.base26Power3 + p.base26Power4 + 27

}

func (p *Pagekey) Key(n int, keyLen int) string {

	n = n + p.keyOffsets[keyLen]

	keyLengthStr := strconv.Itoa(keyLen)
	nstr := strconv.Itoa(n)
	if _, ok := p.seqCache[nstr+" "+keyLengthStr]; ok { // todo check again here
		return p.seqCache[nstr+" "+keyLengthStr]
	}

	// char[] buf = new char[(int) floor(log(25 * (n + 1)) / log(26))];
	buf := make([]rune, int(math.Floor(math.Log(float64(25*(n+1)))/math.Log(26))))

	for i := len(buf) - 1; i >= 0; i-- {
		n--
		buf[i] = rune('a' + n%26)
		n /= 26
	}

	if len(p.seqCache) < p.maxCacheSize {
		p.seqCache[nstr+" "+keyLengthStr] = string(buf)
	}

	return string(buf)
}

func (p *Pagekey) KeyLen(pageSize int) int {

	if pageSize <= 26 {
		return 1
	} else if pageSize <= p.base26Power2 {
		return 2
	} else if pageSize <= p.base26Power3 {
		return 3
	} else if pageSize <= p.base26Power4 {
		return 4
	} else if pageSize <= p.base26Power5 {
		return 5
	} else {
		panic("Page size too large to generate key")
	}

}
