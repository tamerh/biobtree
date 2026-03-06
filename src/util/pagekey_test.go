package util

import "testing"

func TestPageKey(t *testing.T) {

	p := &Pagekey{}
	p.Init()

	page := 25
	keyLen := p.KeyLen(page)
	first := p.Key(0, keyLen)
	last := p.Key(25, keyLen)

	if first != "a" {
		panic("invalid page key")
	}

	if last != "z" {
		panic("invalid page key")
	}

	page = 676
	keyLen = p.KeyLen(page)
	first = p.Key(0, keyLen)
	last = p.Key(25, keyLen)

	if first != "aa" {
		panic("invalid page key")
	}

	if last != "az" {
		panic("invalid page key")
	}

}
