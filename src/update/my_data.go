package update

import (
	"bufio"
	"log"
	"os"
	"strings"
)

// mydata processes simple text files with one identifier per line
// Used for custom datasets like biotech CID filters
type mydata struct {
	source string
	d      *DataUpdate
}

// update reads a text file with one identifier per line and indexes each ID
func (m *mydata) update() {
	defer m.d.wg.Done()

	filePath, ok := config.Dataconf[m.source]["path"]
	if !ok || filePath == "" {
		log.Printf("[%s] ERROR: Missing 'path' configuration", m.source)
		return
	}

	log.Printf("[%s] Reading identifiers from %s", m.source, filePath)

	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("[%s] ERROR: Cannot open file %s: %v", m.source, filePath, err)
		return
	}
	defer file.Close()

	// Get dataset ID
	datasetID := config.Dataconf[m.source]["id"]

	// Read file line by line
	scanner := bufio.NewScanner(file)
	count := 0
	skipped := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			skipped++
			continue
		}

		// Add identifier to database (minimal entry, just the ID)
		// Note: addProp3 requires len(attr) > 2, so we can't use empty "{}"
		// Using a minimal JSON object with a placeholder field
		attrJSON := []byte(`{"id":"` + line + `"}`)
		m.d.addProp3(line, datasetID, attrJSON)

		count++

		// Progress logging every 1M entries
		if count%1000000 == 0 {
			log.Printf("[%s] Indexed %dM identifiers...", m.source, count/1000000)
		}

		// Test mode: limit entries
		if config.IsTestMode() && shouldStopProcessing(config.GetTestLimit(m.source), count) {
			log.Printf("[%s] Test mode: Stopping after %d identifiers", m.source, count)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[%s] ERROR: Error reading file: %v", m.source, err)
	}

	log.Printf("[%s] Indexed %d identifiers (%d lines skipped)", m.source, count, skipped)

	// Signal completion
	m.d.progChan <- &progressInfo{dataset: m.source, done: true}
}
