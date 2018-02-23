package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/b71729/opendcm/dictionary"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

var stringRE, tagRE, uidStartRE *regexp.Regexp

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

// ParseDataElements accepts a string buffer, and returns an array of `DictEntry`
func ParseDataElements(data string) (elements []dictionary.DictEntry) {
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
				log.Printf("Note: VR for Data Element %s is '%s'. Using 'UN' instead.", elements[index].Tag, token)
			}
		case 6:
			if token == "RET" {
				elements[index].Retired = true
			}
		}
		mode++
	})
	return elements
}

// ParseUIDs accepts a string buffer, and returns an array of `UIDEntry`
func ParseUIDs(data string) (uids []dictionary.UIDEntry) {
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
		return 0, 0, errors.New("Could not find <tbody>")
	}
	posEnd = strings.Index(data, "</tbody>")
	if posEnd <= 0 {
		return posStart, 0, errors.New("Could not find </tbody>")
	}
	return posStart, posEnd, nil
}

// Generates a DICOM data dictionary file from XML
func main() {
	if len(os.Args) != 2 {
		log.Fatalln("Usage: gendatadict XMLFILEPATH")
	}
	xmlfile := os.Args[1]
	f, err := os.Open(xmlfile)
	check(err)
	stat, err := f.Stat()
	check(err)
	buf := make([]byte, stat.Size())
	_, err = f.Read(buf)
	check(err)

	data := string(buf)
	tagRE, _ = regexp.Compile("\\([0-9A-Fa-f]{4},[0-9A-Fa-f]{4}\\)")
	uidStartRE, _ = regexp.Compile("([0-9]+\\.[0-9]+\\.[0-9]+)")
	stringRE, _ = regexp.Compile("([a-zA-Z0-9])")

	// data elements
	posStart, posEnd, err := tableBodyPosition(data)
	check(err)

	dataElements := ParseDataElements(data[posStart+7 : posEnd])
	log.Printf("Found %d data elements\n", len(dataElements))

	// file meta elements
	data = data[posEnd+8:]
	posStart, posEnd, err = tableBodyPosition(data)
	check(err)

	fileMetaElements := ParseDataElements(data[posStart+7 : posEnd])
	log.Printf("Found %d file meta elements\n", len(fileMetaElements))

	// directory structure elements
	data = data[posEnd+8:]
	posStart, posEnd, err = tableBodyPosition(data)
	check(err)

	dirStructElements := ParseDataElements(data[posStart+7 : posEnd])
	log.Printf("Found %d directory structure elements\n", len(dirStructElements))

	// UIDs
	data = data[posEnd+8:]
	posStart, posEnd, err = tableBodyPosition(data)
	check(err)

	UIDs := ParseUIDs(data[posStart+7 : posEnd])
	log.Printf("Found %d UIDs elements\n", len(UIDs))

	// build golang string
	outF, err := os.Create("../../dictionary/datadict.go")
	check(err)
	outCode := `// Code generated using util:gendatadict. DO NOT EDIT.

package dictionary

import ("fmt")

type Tag uint32

type DictEntry struct {
	Tag       Tag
	NameHuman string
	Name      string
	VR        string
	Retired   bool
}

type UIDEntry struct {
	UID       string
	Type      string
	NameHuman string
}

func (t Tag) String() string {
	upper := uint32(t) >> 16
	lower := uint32(t) & 0xff
	return fmt.Sprintf("(%04X,%04X)", upper, lower)
}
// DicomDictionary provides a mapping between uint32 representation of a DICOM Tag and a DictEntry pointer.
var DicomDictionary = map[uint32]*DictEntry{
`
	outCode += "    // File Meta Elements\n"
	for _, v := range fileMetaElements {
		outCode += fmt.Sprintf(`    0x%08X: &DictEntry{Tag: 0x%08X, Name: "%s", NameHuman: "%s", VR: "%s", Retired: %v},`, uint32(v.Tag), uint32(v.Tag), v.Name, v.NameHuman, v.VR, v.Retired) + "\n"
	}

	outCode += "    // Directory Structure Elements\n"
	for _, v := range dirStructElements {
		outCode += fmt.Sprintf(`    0x%08X: &DictEntry{Tag: 0x%08X, Name: "%s", NameHuman: "%s", VR: "%s", Retired: %v},`, uint32(v.Tag), uint32(v.Tag), v.Name, v.NameHuman, v.VR, v.Retired) + "\n"
	}

	outCode += "    // Data Elements\n"
	for _, v := range dataElements {
		outCode += fmt.Sprintf(`    0x%08X: &DictEntry{Tag: 0x%08X, Name: "%s", NameHuman: "%s", VR: "%s", Retired: %v},`, uint32(v.Tag), uint32(v.Tag), v.Name, v.NameHuman, v.VR, v.Retired) + "\n"
	}

	outCode += `}

// UIDs
var UIDDictionary = map[string]*UIDEntry{
`
	for _, v := range UIDs {
		outCode += fmt.Sprintf(`    "%s": &UIDEntry{UID: "%s", Type: "%s", NameHuman: "%s"},`, v.UID, v.UID, v.Type, v.NameHuman) + "\n"
	}

	outCode += `}
`
	// write to disk
	_, err = outF.WriteString(outCode)
	check(err)
	log.Printf("Wrote file OK.")
}
