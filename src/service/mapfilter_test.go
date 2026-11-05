package service

import "testing"

// A multi-input map page token encodes one segment per input. parsePagingKey
// must decode ALL of them; an early return previously kept only the first
// input, so paging a 10-gene query dropped 9 genes on page 2.
func TestParsePagingKeyMultiInput(t *testing.T) {
	s := &Service{}
	token := "-1[]HGNC:1,10,HGNC:1,166,5][HGNC:2,10,HGNC:2,51,3]["

	root, pages, err := s.parsePagingKey(token)
	if err != nil {
		t.Fatalf("parsePagingKey: %v", err)
	}
	if root != "" {
		t.Fatalf("root = %q, want empty", root)
	}
	if len(pages) != 2 {
		t.Fatalf("decoded %d inputs, want 2 (%v)", len(pages), pages)
	}

	p1 := pages["HGNC:1"][10][0]
	if p1.sourceID != "HGNC:1" || p1.entryIndex != 166 || p1.page != 5 {
		t.Fatalf("HGNC:1 page = %+v", p1)
	}
	p2 := pages["HGNC:2"][10][0]
	if p2.sourceID != "HGNC:2" || p2.entryIndex != 51 || p2.page != 3 {
		t.Fatalf("HGNC:2 page = %+v", p2)
	}
}

func TestParsePagingKeySingleInput(t *testing.T) {
	s := &Service{}
	root, pages, err := s.parsePagingKey("-1[]HGNC:11998,10,HGNC:11998,75,1][")
	if err != nil {
		t.Fatalf("parsePagingKey: %v", err)
	}
	if root != "" || len(pages) != 1 {
		t.Fatalf("root=%q pages=%v", root, pages)
	}
	p := pages["HGNC:11998"][10][0]
	if p.entryIndex != 75 || p.page != 1 {
		t.Fatalf("page = %+v", p)
	}
}

func TestParsePagingKeyRootOnly(t *testing.T) {
	s := &Service{}
	root, pages, err := s.parsePagingKey("12[]-1")
	if err != nil {
		t.Fatalf("parsePagingKey: %v", err)
	}
	if root != "12" || pages != nil {
		t.Fatalf("root=%q pages=%v", root, pages)
	}
}

func TestParsePagingKeyEmpty(t *testing.T) {
	s := &Service{}
	root, pages, err := s.parsePagingKey("")
	if err != nil || root != "" || pages != nil {
		t.Fatalf("root=%q pages=%v err=%v", root, pages, err)
	}
}

func TestParsePagingKeyMalformed(t *testing.T) {
	s := &Service{}
	if _, _, err := s.parsePagingKey("not-a-token"); err == nil {
		t.Fatal("expected an error for a token without []")
	}
}
