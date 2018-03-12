package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/b71729/opendcm"
	"github.com/b71729/opendcm/dictionary"
)

var console = opendcm.NewConsoleLogger(os.Stdout)

var baseFile = filepath.Base(os.Args[0])

func check(err error) {
	if err != nil {
		console.Fatal(err)
	}
}

func main() {
	console.Infof("opendcm version %s", opendcm.OpenDCMVersion)
	if len(os.Args) == 1 || (os.Args[1] == "--help" || os.Args[1] == "-h") {
		goto usage
	} else {
		cmd := os.Args[1]
		switch cmd {
		case "inspect":
			StartInspect()
		case "reduce":
			StartReduce()
		case "gendatadict":
			StartGenDataDict()
		case "createdicom":
			StartCreateDicom()
		default:
			goto usage
		}
	}
	return
usage:
	console.Fatalf("usage: %s [%s] [flags]", baseFile, strings.Join([]string{"inspect", "reduce", "gendatadict", "createdicom"}, " / "))
}

/*
===============================================================================
    Mode: Generate Data Dictionary
===============================================================================
*/

var stringRE, tagRE, uidStartRE, acceptibleVM *regexp.Regexp

func eachToken(data string, cb func(token string)) {
	decoder := xml.NewDecoder(strings.NewReader(data))
	for {
		token, err := decoder.Token()
		if token == nil {
			break
		}
		check(err)
		if token, ok := token.(xml.CharData); ok {
			val := strings.Replace(string(token), "\u200b", " ", -1)
			if stringRE.MatchString(val) {
				cb(val)
			}
		}
	}
}

// parseDataElements accepts a string buffer, and returns an array of `DictEntry`
func parseDataElements(data string) (elements []dictionary.DictEntry) {
	index := -1
	mode := -1
	eachToken(data, func(token string) {
		if tagRE.MatchString(token) {
			mode = 1
			index++
		}
		switch mode {
		case 1:
			elements = append(elements, dictionary.DictEntry{})
			tagString := token[1:][:9]
			tagString = strings.Replace(tagString, ",", "", 1)
			tagInt, err := strconv.ParseUint(tagString, 16, 32)
			check(err)
			elements[index].Tag = dictionary.Tag(tagInt)
			elements[index].Retired = false
		case 2:
			elements[index].NameHuman = token
		case 3:
			elements[index].Name = strings.Replace(token, " ", "", -1)
		case 4:
			if len(token) < 2 {
				token = "UN"
			}
			switch token[:2] {
			case "AE", "AS", "AT", "CS", "DA", "DS", "DT", "FL", "FD", "IS", "LO", "LT", "PN", "SH", "SL", "ST", "SS", "TM", "UI", "UL", "US",
				"OB", "OD", "OF", "OL", "OW", "SQ", "UC", "UR", "UT", "UN": // Table 7.1-1
				elements[index].VR = token[:2]
			default:
				elements[index].VR = "UN"
				console.Warnf("VR for Data Element %s is '%s'. Using 'UN' instead.", elements[index].Tag, token)
			}
		case 5:
			orIndex := strings.Index(token, " or")
			if orIndex > -1 {
				token = token[:orIndex]
			}
			if !acceptibleVM.Match([]byte(token)) {
				console.Warnf("VM for Data Element %s is '%s'. Using 'n' instead.", elements[index].Tag, token)
				token = "n"
			}
			elements[index].VM = token
		case 6:
			if token == "RET" {
				elements[index].Retired = true
			}
		}
		mode++
	})
	return elements
}

// parseUIDs accepts a string buffer, and returns an array of `UIDEntry`
func parseUIDs(data string) (uids []dictionary.UIDEntry) {
	index := -1
	mode := -1
	eachToken(data, func(token string) {
		if uidStartRE.Match([]byte(token)) {
			mode = 1
			index++
		}
		switch mode {
		case 1:
			uids = append(uids, dictionary.UIDEntry{})
			uids[index].UID = strings.Replace(token, " ", "", -1)
		case 2:
			uids[index].NameHuman = token
		case 3:
			uids[index].Type = token
		}
		mode++
	})
	return uids
}

func tableBodyPosition(data string) (posStart int, posEnd int, err error) {
	posStart = strings.Index(data, "<tbody>")
	if posStart <= 0 {
		return 0, 0, errors.New("could not find <tbody>")
	}
	posEnd = strings.Index(data, "</tbody>")
	if posEnd <= 0 {
		return posStart, 0, errors.New("could not find </tbody>")
	}
	return posStart, posEnd, nil
}

func StartGenDataDict() {
	if len(os.Args) != 3 {
		console.Fatalf("usage: %s gendatadict dictFromNEMA.xml", baseFile)
	}

	// read input XML file to buffer
	stat, err := os.Stat(os.Args[2])
	check(err)

	f, err := os.Open(os.Args[2])
	check(err)
	defer f.Close()

	buf := make([]byte, stat.Size())
	_, err = f.Read(buf)
	check(err)

	data := string(buf)
	tagRE = regexp.MustCompile(`\([0-9A-Fa-f]{4},[0-9A-Fa-f]{4}\)`)
	uidStartRE = regexp.MustCompile(`([0-9]+\.[0-9]+\.[0-9]+)`)
	stringRE = regexp.MustCompile("([a-zA-Z0-9])")
	acceptibleVM = regexp.MustCompile("^([0-9-n]+)$")
	// data elements
	posStart, posEnd, err := tableBodyPosition(data)
	check(err)

	dataElements := parseDataElements(data[posStart+7 : posEnd])
	console.Infof("found %d data elements", len(dataElements))

	// file meta elements
	data = data[posEnd+8:]
	posStart, posEnd, err = tableBodyPosition(data)
	check(err)

	fileMetaElements := parseDataElements(data[posStart+7 : posEnd])
	console.Infof("found %d file meta elements", len(fileMetaElements))

	// directory structure elements
	data = data[posEnd+8:]
	posStart, posEnd, err = tableBodyPosition(data)
	check(err)

	dirStructElements := parseDataElements(data[posStart+7 : posEnd])
	console.Infof("found %d directory structure elements", len(dirStructElements))

	// UIDs
	data = data[posEnd+8:]
	posStart, posEnd, err = tableBodyPosition(data)
	check(err)

	UIDs := parseUIDs(data[posStart+7 : posEnd])
	console.Infof("found %d UIDs elements", len(UIDs))

	// build golang string
	outF, err := os.Create("datadict.go")
	check(err)
	outCode := `// Code generated using util:gendatadict. DO NOT EDIT.
package dictionary

import "fmt"

type Tag uint32

type DictEntry struct {
	Tag       Tag
	NameHuman string
	Name      string
	VR        string
	VM        string
	Retired   bool
}

type UIDEntry struct {
	UID       string
	Type      string
	NameHuman string
}

func (t Tag) String() string {
	upper := uint16(t >> 16)
	lower := uint16(t)
	return fmt.Sprintf("(%04X,%04X)", upper, lower)
}

// DicomDictionary provides a mapping between uint32 representation of a DICOM Tag and a DictEntry pointer.
var DicomDictionary = map[uint32]*DictEntry{
`
	outCode += "    // File Meta Elements\n"
	for _, v := range fileMetaElements {
		outCode += fmt.Sprintf(`    0x%08X: {Tag: 0x%08X, Name: "%s", NameHuman: "%s", VR: "%s", Retired: %v},`, uint32(v.Tag), uint32(v.Tag), v.Name, v.NameHuman, v.VR, v.Retired) + "\n"
	}

	outCode += "    // Directory Structure Elements\n"
	for _, v := range dirStructElements {
		outCode += fmt.Sprintf(`    0x%08X: {Tag: 0x%08X, Name: "%s", NameHuman: "%s", VR: "%s", VM: "%s", Retired: %v},`, uint32(v.Tag), uint32(v.Tag), v.Name, v.NameHuman, v.VR, v.VM, v.Retired) + "\n"
	}

	outCode += "    // Data Elements\n"
	for _, v := range dataElements {
		outCode += fmt.Sprintf(`    0x%08X: {Tag: 0x%08X, Name: "%s", NameHuman: "%s", VR: "%s", VM: "%s", Retired: %v},`, uint32(v.Tag), uint32(v.Tag), v.Name, v.NameHuman, v.VR, v.VM, v.Retired) + "\n"
	}

	outCode += `}

// UIDs
var UIDDictionary = map[string]*UIDEntry{
    `
	for _, v := range UIDs {
		outCode += fmt.Sprintf(`    "%s": {UID: "%s", Type: "%s", NameHuman: "%s"},`, v.UID, v.UID, v.Type, v.NameHuman) + "\n"
	}

	outCode += `}
    `
	// write to disk
	_, err = outF.WriteString(outCode)
	check(err)
	console.Infof("wrote file datadict.go OK")
}

/*
===============================================================================
    Mode: Create DICOM File
===============================================================================
*/

// TODO: move to common
func tagStringToTagUint32(tag string) (uint32, error) {
	tagString := strings.Replace(tag, ",", "", 1)
	tagInt, err := strconv.ParseUint(tagString, 16, 32)
	return uint32(tagInt), err
}

func generateElement(tagString string, value []byte, VR string) ([]byte, error) {
	return generateElementWithLength(tagString, value, VR, uint32(len(value)))
}

// NOTE: Explicit VR, Little Endian
func generateElementWithLength(tagString string, value []byte, VR string, length uint32) ([]byte, error) {
	ret := make([]byte, 4)
	tag, err := tagStringToTagUint32(tagString)
	if err != nil {
		return ret, nil
	}
	binary.LittleEndian.PutUint16(ret[0:], uint16(tag>>16))
	binary.LittleEndian.PutUint16(ret[2:], uint16(tag))
	ret = append(ret, []byte(VR)...)

	if length > 0 {
		// deal with padding
		switch VR {
		case "UI", "OB", "CS", "DS", "IS", "AE", "AS", "DA", "DT", "LO", "LT", "OD", "OF", "OW", "PN", "SH", "ST", "TM", "UT":
			if length%2 != 0 {
				value = append(value, 0x00)
				length++
			}
		}
	}

	switch VR {
	case "OB", "OW", "SQ", "UN", "UT":
		if length > 0xFFFFFFFF {
			return nil, errors.New("value length would overflow uint32")
		}
		// write length
		ret = append(ret, make([]byte, 2)...) // skip two bytes
		ret = append(ret, make([]byte, 4)...)
		binary.LittleEndian.PutUint32(ret[len(ret)-4:], length)
	default:
		if length > 0xFFFF {
			return nil, errors.New("value length would overflow uint16")
		}
		// write length
		ret = append(ret, make([]byte, 2)...)
		binary.LittleEndian.PutUint16(ret[len(ret)-2:], uint16(length))
	}
	if length > 0 {
		ret = append(ret, value...)
	}
	//console.Debugf("% 0x", ret)
	return ret, nil
}

// TODO: move to common
func elementFromBuffer(buf []byte) (opendcm.Element, error) {
	r := bufio.NewReader(bytes.NewReader(buf))
	es := opendcm.NewElementStream(r, int64(len(buf)))
	return es.GetElement()
}

func writeMeta() []byte {
	buffer := make([]byte, 128)
	buffer = append(buffer, []byte("DICM")...)

	// 0002,0001 File Meta Version
	elementBytes, err := generateElement("0002,0001", []byte{0x00, 0x01}, "OB")
	check(err)
	buffer = append(buffer, elementBytes...)

	// 0002,0002 Media Storage SOP Class UID
	// Use 1.2.840.10008.5.1.4.1.1.66 (Raw Data Storage), but may need to be adjusted.
	elementBytes, err = generateElement("0002,0002", []byte("1.2.840.10008.5.1.4.1.1.66"), "UI")
	check(err)
	buffer = append(buffer, elementBytes...)

	// 0002,0003 Media Storage SOP Instance UID
	randUID, err := opendcm.NewRandInstanceUID()
	check(err)
	elementBytes, err = generateElement("0002,0003", []byte(randUID), "UI")
	check(err)
	buffer = append(buffer, elementBytes...)

	// 0002,0010 Transfer Syntax UID
	elementBytes, err = generateElement("0002,0010", []byte("1.2.840.10008.1.2.1"), "UI")
	check(err)
	buffer = append(buffer, elementBytes...)

	// 0002,0012 Implementation Class UID
	elementBytes, err = generateElement("0002,0012", []byte(opendcm.GetImplementationUID(true)), "UI")
	check(err)
	buffer = append(buffer, elementBytes...)

	// (0002,0013)    Implementation Version Name    opendcm-0.1
	elementBytes, err = generateElement("0002,0013", []byte(fmt.Sprintf("opendcm-%s", opendcm.OpenDCMVersion)), "SH")
	check(err)
	buffer = append(buffer, elementBytes...)

	// Now return to File Meta Length and populate
	val := make([]byte, 4)
	binary.LittleEndian.PutUint32(val, uint32(len(buffer)-132))
	elementBytes, err = generateElement("0002,0000", val, "UL")
	check(err)
	buffer = append(buffer[:132], append(elementBytes, buffer[132:]...)...)

	return buffer
}

// StartCreateDicom enters "create dicom" mode.
// This allows for the creation of synthetic dicom files. Primary usage is for unit tests and verification of bugs.
func StartCreateDicom() {
	if len(os.Args) != 3 {
		console.Fatalf("usage: %s createdicom out_file", baseFile)
	}
	outFileName := os.Args[2]
	if _, err := os.Stat(outFileName); err == nil {
		console.Fatalf("error: %s already exists", outFileName)
	}

	buffer := writeMeta()

	// write output
	f, err := os.Create(outFileName)
	check(err)
	nwrite, err := f.Write(buffer)
	check(err)
	if nwrite != len(buffer) {
		console.Fatalf("nwrite = %d (!= %d)", nwrite, len(buffer))
	}

	console.Infof("wrote meta information ok")

	elementBuffer := make([]byte, 0)

	// Create overflow element length (past buffer boundary)
	elementBytes, err := generateElementWithLength("0008,0005", []byte(""), "CS", 0xFF)
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	nwrite, err = f.Write(elementBuffer)
	check(err)
	if nwrite != len(elementBuffer) {
		console.Fatalf("nwrite = %d (!= %d)", nwrite, len(elementBuffer))
	}

	console.Infof("wrote elements ok")

	defer f.Close()
}

/*
===============================================================================
    Mode: Reduce DICOM Directory
===============================================================================
*/

// StartReduce enters "reduce" directory mode.
// This scans the input directory for unique dicoms (unique SeriesInstanceUID) and copies those dicoms
//   to the output directory.
// copy the src file to dst. Any existing file will be overwritten and will not
// copy file attributes.
// Source: https://stackoverflow.com/a/21061062
func copy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

func StartReduce() {
	if len(os.Args) != 4 {
		console.Fatalf("usage: %s reduce in_dir out_dir", baseFile)
	}
	dirIn := os.Args[2]
	dirOut := os.Args[3]

	statIn, err := os.Stat(dirIn)
	if err != nil {
		console.Fatal(err)
	}
	if !statIn.IsDir() {
		console.Fatalf("%s is not a directory", dirIn)
	}

	statOut, err := os.Stat(dirOut)
	if err != nil {
		console.Fatal(err)
	}
	if !statOut.IsDir() {
		console.Fatalf("%s is not a directory", dirOut)
	}

	seriesInstanceUIDs := make(map[string]bool, 0)
	opendcm.ConcurrentlyWalkDir(dirIn, func(filePath string) {
		dcm, err := opendcm.ParseDicom(filePath)
		if err != nil {
			console.Errorf("error parsing %s: %v", filePath, err)
			return
		}
		if e, found := dcm.GetElement(0x0020000E); found {
			if val, ok := e.Value().(string); ok {
				_, found := seriesInstanceUIDs[val]
				if !found {
					console.Infof("%s", val)
					seriesInstanceUIDs[val] = true
					outputFilePath := filepath.Join(dirOut, fmt.Sprintf("%s.dcm", val))
					if _, err := os.Stat(outputFilePath); os.IsNotExist(err) {
						// file does not exist - lets create it
						err := copy(dcm.FilePath, outputFilePath)
						if err != nil {
							console.Fatalf("error copying: %v", err)
						}
					} else {
						console.Infof("skip %s: file exists", outputFilePath)
					}
				}
			}
		}
	})
}

/*
===============================================================================
    Mode: Inspect DICOM File
===============================================================================
*/

// StartInspect enters "inspect" mode.
// This allows for the inspection of a dicom file, and listing of its elements.
func StartInspect() {
	if len(os.Args) != 3 {
		console.Fatalf("usage: %s inspect file_or_dir", baseFile)
	}
	stat, err := os.Stat(os.Args[2])
	check(err)
	if isDir := stat.IsDir(); !isDir {
		dcm, err := opendcm.ParseDicom(os.Args[2])
		if err != nil {
			console.Fatalf(`error parsing "%s": %v`, dcm.FilePath, err)
		}
		var elements []opendcm.Element
		for _, v := range dcm.Elements {
			elements = append(elements, v)
		}
		sort.Sort(opendcm.ByTag(elements))
		for _, element := range elements {
			description := element.Describe()
			for _, line := range description {
				console.Info(line)
			}
		}
	} else {
		err := opendcm.ConcurrentlyWalkDir(os.Args[2], func(path string) {
			_, err := opendcm.ParseDicom(path)
			if err == nil {
				console.Infof("%s: parsed ok", filepath.Base(path))
			} else {
				console.Errorf("%s: %v", filepath.Base(path), err)
			}
		})
		check(err)
	}
}
