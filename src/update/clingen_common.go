package update

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// openClingenReader opens a buffered reader for a ClinGen source path.
// ClinGen files are served over HTTPS (or a local file in test setups); there is
// no FTP path to handle. Returns the reader and a cleanup func to defer.
func openClingenReader(source, path string) (*bufio.Reader, func(), error) {
	if config.Dataconf[source]["useLocalFile"] == "yes" {
		f, err := os.Open(filepath.FromSlash(path))
		if err != nil {
			return nil, func() {}, err
		}
		return bufio.NewReaderSize(f, fileBufSize), func() { f.Close() }, nil
	}
	resp, err := http.Get(path)
	if err != nil {
		return nil, func() {}, err
	}
	return bufio.NewReaderSize(resp.Body, fileBufSize), func() { resp.Body.Close() }, nil
}

// clingenDiseaseXref routes a disease id field (which may contain one or more
// comma/semicolon-separated ids like "MONDO:0007858" or "OMIM:314580") to the
// matching biobtree disease dataset. Key formats follow the ClinVar parser:
// MONDO keeps its prefix; OMIM is reduced to a bare numeric MIM id; Orphanet is
// reduced to a bare OrphaCode.
func clingenDiseaseXref(d *DataUpdate, key, sourceID, diseaseField string) {
	if diseaseField == "" {
		return
	}
	for _, raw := range strings.FieldsFunc(diseaseField, func(r rune) bool { return r == ',' || r == ';' }) {
		id := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(id, "MONDO:"):
			d.addXref(key, sourceID, id, "mondo", false)
		case strings.HasPrefix(id, "OMIM:"):
			d.addXref(key, sourceID, strings.TrimPrefix(id, "OMIM:"), "mim", false)
		case strings.HasPrefix(id, "Orphanet:"):
			d.addXref(key, sourceID, strings.TrimPrefix(id, "Orphanet:"), "orphanet", false)
		case strings.HasPrefix(id, "ORPHA:"):
			d.addXref(key, sourceID, strings.TrimPrefix(id, "ORPHA:"), "orphanet", false)
		}
	}
}
