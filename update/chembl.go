package update

import (
	"biobtree/pbuf"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/pquerna/ffjson/ffjson"

	"github.com/tamerh/rdf"
)

type chembl struct {
	source        string
	d             *DataUpdate
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

	c.ftpPath = config.Appconf["chembl_ftp_path"]

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
	br, gz, ftpFile, localFile, _ := c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, protclassFtpPath)

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
	gz.Close()

}

func (c *chembl) updateBiocomponents() {

	c.biocomponents = map[string]*biocomponent{}

	biocomponentFtpPath := c.getFtpPath(config.Dataconf["chembl_molecule"]["pathBioComponentPattern"])
	br, gz, ftpFile, localFile, _ := c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, biocomponentFtpPath)

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
	gz.Close()

}

func (c *chembl) updateBindingSites() {

	c.bindingSites = map[string]*pbuf.ChemblBindingSite{}

	bindingSiteFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathBindingSitePattern"])
	br, gz, ftpFile, localFile, _ := c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, bindingSiteFtpPath)

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
	gz.Close()

	// now mechanism which use binding sites
	c.updateMechanisms()

}

func (c *chembl) updateMechanisms() {

	c.mechanisms = map[string]*pbuf.ChemblMechanism{}

	//first for just mechanisms name and action type
	mechanismFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathMechanismPattern"])
	br, gz, ftpFile, localFile, _ := c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, mechanismFtpPath)

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
	gz.Close()

	// now set it for bindings , molecule and target
	br, gz, ftpFile, localFile, _ = c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, mechanismFtpPath)

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
				if _, ok := c.mechanisms[id]; ok {
					attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Mechanism: c.mechanisms[id]}}
					b, _ := ffjson.Marshal(attr)
					c.d.addProp3(id, config.Dataconf["chembl_molecule"]["id"], b)
				}
			case "hasTarget":
				if _, ok := c.mechanisms[id]; ok {
					attr := pbuf.ChemblAttr{Target: &pbuf.ChemblTarget{Mechanism: c.mechanisms[id]}}
					b, _ := ffjson.Marshal(attr)
					c.d.addProp3(id, config.Dataconf["chembl_target"]["id"], b)
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
	gz.Close()

}

func (c *chembl) updateCellline() {

	fr := config.Dataconf["chembl_cell_line"]["id"]
	celllineFtpPath := c.getFtpPath(config.Dataconf["chembl_cell_line"]["pathCelllinePattern"])
	br, gz, ftpFile, localFile, _ := c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, celllineFtpPath)

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
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
			id := c.getChemblID(triple.Subj.String())
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
				efoid := c.getChemblID(triple.Obj.String())
				attr := pbuf.ChemblAttr{CellLine: &pbuf.ChemblCellLine{Efo: efoid}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
				c.d.addXref(id, fr, strings.Replace(efoid, "_", ":", 1), "efo", false)
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

	// binding site
	fr := config.Dataconf["chembl_target"]["id"]
	bindingSiteFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathBindingSitePattern"])
	br, gz, ftpFile, localFile, _ := c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, bindingSiteFtpPath)

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
	gz.Close()

	// protein target class
	protclassFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathProteinTargetClassificationPattern"])
	br, gz, ftpFile, localFile, _ = c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, protclassFtpPath)

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
	gz.Close()

	// target
	targetFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathTargetPattern"])
	br, gz, ftpFile, localFile, _ = c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, targetFtpPath)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > c.previous+c.d.progInterval {
			kbytesPerSecond := int64(c.totalRead) / elapsed / 1024
			c.previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: kbytesPerSecond}
		}
		c.totalRead = c.totalRead + len(triple.Subj.String()) + len(triple.Obj.String()) + len(triple.Pred.String())

		if strings.HasPrefix(triple.Subj.String(), "/target/") {
			id := c.getChemblID(triple.Subj.String())
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
	gz.Close()

	// target relations
	targetRelFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathTargetRelPattern"])
	br, gz, ftpFile, localFile, _ = c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, targetRelFtpPath)

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
	gz.Close()

	// single target -> target_component
	singleTargetComptMappingFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathSingleTargetComponentMappingPattern"])
	br, gz, ftpFile, localFile, _ = c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, singleTargetComptMappingFtpPath)

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
	gz.Close()

	// complex target -> target_component
	complexTargetComptMappingFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathComplexTargetComponentMappingPattern"])
	br, gz, ftpFile, localFile, _ = c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, complexTargetComptMappingFtpPath)

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
	gz.Close()

	// group target -> target_component
	groupTargetComptMappingFtpPath := c.getFtpPath(config.Dataconf["chembl_target"]["pathGroupTargetComponentMappingPattern"])
	br, gz, ftpFile, localFile, _ = c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, groupTargetComptMappingFtpPath)

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
	gz.Close()

}

func (c *chembl) updateTargetComponent() {
	fr := config.Dataconf["chembl_target_component"]["id"]
	targetComptFtpPath := c.getFtpPath(config.Dataconf["chembl_target_component"]["pathTargetComponentPattern"])
	br, gz, ftpFile, localFile, _ := c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, targetComptFtpPath)

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
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
	br, gz, ftpFile, localFile, _ = c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, protclassFtpPath)

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
	gz.Close()

	// component xref mapping this is essentially allows gene,protein ids to map the target_component and target
	uniprotMappingFtpPath := c.getFtpPath(config.Dataconf["chembl_target_component"]["pathUniprotMappingPattern"])
	br, gz, ftpFile, localFile, _ = c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, uniprotMappingFtpPath)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > c.previous+c.d.progInterval {
			kbytesPerSecond := int64(c.totalRead) / elapsed / 1024
			c.previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: kbytesPerSecond}
		}
		c.totalRead = c.totalRead + len(triple.Subj.String()) + len(triple.Obj.String()) + len(triple.Pred.String())

		if strings.HasPrefix(triple.Subj.String(), "/targetcomponent/") {
			id := c.getChemblID(triple.Subj.String())
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
	gz.Close()

}

func (c *chembl) updateIndications() {

	c.indications = map[string]*pbuf.ChemblIndication{}

	indicationFtpPath := c.getFtpPath(config.Dataconf["chembl_molecule"]["pathIndicationPattern"])
	br, gz, ftpFile, localFile, _ := c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, indicationFtpPath)

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
				cc, err := strconv.ParseInt(triple.Obj.String(), 10, 32)
				check(err)
				if _, ok := c.indications[id]; ok {
					ind := c.indications[id]
					ind.HighestDevelopmentPhase = int32(cc)
					c.indications[id] = ind
				} else {
					ind := pbuf.ChemblIndication{}
					ind.HighestDevelopmentPhase = int32(cc)
					c.indications[id] = &ind
				}

			case "hasMolecule":
			case "hasEFO":
				id := c.getChemblID(triple.Subj.String())
				efoid := c.getChemblID(triple.Obj.String())
				if _, ok := c.indications[id]; ok {
					ind := c.indications[id]
					ind.Efo = strings.Replace(efoid, "_", ":", 1)
					c.indications[id] = ind
				} else {
					ind := pbuf.ChemblIndication{}
					ind.Efo = strings.Replace(efoid, "_", ":", 1)
					c.indications[id] = &ind
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
	gz.Close()

}

func (c *chembl) updateMolecule() {

	defer c.d.wg.Done()

	// set indications
	fr := config.Dataconf["chembl_molecule"]["id"]
	indicationFtpPath := c.getFtpPath(config.Dataconf["chembl_molecule"]["pathIndicationPattern"])
	br, gz, ftpFile, localFile, _ := c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, indicationFtpPath)

	dec := rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/molecule/") {
			switch triple.Pred.String() {
			case "hasDrugIndication":
				indid := c.getChemblID(triple.Obj.String())
				if _, ok := c.indications[indid]; ok {
					id := c.getChemblID(triple.Subj.String())
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

	gz.Close()

	// set_molecule
	moleculeFtpPath := c.getFtpPath(config.Dataconf["chembl_molecule"]["pathMoleculePattern"])
	br, gz, ftpFile, localFile, _ = c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, moleculeFtpPath)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	var b []byte
	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > c.previous+c.d.progInterval {
			kbytesPerSecond := int64(c.totalRead) / elapsed / 1024
			c.previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: kbytesPerSecond}
		}
		c.totalRead = c.totalRead + len(triple.Subj.String()) + len(triple.Obj.String()) + len(triple.Pred.String())

		if strings.HasPrefix(triple.Subj.String(), "/molecule/") {
			switch triple.Pred.String() {
			case "http://purl.org/dc/terms/description":
				id := c.getChemblID(triple.Subj.String())
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
				cc, err := strconv.ParseInt(triple.Obj.String(), 10, 32)
				check(err)
				if cc > 0 { // think again
					attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{HighestDevelopmentPhase: int32(cc)}}
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

	gz.Close()

	// chebi molecule
	chebiFtpPath := c.getFtpPath(config.Dataconf["chembl_molecule"]["pathChebiPattern"])
	br, gz, ftpFile, localFile, _ = c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, chebiFtpPath)

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

	gz.Close()

	//molecule Hierarchy
	hieararchyFtpPath := c.getFtpPath(config.Dataconf["chembl_molecule"]["pathHierarchyPattern"])
	br, gz, ftpFile, localFile, _ = c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, hieararchyFtpPath)
	dec = rdf.NewTripleDecoder(br, rdf.Turtle)
	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		if strings.HasPrefix(triple.Subj.String(), "/molecule/") {
			switch triple.Pred.String() {
			case "hasChildMolecule":
				id := c.getChemblID(triple.Subj.String())
				childid := c.getChemblID(triple.Obj.String())
				attr := pbuf.ChemblAttr{Molecule: &pbuf.ChemblMolecule{Childs: []string{childid}}}
				b, _ = ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "hasParentMolecule":
				id := c.getChemblID(triple.Subj.String())
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

	gz.Close()

}

func (c *chembl) updateActivity() {
	fr := config.Dataconf["chembl_activity"]["id"]
	activityFtpPath := c.getFtpPath(config.Dataconf["chembl_activity"]["pathActivityPattern"])
	br, gz, ftpFile, localFile, _ := c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, activityFtpPath)

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
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

		if strings.HasPrefix(triple.Subj.String(), "/activity/") {
			switch triple.Pred.String() {
			case "type":
				id := c.getChemblID(triple.Subj.String())
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

	assaySources := map[string]*assaysource{}
	fr := config.Dataconf["chembl_assay"]["id"]
	assaySourceFtpPath := c.getFtpPath(config.Dataconf["chembl_assay"]["pathAssaySourcePattern"])
	br, gz, ftpFile, localFile, _ := c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, assaySourceFtpPath)

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
	gz.Close()

	assayFtpPath := c.getFtpPath(config.Dataconf["chembl_assay"]["pathAssayPattern"])
	br, gz, ftpFile, localFile, _ = c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, assayFtpPath)

	dec = rdf.NewTripleDecoder(br, rdf.Turtle)

	for triple, err := dec.Decode(); err != io.EOF; triple, err = dec.Decode() {

		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > c.previous+c.d.progInterval {
			kbytesPerSecond := int64(c.totalRead) / elapsed / 1024
			c.previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: kbytesPerSecond}
		}
		c.totalRead = c.totalRead + len(triple.Subj.String()) + len(triple.Obj.String()) + len(triple.Pred.String())

		if strings.HasPrefix(triple.Subj.String(), "/assay/") {

			switch triple.Pred.String() {
			case "http://purl.org/dc/terms/description":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Desc: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "assayType":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Type: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "targetConfDesc":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{TargetConfDesc: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "targetConfScore":
				cc, err := strconv.Atoi(triple.Obj.String())
				check(err)
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{TargetConfScore: int32(cc)}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "targetRelType":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{TargetRelType: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "targetRelDesc":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{TargetRelDesc: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "http://www.bioassayontology.org/bao#BAO_0000205":
				id := c.getChemblID(triple.Subj.String())
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
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Tissue: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "hasTarget":
				id := c.getChemblID(triple.Subj.String())
				docid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, docid, "chembl_target", false)
			case "hasSource":
				id := c.getChemblID(triple.Subj.String())
				sourceid := c.getChemblID(triple.Obj.String())
				if _, ok := assaySources[sourceid]; ok {
					attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Source: strings.ToLower(assaySources[sourceid].name), SourceDesc: assaySources[sourceid].description}}
					b, _ := ffjson.Marshal(attr)
					c.d.addProp3(id, fr, b)
				}
			case "hasDocument":
				id := c.getChemblID(triple.Subj.String())
				docid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, docid, "chembl_document", false)
			case "hasActivity":
				id := c.getChemblID(triple.Subj.String())
				docid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, docid, "chembl_activity", false)
			case "taxonomy":
				id := c.getChemblID(triple.Subj.String())
				taxid := c.getTaxID(triple.Obj.String())
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Tax: taxid}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
				c.d.addXref(id, fr, taxid, "taxonomy", false)
			case "hasCellLine":
				id := c.getChemblID(triple.Subj.String())
				docid := c.getChemblID(triple.Obj.String())
				c.d.addXref(id, fr, docid, "chembl_cell_line", false)
			case "assayXref":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Tissue: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "assayCellType":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{CellType: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "assaySubCellFrac":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{SubCellFrac: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "assayTestType":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{TestType: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "assayStrain":
				id := c.getChemblID(triple.Subj.String())
				attr := pbuf.ChemblAttr{Assay: &pbuf.ChemblAssay{Strain: triple.Obj.String()}}
				b, _ := ffjson.Marshal(attr)
				c.d.addProp3(id, fr, b)
			case "assayCategory":
				id := c.getChemblID(triple.Subj.String())
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
	gz.Close()

}

func (c *chembl) updateDocument() {

	defer c.d.wg.Done()

	// JOURNALS
	fr := config.Dataconf["chembl_document"]["id"]
	journalFtpPath := c.getFtpPath(config.Dataconf["chembl_document"]["pathJournalPattern"])
	br, gz, ftpFile, _, _ := c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, journalFtpPath)

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
	ftpFile.Close()
	gz.Close()

	// DOCUMENTS
	documentFtpPath := c.getFtpPath(config.Dataconf["chembl_document"]["pathDocumentPattern"])
	br, gz, ftpFile, _, _ = c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.ftpPath, documentFtpPath)

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
			c.d.addXref(id, fr, triple.Obj.String(), "pmc", false)
			//case "http://purl.org/ontology/bibo/issue":
			//case "http://purl.org/ontology/bibo/volume":
			//case "http://purl.org/ontology/bibo/pageStart":
			//case "http://purl.org/ontology/bibo/pageEnd":
			//case "http://purl.org/dc/terms/date":
		}
	}
	ftpFile.Close()
	gz.Close()

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

	client := c.d.ftpClient(c.d.ebiFtp)
	entries, err := client.List(c.ftpPath + regexFileName)
	check(err)

	if len(entries) != 1 {
		panic("Error while retrieving path of regex->" + regexFileName)
	}

	return entries[0].Name

}
