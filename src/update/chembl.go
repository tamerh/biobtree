package update

import (
	"biobtree/pbuf"
	"fmt"
	"io"
	"io/ioutil"
//	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pquerna/ffjson/ffjson"

	"github.com/tamerh/rdf"
)

type chembl struct {
	source        string
	d             *DataUpdate
	ftpHost       string
	ftpPath       string
	bindingSites  map[string]*pbuf.ChemblBindingSite
	biocomponents map[string]*biocomponent
	indications   map[string]*pbuf.ChemblIndication
	protclasses   map[string]*pbuf.ChemblProteinTargetClassification
	mechanisms    map[string]*pbuf.ChemblMechanism
	// progress related
	previous  int64
	totalRead int
}

// check provides context-aware error checking for chembl processor
func (c *chembl) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

type assaysource struct {
	name        string
	description string
}

type indication struct {
	efoName                 string
	meshHeading             string
	highestDevelopmentPhase int
	efo                     string
	mesh                    string
}

type biocomponent struct {
	typee       string
	description string
	sequence    string
	taxo        string
}

func (c *chembl) update() {

	// Get ChEMBL base path from molecule dataset (same for all ChEMBL datasets)
	// e.g., "ftp://ftp.ebi.ac.uk/pub/databases/chembl/ChEMBL-RDF/latest/"
	fullURL := config.Dataconf["chembl_molecule"]["path"]
	ftpHost, ftpPath, err := parseFTPURL(fullURL)
	if err != nil {
		panic("Invalid ChEMBL FTP path in config: " + err.Error())
	}
	c.ftpHost = ftpHost
	c.ftpPath = ftpPath

	c.updateBindingSites()
	c.updateBiocomponents()
	c.updateIndications()
	c.updateProteinTargetClasses()

	switch c.source {
	case "chembl_activity":
		c.updateActivity()
	case "chembl_molecule":
		c.updateMolecule()
	case "chembl_assay":
		c.updateAssay()
	case "chembl_document":
		c.updateDocument()
	case "chembl_target":
		c.updateTarget()
	case "chembl_target_component":
		c.updateTargetComponent()
	case "chembl_cell_line":
		c.updateCellline()
	default:
		panic("Unrecognized chembl dataset ->" + c.source)
	}
	c.d.progChan <- &progressInfo{dataset: c.source, done: true}
}

func (c *chembl) updateProteinTargetClasses() {

	// todo narrow,broader,subclass
	c.protclasses = map[string]*pbuf.ChemblProteinTargetClassification{}

	protclassFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathProteinTargetClassificationPattern"])
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, c.ftpHost, c.ftpPath, protclassFtpPath)
	c.check(err, "opening protein target classification file")

	dec := rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/protclass/") {
			id := c.getChemblID(triple.Subj.String())
			switch triple.Pred.String() {
			case "http://www.w3.org/2000/01/rdf-schema#label":
				if _, ok := c.protclasses[id]; ok {
					bio := c.protclasses[id]
					bio.ClassName = strings.ToLower(strings.Replace(triple.Obj.String(), " ", "_", -1))
					c.protclasses[id] = bio
				} else {
					bio := pbuf.ChemblProteinTargetClassification{}
					bio.ClassName = strings.ToLower(strings.Replace(triple.Obj.String(), " ", "_", -1))
					c.protclasses[id] = &bio
				}
			case "classLevel":
				if _, ok := c.protclasses[id]; ok {
					bio := c.protclasses[id]
					bio.ClassLevel = triple.Obj.String()
					c.protclasses[id] = bio
				} else {
					bio := pbuf.ChemblProteinTargetClassification{}
					bio.ClassLevel = triple.Obj.String()
					c.protclasses[id] = &bio
				}
			case "classPath":
				if _, ok := c.protclasses[id]; ok {
					bio := c.protclasses[id]
					bio.ClassPath = triple.Obj.String()
					c.protclasses[id] = bio
				} else {
					bio := pbuf.ChemblProteinTargetClassification{}
					bio.ClassPath = triple.Obj.String()
					c.protclasses[id] = &bio
				}
			}
		}

	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}

	if client != nil {
		client.Quit()
	}
	gz.Close()

}

func (c *chembl) updateBiocomponents() {

	c.biocomponents = map[string]*biocomponent{}

	biocomponentFtpPath := c.getFtpPath(config.Dataconf["chembl_molecule"]["pathBioComponentPattern"])
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, c.ftpHost, c.ftpPath, biocomponentFtpPath)
	c.check(err, "opening biocomponent file")

	dec := rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/biocomponent/") {
			id := c.getChemblID(triple.Subj.String())
			switch triple.Pred.String() {
			case "componentType":
				if _, ok := c.biocomponents[id]; ok {
					bio := c.biocomponents[id]
					bio.typee = triple.Obj.String()
					c.biocomponents[id] = bio
				} else {
					bio := biocomponent{}
					bio.typee = triple.Obj.String()
					c.biocomponents[id] = &bio
				}
			case "http://purl.org/dc/terms/description":
				if _, ok := c.biocomponents[id]; ok {
					bio := c.biocomponents[id]
					bio.description = triple.Obj.String()
					c.biocomponents[id] = bio
				} else {
					bio := biocomponent{}
					bio.description = triple.Obj.String()
					c.biocomponents[id] = &bio
				}
			case "proteinSequence":
				if _, ok := c.biocomponents[id]; ok {
					bio := c.biocomponents[id]
					bio.sequence = triple.Obj.String()
					c.biocomponents[id] = bio
				} else {
					bio := biocomponent{}
					bio.sequence = triple.Obj.String()
					c.biocomponents[id] = &bio
				}
			case "taxonomy":
				if _, ok := c.biocomponents[id]; ok {
					bio := c.biocomponents[id]
					bio.taxo = c.getTaxID(triple.Obj.String())
					c.biocomponents[id] = bio
				} else {
					bio := biocomponent{}
					bio.taxo = c.getTaxID(triple.Obj.String())
					c.biocomponents[id] = &bio
				}
			}
		}

	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}

	if client != nil {
		client.Quit()
	}
	gz.Close()

}

func (c *chembl) updateBindingSites() {

	c.bindingSites = map[string]*pbuf.ChemblBindingSite{}

	bindingSiteFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathBindingSitePattern"])
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, c.ftpHost, c.ftpPath, bindingSiteFtpPath)
	check(err)

	dec := rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/binding_site/") {
			switch triple.Pred.String() {
			case "bindingSiteName":
				id := c.getChemblID(triple.Subj.String())
				c.bindingSites[id] = &pbuf.ChemblBindingSite{Name: triple.Obj.String()}
			}
		}

	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}

	if client != nil {
		client.Quit()
	}
	gz.Close()

	// now mechanism which use binding sites
	c.updateMechanisms()

}

func (c *chembl) updateMechanisms() {

	c.mechanisms = map[string]*pbuf.ChemblMechanism{}

	//first for just mechanisms name and action type
	mechanismFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathMechanismPattern"])
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, c.ftpHost, c.ftpPath, mechanismFtpPath)
	c.check(err, "opening mechanism file")

	dec := rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/drug_mechanism/") {
			id := c.getChemblID(triple.Subj.String())
			switch triple.Pred.String() {
			case "mechanismDescription":
				if _, ok := c.mechanisms[id]; ok {
					mec := c.mechanisms[id]
					mec.Desc = triple.Obj.String()
					c.mechanisms[id] = mec
				} else {
					mec := pbuf.ChemblMechanism{}
					mec.Desc = triple.Obj.String()
					c.mechanisms[id] = &mec
				}
			case "mechanismActionType":
				if _, ok := c.mechanisms[id]; ok {
					mec := c.mechanisms[id]
					mec.Action = triple.Obj.String()
					c.mechanisms[id] = mec
				} else {
					mec := pbuf.ChemblMechanism{}
					mec.Action = triple.Obj.String()
					c.mechanisms[id] = &mec
				}
			}
		}
	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
	if client != nil {
		client.Quit()
	}
	gz.Close()

	// now set it for bindings , molecule and target
	br, gz, ftpFile, client, localFile, _, err = getDataReaderNew(c.source, c.ftpHost, c.ftpPath, mechanismFtpPath)
	check(err)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/drug_mechanism/") {
			id := c.getChemblID(triple.Subj.String())
			switch triple.Pred.String() {
			case "hasBindingSite":
				bindid := c.getChemblID(triple.Obj.String())
				if _, ok := c.bindingSites[bindid]; ok {
					mec := c.mechanisms[id]
					c.bindingSites[bindid].Mechanism = mec
				}
			case "hasMolecule":
				// id = mechanism ID from Subject (e.g., CHEMBL_MEC_1664)
				// molecule ID from Object (e.g., CHEMBL22)
				if _, ok := c.mechanisms[id]; ok {
					moleculeID := c.getChemblID(triple.Obj.String())
					attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Mechanism: c.mechanisms[id]}}
					b, _ := ffjson.Marshal(attr)
					c.d.addProp3(moleculeID, config.Dataconf["chembl_molecule"]["id"], b)
				}
			case "hasTarget":
				// id = mechanism ID from Subject (e.g., CHEMBL_MEC_1664)
				// target ID from Object (e.g., CHEMBL2364669)
				if _, ok := c.mechanisms[id]; ok {
					targetID := c.getChemblID(triple.Obj.String())
					attr := pbuf.ChemblAttr{Target: &pbuf.ChemblTarget{Mechanism: c.mechanisms[id]}}
					b, _ := ffjson.Marshal(attr)
					c.d.addProp3(targetID, config.Dataconf["chembl_target"]["id"], b)
				}
			}
		}
	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
	if client != nil {
		client.Quit()
	}
	gz.Close()

}

func (c *chembl) updateCellline() {

	// Test mode: get limit and open ID log file
	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// Track processed cell lines in test mode
	processedCellLines := make(map[string]bool)
	var cellLineCount int

	fr := config.Dataconf["chembl_cell_line"]["id"]
	celllineFtpPath := c.getFtpPath(config.Dataconf["chembl_cell_line"]["pathCelllinePattern"])
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, c.ftpHost, c.ftpPath, celllineFtpPath)
	check(err)

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	if client != nil {
		defer client.Quit()
	}

	defer gz.Close()
	defer c.d.wg.Done()

	dec := rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > c.previous+c.d.progInterval {
			kbytesPerSecond := int64(c.totalRead) / elapsed / 1024
			c.previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: kbytesPerSecond}
		}
		c.totalRead = c.totalRead + len(triple.Subj.String()) + len(triple.Obj.String()) + len(triple.Pred.String())

		if strings.HasPrefix(triple.Subj.String(), "/cell_line/") {
			// Test mode: stop early if we've already processed enough cell lines
			if config.IsTestMode() && shouldStopProcessing(testLimit, cellLineCount) {
				if ftpFile != nil {
					ftpFile.Close()
				}
				if localFile != nil {
					localFile.Close()
				}
				if client != nil {
					client.Quit()
				}
				gz.Close()
				return
			}

			id := c.getChemblID(triple.Subj.String())

			// Test mode: track cell line ID on FIRST appearance (any predicate)
			// Only count actual cell line IDs, not blank nodes (which contain "#")
			if idLogFile != nil && !processedCellLines[id] && !strings.Contains(id, "#") {
				logProcessedID(idLogFile, id)
				processedCellLines[id] = true
				cellLineCount++
			}

			switch triple.Pred.String() {
			case "http://purl.org/dc/terms/description":
				attr := pbuf.ChemblAttr{CellLine: &pbuf.ChemblCellLine{Desc: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "cellosaurusId":
				attr := pbuf.ChemblAttr{CellLine: &pbuf.ChemblCellLine{CellosaurusId: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "taxonomy":
				taxid := c.getTaxID(triple.Obj.String())
				attr := pbuf.ChemblAttr{CellLine: &pbuf.ChemblCellLine{Tax: taxid}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
				c.d.addXref(id, fr, taxid, "taxonomy", false)
			case "hasEFO":
				efoid := c.extractOntologyID(triple.Obj.String())
				if efoid != "" {
					attr := pbuf.ChemblAttr{CellLine: &pbuf.ChemblCellLine{Efo: efoid}}
					b, _ := ffjson.Marshal(attr)
					c.d.addProp3(id, fr, b)
					c.d.addXref(id, fr, efoid, "efo", false)
				}
			case "hasCLO":
				cloid := c.getChemblID(triple.Obj.String())
				attr := pbuf.ChemblAttr{CellLine: &pbuf.ChemblCellLine{Clo: cloid}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "cellXref":
				attr := pbuf.ChemblAttr{CellLine: &pbuf.ChemblCellLine{Cellxref: []string{triple.Obj.String()}}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			}
		}
	}
}

func (c *chembl) updateTarget() {

	defer c.d.wg.Done()

	// Test mode: get limit and open ID log file
	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// Track processed targets in test mode
	processedTargets := make(map[string]bool)
	var targetCount int

	// binding site
	fr := config.Dataconf["chembl_target"]["id"]
	bindingSiteFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathBindingSitePattern"])
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, c.ftpHost, c.ftpPath, bindingSiteFtpPath)
	check(err)

	dec := rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/binding_site/") {
			switch triple.Pred.String() {
			case "hasTarget":

				id := c.getChemblID(triple.Subj.String())
				targetID := c.getChemblID(triple.Obj.String())
				if _, ok := c.bindingSites[id]; ok {
					attr := pbuf.ChemblAttr{Target: &pbuf.ChemblTarget{BindingSite: c.bindingSites[id]}}
					b, _ := ffjson.Marshal(attr)
					c.d.addProp3(targetID, fr, b)
				}

			}
		}

	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
	if client != nil {
		client.Quit()
	}
	gz.Close()

	// protein target class
	protclassFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathProteinTargetClassificationPattern"])
	br, gz, ftpFile, client, localFile, _, err = getDataReaderNew(c.source, c.ftpHost, c.ftpPath, protclassFtpPath)
	check(err)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/target/") {
			id := c.getChemblID(triple.Subj.String())
			switch triple.Pred.String() {
			case "hasProteinClassification":
				protclassid := c.getChemblID(triple.Obj.String())
				if _, ok := c.protclasses[protclassid]; ok {

					attr := pbuf.ChemblAttr{Target: &pbuf.ChemblTarget{Ptclassifications: []*pbuf.ChemblProteinTargetClassification{c.protclasses[protclassid]}}}
					b, _ := ffjson.Marshal(attr)
					c.d.addProp3(id, fr, b)
					c.d.addXref(c.protclasses[protclassid].ClassName, textLinkID, id, "chembl_target", true)
				}
			}
		}
	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
	if client != nil {
		client.Quit()
	}
	gz.Close()

	// target
	targetFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathTargetPattern"])
	br, gz, ftpFile, client, localFile, _, err = getDataReaderNew(c.source, c.ftpHost, c.ftpPath, targetFtpPath)
	check(err)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		// Test mode: stop early if we've already processed enough targets
		if config.IsTestMode() && shouldStopProcessing(testLimit, targetCount) {
			if ftpFile != nil {
				ftpFile.Close()
			}
			if localFile != nil {
				localFile.Close()
			}
			if client != nil {
				client.Quit()
			}
			gz.Close()
			return
		}

		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > c.previous+c.d.progInterval {
			kbytesPerSecond := int64(c.totalRead) / elapsed / 1024
			c.previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: kbytesPerSecond}
		}
		c.totalRead = c.totalRead + len(triple.Subj.String()) + len(triple.Obj.String()) + len(triple.Pred.String())

		if strings.HasPrefix(triple.Subj.String(), "/target/") {
			// Test mode: track target ID on FIRST appearance (any predicate)
			// Only count actual target IDs, not blank nodes (which contain "#")
			id := c.getChemblID(triple.Subj.String())
			if idLogFile != nil && !processedTargets[id] && !strings.Contains(id, "#") {
				logProcessedID(idLogFile, id)
				processedTargets[id] = true
				targetCount++
			}

			switch triple.Pred.String() {
			case "http://purl.org/dc/terms/title":
				attr := pbuf.ChemblAttr{Target: &pbuf.ChemblTarget{Title: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "targetType":
				typee := strings.ToLower(strings.Replace(triple.Obj.String(), " ", "_", -1))
				attr := pbuf.ChemblAttr{Target: &pbuf.ChemblTarget{Type: typee}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "isSpeciesGroup":
				attr := pbuf.ChemblAttr{Target: &pbuf.ChemblTarget{IsSpeciesGroup: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "taxonomy":
				taxid := c.getTaxID(triple.Obj.String())
				attr := pbuf.ChemblAttr{Target: &pbuf.ChemblTarget{Tax: taxid}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
				c.d.addXref(id, fr, taxid, "taxonomy", false)
			case "hasTargetComponent":
				tcmptid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, tcmptid, "chembl_target_component", false)
			case "isTargetForCellLine":
				cellid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, cellid, "chembl_cell_line", false)
			}
		}
	}
	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
	if client != nil {
		client.Quit()
	}
	gz.Close()

	// target relations
	targetRelFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathTargetRelPattern"])
	br, gz, ftpFile, client, localFile, _, err = getDataReaderNew(c.source, c.ftpHost, c.ftpPath, targetRelFtpPath)
	check(err)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/target/") {
			switch triple.Pred.String() {
			case "relSubsetOf":
				id := c.getChemblID(triple.Subj.String())
				relid := c.getChemblID(triple.Obj.String())
				attr := pbuf.ChemblAttr{Target: &pbuf.ChemblTarget{Subsetofs: []string{relid}}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "relEquivalentTo":
				id := c.getChemblID(triple.Subj.String())
				relid := c.getChemblID(triple.Obj.String())
				attr := pbuf.ChemblAttr{Target: &pbuf.ChemblTarget{Equivalents: []string{relid}}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "relHasSubset":
				id := c.getChemblID(triple.Subj.String())
				relid := c.getChemblID(triple.Obj.String())
				attr := pbuf.ChemblAttr{Target: &pbuf.ChemblTarget{Subsets: []string{relid}}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "relOverlapsWith":
				id := c.getChemblID(triple.Subj.String())
				relid := c.getChemblID(triple.Obj.String())
				attr := pbuf.ChemblAttr{Target: &pbuf.ChemblTarget{Overlaps: []string{relid}}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			}
		}

	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
	if client != nil {
		client.Quit()
	}
	gz.Close()

	// single target -> target_component
	singleTargetComptMappingFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathSingleTargetComponentMappingPattern"])
	br, gz, ftpFile, client, localFile, _, err = getDataReaderNew(c.source, c.ftpHost, c.ftpPath, singleTargetComptMappingFtpPath)
	check(err)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/target/") {
			id := c.getChemblID(triple.Subj.String())
			switch triple.Pred.String() {
			case "http://www.w3.org/2004/02/skos/core#exactMatch":
				targetcomptid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, targetcomptid, "chembl_target_component", false)

				attr := pbuf.ChemblAttr{Target: &pbuf.ChemblTarget{Components: []*pbuf.ChemblTargetComponentInfo{&pbuf.ChemblTargetComponentInfo{Acc: targetcomptid, Type: "single"}}}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)

			}
		}
	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
	if client != nil {
		client.Quit()
	}
	gz.Close()

	// complex target -> target_component
	complexTargetComptMappingFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathComplexTargetComponentMappingPattern"])
	br, gz, ftpFile, client, localFile, _, err = getDataReaderNew(c.source, c.ftpHost, c.ftpPath, complexTargetComptMappingFtpPath)
	check(err)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/target/") {
			id := c.getChemblID(triple.Subj.String())
			switch triple.Pred.String() {
			case "http://www.w3.org/2004/02/skos/core#relatedMatch":
				targetcomptid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, targetcomptid, "chembl_target_component", false)

				attr := pbuf.ChemblAttr{Target: &pbuf.ChemblTarget{Components: []*pbuf.ChemblTargetComponentInfo{&pbuf.ChemblTargetComponentInfo{Acc: targetcomptid, Type: "complex"}}}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)

			}
		}
	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
	if client != nil {
		client.Quit()
	}
	gz.Close()

	// group target -> target_component
	groupTargetComptMappingFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathGroupTargetComponentMappingPattern"])
	br, gz, ftpFile, client, localFile, _, err = getDataReaderNew(c.source, c.ftpHost, c.ftpPath, groupTargetComptMappingFtpPath)
	check(err)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/target/") {
			id := c.getChemblID(triple.Subj.String())
			switch triple.Pred.String() {
			case "http://www.w3.org/2004/02/skos/core#relatedMatch":
				targetcomptid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, targetcomptid, "chembl_target_component", false)

				attr := pbuf.ChemblAttr{Target: &pbuf.ChemblTarget{Components: []*pbuf.ChemblTargetComponentInfo{&pbuf.ChemblTargetComponentInfo{Acc: targetcomptid, Type: "group"}}}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)

			}
		}
	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
	if client != nil {
		client.Quit()
	}
	gz.Close()

}

func (c *chembl) updateTargetComponent() {

	// Test mode: get limit and open ID log file
	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// Track processed target components in test mode
	processedComponents := make(map[string]bool)
	var componentCount int

	fr := config.Dataconf["chembl_target_component"]["id"]
	targetComptFtpPath := c.getFtpPath(config.Dataconf["chembl_target_component"]["pathTargetComponentPattern"])
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, c.ftpHost, c.ftpPath, targetComptFtpPath)
	check(err)

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}

	if client != nil {
		defer client.Quit()
	}

	defer gz.Close()
	defer c.d.wg.Done()

	dec := rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {
		if strings.HasPrefix(triple.Subj.String(), "/targetcomponent/") {
			id := c.getChemblID(triple.Subj.String())
			switch triple.Pred.String() {
			case "http://purl.org/dc/terms/description":
				attr := pbuf.ChemblAttr{TargetComponent: &pbuf.ChemblTargetComponent{Desc: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "componentType":
				attr := pbuf.ChemblAttr{TargetComponent: &pbuf.ChemblTargetComponent{Type: strings.ToLower(triple.Obj.String())}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "altLabel":
				attr := pbuf.ChemblAttr{TargetComponent: &pbuf.ChemblTargetComponent{AltLabel: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "taxonomy":
				taxid := c.getTaxID(triple.Obj.String())
				attr := pbuf.ChemblAttr{TargetComponent: &pbuf.ChemblTargetComponent{Tax: taxid}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
				c.d.addXref(id, fr, taxid, "taxonomy", false)
				// not need case "targetCmptXref":
			}
		}
	}

	// protein target class
	protclassFtpPath := c.getFtpPath(config.Dataconf["chembl_target_component"]["pathProteinTargetClassificationPattern"])
	br, gz, ftpFile, client, localFile, _, err = getDataReaderNew(c.source, c.ftpHost, c.ftpPath, protclassFtpPath)
	check(err)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/targetcomponent/") {
			id := c.getChemblID(triple.Subj.String())
			switch triple.Pred.String() {
			case "hasProteinClassification":
				protclassid := c.getChemblID(triple.Obj.String())
				if _, ok := c.protclasses[protclassid]; ok {

					attr := pbuf.ChemblAttr{TargetComponent: &pbuf.ChemblTargetComponent{Ptclassifications: []*pbuf.ChemblProteinTargetClassification{c.protclasses[protclassid]}}}
					b, _ := ffjson.Marshal(attr)
					c.d.addProp3(id, fr, b)
				}
			}
		}
	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}

	if client != nil {
		client.Quit()
	}
	gz.Close()

	// component xref mapping this is essentially allows gene,protein ids to map the target_component and target
	uniprotMappingFtpPath := c.getFtpPath(config.Dataconf["chembl_target_component"]["pathUniprotMappingPattern"])
	br, gz, ftpFile, client, localFile, _, err = getDataReaderNew(c.source, c.ftpHost, c.ftpPath, uniprotMappingFtpPath)
	check(err)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF && triple.Subj != nil; triple, err = dec.Decode() {

		// Test mode: stop early if we've already processed enough target components
		if config.IsTestMode() && shouldStopProcessing(testLimit, componentCount) {
			if ftpFile != nil {
				ftpFile.Close()
			}
			if localFile != nil {
				localFile.Close()
			}
			if client != nil {
				client.Quit()
			}
			gz.Close()
			return
		}

		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > c.previous+c.d.progInterval {
			kbytesPerSecond := int64(c.totalRead) / elapsed / 1024
			c.previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: kbytesPerSecond}
		}

		c.totalRead = c.totalRead + len(triple.Subj.String()) + len(triple.Obj.String()) + len(triple.Pred.String())

		if strings.HasPrefix(triple.Subj.String(), "/targetcomponent/") {
			// Test mode: track target component ID on FIRST appearance (any predicate)
			// Only count actual component IDs, not blank nodes (which contain "#")
			id := c.getChemblID(triple.Subj.String())
			if idLogFile != nil && !processedComponents[id] && !strings.Contains(id, "#") {
				logProcessedID(idLogFile, id)
				processedComponents[id] = true
				componentCount++
			}

			switch triple.Pred.String() {
			case "http://www.w3.org/2004/02/skos/core#exactMatch":
				xrefid := c.getChemblID(triple.Obj.String())
				if strings.HasPrefix(xrefid, "ENSG") { // rna type target_component
					c.d.addXref(id, fr, xrefid, "ensembl", false)
				} else if strings.HasPrefix(xrefid, "ENST") { // rna type target_component
					c.d.addXref(id, fr, xrefid, "transcript", false)
				} else {
					c.d.addXref(id, fr, xrefid, "uniprot", false)
				}
				attr := pbuf.ChemblAttr{TargetComponent: &pbuf.ChemblTargetComponent{Acc: xrefid}}
				b, _ := ffjson.Marshal(attr)

				c.d.addProp3(id, fr, b)
			}
		}
	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
	if client != nil {
		client.Quit()
	}
	gz.Close()

}

func (c *chembl) updateIndications() {

	c.indications = map[string]*pbuf.ChemblIndication{}

	indicationFtpPath := c.getFtpPath(config.Dataconf["chembl_molecule"]["pathIndicationPattern"])
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, c.ftpHost, c.ftpPath, indicationFtpPath)
	check(err)

	dec := rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/drug_indication/") {
			switch triple.Pred.String() {
			case "hasEFOName":
				id := c.getChemblID(triple.Subj.String())
				if _, ok := c.indications[id]; ok {
					ind := c.indications[id]
					ind.EfoName = triple.Obj.String()
					c.indications[id] = ind
				} else {
					ind := pbuf.ChemblIndication{}
					ind.EfoName = triple.Obj.String()
					c.indications[id] = &ind
				}

			case "hasMeshHeading":
				id := c.getChemblID(triple.Subj.String())
				if _, ok := c.indications[id]; ok {
					ind := c.indications[id]
					ind.MeshHeading = triple.Obj.String()
					c.indications[id] = ind
				} else {
					ind := pbuf.ChemblIndication{}
					ind.MeshHeading = triple.Obj.String()
					c.indications[id] = &ind
				}
			case "highestDevelopmentPhase":

				id := c.getChemblID(triple.Subj.String())
				ccFloat, err := strconv.ParseFloat(triple.Obj.String(), 64)
				check(err)
			cc := int32(ccFloat)
				if _, ok := c.indications[id]; ok {
					ind := c.indications[id]
					ind.HighestDevelopmentPhase = cc
					c.indications[id] = ind
				} else {
					ind := pbuf.ChemblIndication{}
					ind.HighestDevelopmentPhase = cc
					c.indications[id] = &ind
				}

			case "hasMolecule":
			case "hasEFO":
				id := c.getChemblID(triple.Subj.String())
				efoid := c.extractOntologyID(triple.Obj.String())
				if efoid != "" {
					if _, ok := c.indications[id]; ok {
						ind := c.indications[id]
						ind.Efo = efoid
						c.indications[id] = ind
					} else {
						ind := pbuf.ChemblIndication{}
						ind.Efo = efoid
						c.indications[id] = &ind
					}
				}
			case "hasMesh":
				id := c.getChemblID(triple.Subj.String())
				meshid := c.getChemblID(triple.Obj.String())
				if _, ok := c.indications[id]; ok {
					ind := c.indications[id]
					ind.Mesh = meshid
					c.indications[id] = ind
				} else {
					ind := pbuf.ChemblIndication{}
					ind.Mesh = meshid
					c.indications[id] = &ind
				}
			}
		}

	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
	if client != nil {
		client.Quit()
	}
	gz.Close()

}

func (c *chembl) updateMolecule() {

	defer c.d.wg.Done()

	// Test mode: get limit and open ID log file
	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// Track processed molecules in test mode
	processedMols := make(map[string]bool)
	var molCount int

	fr := config.Dataconf["chembl_molecule"]["id"]

	// set indications
	indicationFtpPath := c.getFtpPath(config.Dataconf["chembl_molecule"]["pathIndicationPattern"])
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, c.ftpHost, c.ftpPath, indicationFtpPath)
	check(err)

	dec := rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/molecule/") {
			switch triple.Pred.String() {
			case "hasDrugIndication":
				indid := c.getChemblID(triple.Obj.String())
				if _, ok := c.indications[indid]; ok {
					id := c.getChemblID(triple.Subj.String())

					// Test mode: only process molecules in our test set
					if config.IsTestMode() && !processedMols[id] {
						continue
					}

					chemMol := pbuf.ChemblMolecule{Indications: []*pbuf.ChemblIndication{c.indications[indid]}}
					attr := pbuf.ChemblAttr{Molecule: &chemMol}
					b, _ := ffjson.Marshal(attr)
					c.d.addProp3(id, fr, b)

					if len(c.indications[indid].Efo) > 0 {
						c.d.addXref(id, fr, c.indications[indid].Efo, "efo", false)
					}

				}
			}
		}
	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
	if client != nil {
		client.Quit()
	}

	gz.Close()

	// set_molecule (main molecule file - always process this)
	moleculeFtpPath := c.getFtpPath(config.Dataconf["chembl_molecule"]["pathMoleculePattern"])
	br, gz, ftpFile, client, localFile, _, err = getDataReaderNew(c.source, c.ftpHost, c.ftpPath, moleculeFtpPath)
	check(err)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	var b []byte
	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		// Test mode: stop early if we've already processed enough molecules
		// Use break instead of return so parseUnichemMappings is still called
		if config.IsTestMode() && shouldStopProcessing(testLimit, molCount) {
			if ftpFile != nil {
				ftpFile.Close()
				ftpFile = nil
			}
			if localFile != nil {
				localFile.Close()
				localFile = nil
			}
			if client != nil {
				client.Quit()
				client = nil
			}
			gz.Close()
			gz = nil
			break
		}

		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > c.previous+c.d.progInterval {
			kbytesPerSecond := int64(c.totalRead) / elapsed / 1024
			c.previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: kbytesPerSecond}
		}
		if triple.Subj == nil || triple.Obj == nil || triple.Pred == nil { // workaround for macos
			continue
		}
		c.totalRead = c.totalRead + len(triple.Subj.String()) + len(triple.Obj.String()) + len(triple.Pred.String())

		if strings.HasPrefix(triple.Subj.String(), "/molecule/") {
			// Test mode: track molecule ID on FIRST appearance (any predicate)
			// Only count actual CHEMBL IDs, not blank nodes (which contain "#")
			id := c.getChemblID(triple.Subj.String())
			if idLogFile != nil && !processedMols[id] && !strings.Contains(id, "#") {
				logProcessedID(idLogFile, id)
				processedMols[id] = true
				molCount++
			}

			switch triple.Pred.String() {
			case "http://purl.org/dc/terms/description":
				attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Desc: triple.Obj.String()}}
				b, _ = ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "fracClassification":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{FracClassification: triple.Obj.String()}}
				b, _ = ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "highestDevelopmentPhase":
				id := c.getChemblID(triple.Subj.String())
				ccFloat, err := strconv.ParseFloat(triple.Obj.String(), 64)
				check(err)
				cc := int32(ccFloat)
				if cc > 0 { // think again
					attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{HighestDevelopmentPhase: cc}}
					b, _ = ffjson.Marshal(attr)
					c.d.addProp3(id, fr, b)
				}
			case "hracClassification":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{HracClassification: triple.Obj.String()}}
				b, _ = ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "substanceType":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Type: triple.Obj.String()}}
				b, _ = ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "http://www.w3.org/2004/02/skos/core#altLabel":
				if len(triple.Obj.String()) < 100 { // otherwise there is generate issue and also too long to be a search key
					id := c.getChemblID(triple.Subj.String())
					attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{AltNames: []string{triple.Obj.String()}}}
					b, _ = ffjson.Marshal(attr)
					c.d.addProp3(id, fr, b)
					c.d.addXref(triple.Obj.String(), textLinkID, id, "chembl_molecule", true)
				}
			case "atcClassification":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{AtcClassification: []string{triple.Obj.String()}}}
				b, _ = ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "iracClassification":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{IracClassification: triple.Obj.String()}}
				b, _ = ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "isBiotherapeutic":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{IsBiotherapeutic: triple.Obj.String()}}
				b, _ = ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "helmNotation":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{HelmNotation: triple.Obj.String()}}
				b, _ = ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "moleculeXref": // this is wikipedia and we take it as a name.
				id := c.getChemblID(triple.Subj.String())
				name := c.getChemblID(triple.Obj.String())
				attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Name: name}}
				b, _ = ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
				// link the molecule name
				c.d.addXref(name, textLinkID, id, "chembl_molecule", true)
			case "hasBioComponent":
				id := c.getChemblID(triple.Subj.String())
				bioid := c.getChemblID(triple.Obj.String())
				if _, ok := c.biocomponents[bioid]; ok {
					chemMol := pbuf.ChemblMolecule{}
					chemMol.BioComponentDescription = c.biocomponents[bioid].description
					chemMol.BioComponentSquence = c.biocomponents[bioid].sequence
					chemMol.BioComponentType = c.biocomponents[bioid].typee
					chemMol.BioComponentTaxo = c.biocomponents[bioid].taxo
					attr := pbuf.ChemblAttr{Molecule: &chemMol}
					b, _ = ffjson.Marshal(attr)
					c.d.addProp3(id, fr, b)
				}
			case "hasDocument":
				id := c.getChemblID(triple.Subj.String())
				docid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, docid, "chembl_document", false)
			case "http://semanticscience.org/resource/SIO_000300":
				id := c.getChemblID(triple.Subj.String())
				propArr := strings.Split(id, "#")
				if len(propArr) > 1 {
					id = propArr[0]
					prop := propArr[1]
					switch prop {
					case "standard_inchi":
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Inchi: triple.Obj.String()}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
						c.d.addXref(triple.Obj.String(), textLinkID, id, "chembl_molecule", true)
					case "standard_inchi_key":
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{InchiKey: triple.Obj.String()}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
						c.d.addXref(triple.Obj.String(), textLinkID, id, "chembl_molecule", true)
					case "canonical_smiles":
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Smiles: triple.Obj.String()}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
						c.d.addXref(triple.Obj.String(), textLinkID, id, "chembl_molecule", true)
					case "alogp":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Alogp: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "mw_freebase":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{WeightFreebase: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "hba":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Hba: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "hbd":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Hbd: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "psa":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Psa: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "rtb":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Rtb: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "ro3_pass":
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Ro3Pass: triple.Obj.String()}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "num_ro5_violations":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{NumRo5Violations: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "acd_most_apka":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{AcdMostApka: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "acd_most_bpka":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{AcdMostBpka: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "acd_logp":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{AcdLogp: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "acd_logd":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{AcdLogd: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "molecular_species":
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{MolecularSpecies: triple.Obj.String()}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "full_mwt":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Weight: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "aromatic_rings":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{AromaticRings: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "heavy_atoms":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{HeavyAtoms: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "qed_weighted":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{QedWeighted: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					case "full_molformula":
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Formula: triple.Obj.String()}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
						c.d.addXref(triple.Obj.String(), textLinkID, id, "chembl_molecule", true)
					case "mw_monoisotopic":
						cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
						check(err)
						attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{WeightMonoisotopic: cc}}
						b, _ = ffjson.Marshal(attr)
						c.d.addProp3(id, fr, b)
					}
				}
				// image url is invalid at the momentcase "http://xmlns.com/foaf/0.1/depiction":
			}
		}
	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}

	if client != nil {
		client.Quit()
	}

	if gz != nil {
		gz.Close()
	}

	// chebi molecule linkset - REMOVED: ChEMBL no longer provides this file
	// ChEMBL changed their file structure and removed the ChEBI-ChEMBL linkset
	// The file pattern "chembl*molecule_chebi_ls.ttl.gz" no longer exists
	/**
	chebiFtpPath := c.getFtpPath(config.Dataconf["chembl_molecule"]["pathChebiPattern"])
	br, gz, ftpFile, client, localFile, _, err = getDataReaderNew(c.source, c.ftpHost, c.ftpPath, chebiFtpPath)
	check(err)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/molecule/") {
			switch triple.Pred.String() {
			case "http://www.w3.org/2004/02/skos/core#exactMatch":
				id := c.getChemblID(triple.Subj.String())
				chebiid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, strings.Replace(chebiid, "_", ":", -1), "chebi", false)
			}
		}

	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
	if client != nil {
		client.Quit()
	}

	gz.Close()
	**/

	//molecule Hierarchy
	hieararchyFtpPath := c.getFtpPath(config.Dataconf["chembl_molecule"]["pathHierarchyPattern"])
	br, gz, ftpFile, client, localFile, _, err = getDataReaderNew(c.source, c.ftpHost, c.ftpPath, hieararchyFtpPath)
	check(err)
	dec = rdf.NewTripleDecoder(br, rdf.Turtle)
	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/molecule/") {
			id := c.getChemblID(triple.Subj.String())

			// Test mode: only process molecules in our test set
			if config.IsTestMode() && !processedMols[id] {
				continue
			}

			switch triple.Pred.String() {
			case "hasChildMolecule":
				childid := c.getChemblID(triple.Obj.String())
				attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Childs: []string{childid}}}
				b, _ = ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "hasParentMolecule":
				parentid := c.getChemblID(triple.Obj.String())
				attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Parent: parentid}}
				b, _ = ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			}
		}
	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}

	if client != nil {
		client.Quit()
	}

	gz.Close()

	// Parse UniChem mappings for PubChem and ZINC cross-references
	fmt.Println("[ChEMBL] About to call parseUnichemMappings...")
	c.parseUnichemMappings(processedMols)
	fmt.Println("[ChEMBL] parseUnichemMappings completed")

}

// parseUnichemMappings parses unichem.ttl.gz for cross-references to PubChem and ZINC
// This provides comprehensive ChEMBL ↔ PubChem mappings (2.68M+) for better disease→compound coverage
// and ZINC database IDs for virtual screening support
func (c *chembl) parseUnichemMappings(processedMols map[string]bool) {
	// Check if PubChem dataset is configured
	_, hasPubchem := config.Dataconf["pubchem"]
	if !hasPubchem {
		fmt.Println("[ChEMBL] PubChem dataset not configured, skipping unichem mappings")
		return
	}

	fr := config.Dataconf["chembl_molecule"]["id"]

	// Get unichem file path
	unichemFtpPath := c.getFtpPath(config.Dataconf["chembl_molecule"]["pathUnichemPattern"])
	fmt.Printf("[ChEMBL] Parsing UniChem mappings from %s\n", unichemFtpPath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, c.ftpHost, c.ftpPath, unichemFtpPath)
	if err != nil {
		fmt.Printf("[ChEMBL] Warning: Could not load UniChem mappings: %v\n", err)
		return
	}

	defer func() {
		if gz != nil {
			gz.Close()
		}
		if ftpFile != nil {
			ftpFile.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
		if client != nil {
			client.Quit()
		}
	}()

	dec := rdf.NewTripleDecoder(br, rdf.Turtle)

	pubchemXrefs := 0
	zincXrefs := 0
	testLimit := config.GetTestLimit(c.source)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {
		if triple.Subj == nil || triple.Obj == nil || triple.Pred == nil {
			continue
		}

		// Only process moleculeXref predicates
		if triple.Pred.String() != "moleculeXref" {
			continue
		}

		// Only process molecule subjects
		if !strings.HasPrefix(triple.Subj.String(), "/molecule/") {
			continue
		}

		chemblID := c.getChemblID(triple.Subj.String())

		// Test mode: only process molecules in our test set
		if config.IsTestMode() && len(processedMols) > 0 && !processedMols[chemblID] {
			continue
		}

		objURL := triple.Obj.String()

		// Extract PubChem CID from URL like: http://pubchem.ncbi.nlm.nih.gov/compound/46359070
		if strings.Contains(objURL, "pubchem.ncbi.nlm.nih.gov/compound/") {
			parts := strings.Split(objURL, "/compound/")
			if len(parts) == 2 {
				pubchemCID := parts[1]
				// Create bidirectional xref: ChEMBL ↔ PubChem
				c.d.addXref(chemblID, fr, pubchemCID, "pubchem", false)
				pubchemXrefs++
			}
		}

		// Extract ZINC ID from URL like: http://zinc15.docking.org/substances/ZINC000033071208
		if strings.Contains(objURL, "zinc") && strings.Contains(objURL, "/substances/") {
			parts := strings.Split(objURL, "/substances/")
			if len(parts) == 2 {
				zincID := parts[1]
				// Make ZINC ID searchable (text link to ChEMBL molecule)
				c.d.addXref(zincID, textLinkID, chemblID, "chembl_molecule", true)
				zincXrefs++
			}
		}

		// Test mode: limit processing
		if config.IsTestMode() && pubchemXrefs+zincXrefs >= testLimit*10 {
			break
		}

		// Progress reporting
		if (pubchemXrefs+zincXrefs)%100000 == 0 && (pubchemXrefs+zincXrefs) > 0 {
			fmt.Printf("[ChEMBL] UniChem progress: %d PubChem xrefs, %d ZINC xrefs\n", pubchemXrefs, zincXrefs)
		}
	}

	fmt.Printf("[ChEMBL] UniChem parsing complete:\n")
	fmt.Printf("[ChEMBL]   - PubChem xrefs: %d (bidirectional ChEMBL ↔ PubChem)\n", pubchemXrefs)
	fmt.Printf("[ChEMBL]   - ZINC xrefs: %d (searchable text links)\n", zincXrefs)
}

func (c *chembl) updateActivity() {
	// Test mode: get limit and open ID log file
	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// Track processed activities in test mode
	processedActivities := make(map[string]bool)
	var activityCount int

	fr := config.Dataconf["chembl_activity"]["id"]
	activityFtpPath := c.getFtpPath(config.Dataconf["chembl_activity"]["pathActivityPattern"])
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, c.ftpHost, c.ftpPath, activityFtpPath)
	check(err)

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	if client != nil {
		defer client.Quit()
	}
	defer gz.Close()
	defer c.d.wg.Done()

	dec := rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		// Test mode: stop early if we've already processed enough activities
		if config.IsTestMode() && shouldStopProcessing(testLimit, activityCount) {
			return
		}

		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > c.previous+c.d.progInterval {
			kbytesPerSecond := int64(c.totalRead) / elapsed / 1024
			c.previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: kbytesPerSecond}
		}
		if triple.Subj == nil || triple.Obj == nil || triple.Pred == nil { // workaround for macos
			continue
		}
		c.totalRead = c.totalRead + len(triple.Subj.String()) + len(triple.Obj.String()) + len(triple.Pred.String())

		if strings.HasPrefix(triple.Subj.String(), "/activity/") {
			// Test mode: track activity ID on FIRST appearance (any predicate)
			// Only count actual activity IDs, not blank nodes (which contain "#")
			id := c.getChemblID(triple.Subj.String())
			if idLogFile != nil && !processedActivities[id] && !strings.Contains(id, "#") {
				logProcessedID(idLogFile, id)
				processedActivities[id] = true
				activityCount++
			}

			switch triple.Pred.String() {
			case "type":
				attr := pbuf.ChemblAttr{Activity: &pbuf.ChemblActivity{Type: strings.ToLower(triple.Obj.String())}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "relation":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Activity: &pbuf.ChemblActivity{Relation: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "value":
				id := c.getChemblID(triple.Subj.String())
				cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
				check(err)
				attr := pbuf.ChemblAttr{Activity: &pbuf.ChemblActivity{Value: cc}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "units":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Activity: &pbuf.ChemblActivity{Units: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "standardType":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Activity: &pbuf.ChemblActivity{StandardType: strings.ToLower(triple.Obj.String())}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "standardRelation":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Activity: &pbuf.ChemblActivity{StandardRelation: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "standardValue":
				id := c.getChemblID(triple.Subj.String())
				cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
				check(err)
				attr := pbuf.ChemblAttr{Activity: &pbuf.ChemblActivity{StandardValue: cc}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "standardUnits":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Activity: &pbuf.ChemblActivity{StandardUnits: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "dataValidityIssue":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Activity: &pbuf.ChemblActivity{DataValidityIssue: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "dataValidityComment":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Activity: &pbuf.ChemblActivity{DataValidityComment: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "pChembl":
				id := c.getChemblID(triple.Subj.String())
				cc, err := strconv.ParseFloat(triple.Obj.String(), 64)
				check(err)
				attr := pbuf.ChemblAttr{Activity: &pbuf.ChemblActivity{PChembl: cc}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "activityComment":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Activity: &pbuf.ChemblActivity{ActivityComment: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "potentialDuplicate":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Activity: &pbuf.ChemblActivity{PotentialDuplicate: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "hasMolecule":
				id := c.getChemblID(triple.Subj.String())
				molid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, molid, "chembl_molecule", false)
			case "hasDocument":
				id := c.getChemblID(triple.Subj.String())
				docid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, docid, "chembl_document", false)
			case "http://www.bioassayontology.org/bao#BAO_0000208":
				id := c.getChemblID(triple.Subj.String())
				baoidarr := strings.Split(c.getChemblID(triple.Obj.String()), "#")
				baoid := ""
				if len(baoidarr) == 2 {
					baoid = baoidarr[1]
				} else {
					baoid = baoidarr[0]
				}
				attr := pbuf.ChemblAttr{Activity: &pbuf.ChemblActivity{Bao: baoid}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			}
		}
	}

}

func (c *chembl) updateAssay() {

	defer c.d.wg.Done()

	// Test mode: get limit and open ID log file
	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// Track processed assays in test mode
	processedAssays := make(map[string]bool)
	var assayCount int

	assaySources := map[string]*assaysource{}
	fr := config.Dataconf["chembl_assay"]["id"]
	assaySourceFtpPath := c.getFtpPath(config.Dataconf["chembl_assay"]["pathAssaySourcePattern"])
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, c.ftpHost, c.ftpPath, assaySourceFtpPath)
	check(err)

	dec := rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/source/") {
			id := c.getChemblID(triple.Subj.String())
			switch triple.Pred.String() {
			case "http://www.w3.org/2000/01/rdf-schema#label":

				if _, ok := assaySources[id]; ok {
					source := assaySources[id]
					source.name = triple.Obj.String()
					assaySources[id] = source
				} else {
					source := assaysource{}
					source.name = triple.Obj.String()
					assaySources[id] = &source
				}

			case "http://purl.org/dc/terms/description":
				if _, ok := assaySources[id]; ok {
					source := assaySources[id]
					source.description = triple.Obj.String()
					assaySources[id] = source
				} else {
					source := assaysource{}
					source.description = triple.Obj.String()
					assaySources[id] = &source
				}
			}
		}

	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
	if client != nil {
		client.Quit()
	}
	gz.Close()

	assayFtpPath := c.getFtpPath(config.Dataconf["chembl_assay"]["pathAssayPattern"])
	br, gz, ftpFile, client, localFile, _, err = getDataReaderNew(c.source, c.ftpHost, c.ftpPath, assayFtpPath)
	check(err)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		// Test mode: stop early if we've already processed enough assays
		if config.IsTestMode() && shouldStopProcessing(testLimit, assayCount) {
			if ftpFile != nil {
				ftpFile.Close()
			}
			if localFile != nil {
				localFile.Close()
			}
			if client != nil {
				client.Quit()
			}
			gz.Close()
			return
		}

		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > c.previous+c.d.progInterval {
			kbytesPerSecond := int64(c.totalRead) / elapsed / 1024
			c.previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: kbytesPerSecond}
		}
		c.totalRead = c.totalRead + len(triple.Subj.String()) + len(triple.Obj.String()) + len(triple.Pred.String())

		if strings.HasPrefix(triple.Subj.String(), "/assay/") {

			// Test mode: track assay ID on FIRST appearance (any predicate)
			// Only count actual assay IDs, not blank nodes (which contain "#")
			id := c.getChemblID(triple.Subj.String())
			if idLogFile != nil && !processedAssays[id] && !strings.Contains(id, "#") {
				logProcessedID(idLogFile, id)
				processedAssays[id] = true
				assayCount++
			}

			switch triple.Pred.String() {
			case "http://purl.org/dc/terms/description":
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Desc: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "assayType":
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Type: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "targetConfDesc":
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{TargetConfDesc: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "targetConfScore":
				cc, err := strconv.Atoi(triple.Obj.String())
				check(err)
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{TargetConfScore: int32(cc)}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "targetRelType":
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{TargetRelType: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "targetRelDesc":
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{TargetRelDesc: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "http://www.bioassayontology.org/bao#BAO_0000205":
				baoidarr := strings.Split(c.getChemblID(triple.Obj.String()), "#")
				baoid := ""
				if len(baoidarr) == 2 {
					baoid = baoidarr[1]
				} else {
					baoid = baoidarr[0]
				}
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Bao: baoid}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "assayTissue":
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Tissue: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "hasTarget":
				docid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, docid, "chembl_target", false)
			case "hasSource":
				sourceid := c.getChemblID(triple.Obj.String())
				if _, ok := assaySources[sourceid]; ok {
					attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Source: strings.ToLower(assaySources[sourceid].name), SourceDesc: assaySources[sourceid].description}}
					b, _ := ffjson.Marshal(attr)
					c.d.addProp3(id, fr, b)
				}
			case "hasDocument":
				docid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, docid, "chembl_document", false)
			case "hasActivity":
				docid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, docid, "chembl_activity", false)
			case "taxonomy":
				taxid := c.getTaxID(triple.Obj.String())
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Tax: taxid}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
				c.d.addXref(id, fr, taxid, "taxonomy", false)
			case "hasCellLine":
				docid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, docid, "chembl_cell_line", false)
			case "assayXref":
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Tissue: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "assayCellType":
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{CellType: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "assaySubCellFrac":
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{SubCellFrac: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "assayTestType":
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{TestType: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "assayStrain":
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Strain: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "assayCategory":
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Category: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			}

		}

	}

	if ftpFile != nil {
		ftpFile.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
	if client != nil {
		client.Quit()
	}
	gz.Close()

}

func (c *chembl) updateDocument() {

	defer c.d.wg.Done()

	// Test mode: get limit and open ID log file
	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// Track processed documents in test mode
	processedDocs := make(map[string]bool)
	var docCount int

	// JOURNALS
	fr := config.Dataconf["chembl_document"]["id"]
	journalFtpPath := c.getFtpPath(config.Dataconf["chembl_document"]["pathJournalPattern"])
	br, gz, ftpFile, client, _, _, err := getDataReaderNew(c.source, c.ftpHost, c.ftpPath, journalFtpPath)
	check(err)

	journalData := map[string][2]string{}
	dec := rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		switch triple.Pred.String() {
		case "http://purl.org/dc/terms/title":
			id := c.getChemblID(triple.Subj.String())

			var vals [2]string
			if _, ok := journalData[id]; ok {
				vals = journalData[id]
			} else {
				vals = [2]string{}
			}
			vals[0] = triple.Obj.String()
			journalData[id] = vals

		case "http://purl.org/ontology/bibo/shortTitle":
			id := c.getChemblID(triple.Subj.String())

			var vals [2]string
			if _, ok := journalData[id]; ok {
				vals = journalData[id]
			} else {
				vals = [2]string{}
			}
			vals[1] = triple.Obj.String()
			journalData[id] = vals

		}

	}
	if ftpFile != nil {
		ftpFile.Close()
	}
	if gz != nil {
		gz.Close()
	}
	if client != nil {
		client.Quit()
	}

	// DOCUMENTS
	documentFtpPath := c.getFtpPath(config.Dataconf["chembl_document"]["pathDocumentPattern"])
	br, gz, ftpFile, client, _, _, err = getDataReaderNew(c.source, c.ftpHost, c.ftpPath, documentFtpPath)
	check(err)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)
	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > c.previous+c.d.progInterval {
			kbytesPerSecond := int64(c.totalRead) / elapsed / 1024
			c.previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: kbytesPerSecond}
		}
		c.totalRead = c.totalRead + len(triple.Subj.String()) + len(triple.Obj.String()) + len(triple.Pred.String())

		switch triple.Pred.String() {
		case "documentType":
			id := c.getChemblID(triple.Subj.String())

			// Test mode: log ID (only once per document) and check limit
			if idLogFile != nil && !processedDocs[id] {
				logProcessedID(idLogFile, id)
				processedDocs[id] = true
				docCount++

				// Check if we've reached the test limit
				if shouldStopProcessing(testLimit, docCount) {
					goto done
				}
			}

			attr := pbuf.ChemblAttr{Doc: &pbuf.ChemblDocument{Type: strings.ToLower(triple.Obj.String())}}
			b, _ := ffjson.Marshal(attr)
			c.d.addProp3(id, fr, b)
		case "hasJournal":
			id := c.getChemblID(triple.Subj.String())
			journalID := c.getChemblID(triple.Obj.String())

			if _, ok := journalData[journalID]; ok {
				attr := pbuf.ChemblAttr{Doc: &pbuf.ChemblDocument{Journal: journalData[journalID][0], JournalShortName: journalData[journalID][1]}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			}
		case "http://purl.org/dc/terms/title":
			id := c.getChemblID(triple.Subj.String())
			attr := pbuf.ChemblAttr{Doc: &pbuf.ChemblDocument{Title: triple.Obj.String()}}
			b, _ := ffjson.Marshal(attr)
			c.d.addProp3(id, fr, b)
		case "http://purl.org/ontology/bibo/doi":
			id := c.getChemblID(triple.Subj.String())
			c.d.addXref(id, fr, triple.Obj.String(), "doi", false)
		case "http://purl.org/ontology/bibo/pmid":
			id := c.getChemblID(triple.Subj.String())
			// Extract PubMed ID from URL like HTTP://IDENTIFIERS.ORG/PUBMED/6130154
			pmidURL := triple.Obj.String()
			pmid := pmidURL
			if lastSlash := strings.LastIndex(pmidURL, "/"); lastSlash >= 0 && lastSlash < len(pmidURL)-1 {
				pmid = pmidURL[lastSlash+1:]
			}
			c.d.addXref(id, fr, pmid, "pubmed", false)
			//case "http://purl.org/ontology/bibo/issue":
			//case "http://purl.org/ontology/bibo/volume":
			//case "http://purl.org/ontology/bibo/pageStart":
			//case "http://purl.org/ontology/bibo/pageEnd":
			//case "http://purl.org/dc/terms/date":
		}
	}

done:
	if ftpFile != nil {
		ftpFile.Close()
	}
	if gz != nil {
		gz.Close()
	}
	if client != nil {
		client.Quit()
	}

}

func (c *chembl) getChemblID(uri string) string {

	pos := 0
	for pos = len(uri) - 1; pos >= 0; pos-- {
		if uri[pos] == '/' {
			break
		}
	}
	if pos <= 0 { // todo needed?
		panic("Invalid uri")
	}
	return uri[pos+1:]

}

// extractOntologyID extracts and validates an ontology ID from a URI
// Returns the ID in PREFIX:NNNNN format if valid, empty string otherwise
// Filters out malformed URLs like exp.php?expert=309005
func (c *chembl) extractOntologyID(uri string) string {
	// Get last part after /
	pos := strings.LastIndex(uri, "/")
	if pos < 0 || pos >= len(uri)-1 {
		return ""
	}
	lastPart := uri[pos+1:]

	// Must contain underscore (PREFIX_NNNNN format)
	if !strings.Contains(lastPart, "_") {
		return ""
	}

	// Skip URLs with query parameters or file extensions
	if strings.Contains(lastPart, "?") || strings.Contains(lastPart, ".") {
		return ""
	}

	// Convert underscore to colon
	id := strings.ReplaceAll(lastPart, "_", ":")

	// Validate: prefix should be uppercase letters only
	colonIdx := strings.Index(id, ":")
	if colonIdx <= 0 || colonIdx >= len(id)-1 {
		return ""
	}

	prefix := id[:colonIdx]
	for _, ch := range prefix {
		if ch < 'A' || ch > 'Z' {
			return ""
		}
	}

	return id
}

func (c *chembl) getTaxID(uri string) string {

	var stopsep byte
	if strings.HasPrefix(uri, "http://www.ncbi.nlm.nih.gov") {
		stopsep = '='
	} else if strings.HasPrefix(uri, "http://identifiers.org") {
		stopsep = '/'
	} else {
		panic("Unrecognized taxonomy uri" + uri)
	}

	pos := 0
	for pos = len(uri) - 1; pos >= 0; pos-- {
		if uri[pos] == stopsep {
			break
		}
	}
	if pos <= 0 { // todo needed?
		panic("Invalid uri")
	}
	return uri[pos+1:]

}

func (c *chembl) getFtpPath(regexFileName string) string {

	// Get FTP host from config (ChEMBL path is full URL)
	fullURL := config.Dataconf["chembl_molecule"]["path"]
	ftpHost, _, _ := parseFTPURL(fullURL)

	// Try HTTPS directory listing first (EBI has migrated from FTP to HTTPS)
	// ftpHost is like "ftp.ebi.ac.uk:21" - extract just the hostname
	hostOnly := strings.Split(ftpHost, ":")[0]
	if strings.HasPrefix(hostOnly, "ftp.ebi.ac.uk") {
		httpsURL := "https://ftp.ebi.ac.uk" + c.ftpPath
		filename, err := c.getHTTPFilePath(httpsURL, regexFileName)
		if err == nil {
			return filename
		}
		fmt.Printf("HTTPS listing failed, falling back to FTP: %v\n", err)
	}

	// Fall back to FTP
	client := ftpClient(ftpHost)
	entries, err := client.List(c.ftpPath + regexFileName)
	check(err)

	if len(entries) != 1 {
		panic("Error while retrieving path of regex->" + regexFileName)
	}

	return entries[0].Name

}

// getHTTPFilePath fetches directory listing via HTTP and finds files matching the pattern
func (c *chembl) getHTTPFilePath(baseURL string, pattern string) (string, error) {
	// Convert glob pattern to regex (correct order matters!)
	// e.g., "chembl*molecule.ttl.gz" -> "chembl.*molecule\.ttl\.gz"

	// Step 1: Replace * with a placeholder to protect it
	regexPattern := strings.ReplaceAll(pattern, "*", "<<<WILDCARD>>>")

	// Step 2: Escape regex special characters (including dots)
	regexPattern = regexp.QuoteMeta(regexPattern)

	// Step 3: Replace placeholder with .*
	regexPattern = strings.ReplaceAll(regexPattern, "<<<WILDCARD>>>", ".*")

	// Step 4: Add anchors
	regexPattern = "^" + regexPattern + "$"

	re, err := regexp.Compile(regexPattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %v", err)
	}

	// Fetch directory listing
	resp, err := http.Get(baseURL)
	if err != nil {
		return "", fmt.Errorf("HTTP GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP status: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response failed: %v", err)
	}

	// Parse HTML to extract filenames
	// EBI directory listings contain links like: <a href="chembl_36.0_molecule.ttl.gz">
	html := string(body)

	// Extract all hrefs from the HTML
	hrefRegex := regexp.MustCompile(`href="([^"]+)"`)
	matches := hrefRegex.FindAllStringSubmatch(html, -1)

	var matchedFiles []string
	var allFiles []string // For debugging
	for _, match := range matches {
		if len(match) > 1 {
			filename := match[1]
			// Skip parent directory and URLs
			if filename == "../" || strings.HasPrefix(filename, "http") || strings.Contains(filename, "?") {
				continue
			}
			allFiles = append(allFiles, filename)
			// Match against our pattern
			if re.MatchString(filename) {
				matchedFiles = append(matchedFiles, filename)
			}
		}
	}

	if len(matchedFiles) != 1 {
		// Debug output
		fmt.Printf("DEBUG: pattern='%s', regex='%s'\n", pattern, regexPattern)
		fmt.Printf("DEBUG: found %d files total, showing first 10: %v\n", len(allFiles), allFiles[:min(10, len(allFiles))])
		return "", fmt.Errorf("expected 1 match for pattern %s, found %d: %v", pattern, len(matchedFiles), matchedFiles)
	}

	return matchedFiles[0], nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
