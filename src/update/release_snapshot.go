package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"biobtree/service"
)

// Release snapshots give every build a citable, immutable metadata record:
// out_prod/releases/<release_id>.json (never overwritten) plus current.json.
// The payload is the /ws/meta dataset view plus the federation versions at that
// moment, so a consumer can pin provenance without a running server.

type releaseSnapshot struct {
	ReleaseID       string            `json:"release_id"`
	CreatedAt       string            `json:"created_at"`
	BiobtreeVersion string            `json:"biobtree_version"`
	Federation      string            `json:"federation"`
	DBVersion       string            `json:"db_version"`
	Federations     map[string]string `json:"federations"`
	Meta            json.RawMessage   `json:"meta"`
}

type releasePointer struct {
	ReleaseID  string `json:"release_id"`
	Path       string `json:"path"`
	CreatedAt  string `json:"created_at"`
	Federation string `json:"federation"`
	DBVersion  string `json:"db_version"`
}

var releaseIDUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// WriteReleaseSnapshot records one finished build. It is called at the end of a
// successful generate; a failure here is logged and does not fail the build.
func WriteReleaseSnapshot(outDir, federation, dbVersion, binaryVersion string) error {
	if outDir == "" {
		return fmt.Errorf("no output directory")
	}
	if federation == "" {
		federation = "main"
	}
	versions := collectFederationVersions(outDir)
	if dbVersion != "" {
		versions[federation] = dbVersion
	}

	metaJSON := json.RawMessage(service.MetaJSONString(config.Dataconf, config.Appconf, func(datasetKey string) string {
		return versions[federationOf(datasetKey)]
	}))

	now := time.Now().UTC()
	id := fmt.Sprintf("%s-%s-%s", now.Format("20060102T150405Z"), federation, dbVersion)
	id = strings.Trim(releaseIDUnsafe.ReplaceAllString(id, "-"), "-")

	dir := filepath.Join(outDir, "releases")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating releases dir: %w", err)
	}

	snap := releaseSnapshot{
		ReleaseID:       id,
		CreatedAt:       now.Format(time.RFC3339),
		BiobtreeVersion: binaryVersion,
		Federation:      federation,
		DBVersion:       dbVersion,
		Federations:     versions,
		Meta:            metaJSON,
	}
	body, err := json.MarshalIndent(snap, "", " ")
	if err != nil {
		return fmt.Errorf("marshaling snapshot: %w", err)
	}

	// Never overwrite an existing snapshot: two builds in the same second get a
	// numeric suffix instead of clobbering the earlier record.
	path := filepath.Join(dir, id+".json")
	for i := 2; ; i++ {
		f, ferr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if ferr == nil {
			_, werr := f.Write(body)
			cerr := f.Close()
			if werr != nil {
				return werr
			}
			if cerr != nil {
				return cerr
			}
			break
		}
		if !os.IsExist(ferr) {
			return ferr
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%d.json", id, i))
	}

	ptr := releasePointer{
		ReleaseID:  strings.TrimSuffix(filepath.Base(path), ".json"),
		Path:       filepath.Join("releases", filepath.Base(path)),
		CreatedAt:  snap.CreatedAt,
		Federation: federation,
		DBVersion:  dbVersion,
	}
	ptrBody, err := json.MarshalIndent(ptr, "", " ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "current.json.tmp")
	if err := os.WriteFile(tmp, ptrBody, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "current.json"))
}

// collectFederationVersions maps federation -> db_vN by reading the <fed>/db
// symlink (falling back to whatever db_vN directory the link resolves to).
func collectFederationVersions(outDir string) map[string]string {
	versions := map[string]string{}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return versions
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		fed := e.Name()
		link := filepath.Join(outDir, fed, "db")
		target, lerr := os.Readlink(link)
		if lerr != nil {
			continue
		}
		base := filepath.Base(target)
		if base != "" && strings.HasPrefix(base, "db_v") {
			versions[fed] = base
		}
	}
	return versions
}

func federationOf(datasetKey string) string {
	if config == nil {
		return "main"
	}
	fed := config.Dataconf[datasetKey]["federation"]
	if fed == "" {
		return "main"
	}
	return fed
}

// FederationVersions is a small helper for callers that want the snapshot's
// version map without writing one (used by tests and the CLI).
func FederationVersions(outDir string) []string {
	v := collectFederationVersions(outDir)
	out := make([]string, 0, len(v))
	for fed, ver := range v {
		out = append(out, fed+"="+ver)
	}
	sort.Strings(out)
	return out
}
