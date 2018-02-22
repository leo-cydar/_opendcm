package main

import (
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/b71729/opendcm/core"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func ParseDataElements(buf []byte) []core.DictEntry {

	decoder := xml.NewDecoder(strings.NewReader(string(buf)))
	decoder.Strict = false
	whitespaceRE, err := regexp.Compile("[\\s]+")
	check(err)
	tagRE, err := regexp.Compile("\\([\\dA-F]{4}\\,[\\dA-F]{4}\\)")
	check(err)
	length := 0
	var x []core.DictEntry
	mode := -1
	index := -1
	for {
		token, err := decoder.Token()
		if token == nil {
			break
		}
		check(err)
		switch element := token.(type) {
		case xml.CharData:
			elementString := strings.Replace(string(element), "\u200b", "", -1)
			allblank := whitespaceRE.Match([]byte(elementString))
			if !allblank {
				if tagRE.Match([]byte(element)) {
					mode = 1
					index++
				}
				switch mode {
				case 1:
					x = append(x, core.DictEntry{})
					tagString := elementString[1:][:9]
					tagString = strings.Replace(tagString, ",", "", 1)
					tagInt, err := strconv.ParseUint(tagString, 16, 32)
					check(err)
					x[index].Tag = core.Tag(tagInt)
					x[index].Retired = false
				case 2:
					x[index].NameHuman = strings.Replace(fmt.Sprintf("%s", token), "\u200b", " ", -1)
					x[index].Name = elementString
				case 3:
					x[index].VR = elementString
				case 5:
					x[index].Retired = true
				}
				mode++
			}
		}
		length++
	}
	return x
}

func ParseFileMetaElements(buf []byte) []core.DictEntry {
	var x []core.DictEntry
	decoder := xml.NewDecoder(strings.NewReader(string(buf)))
	decoder.Strict = false

	for {
		token, err := decoder.Token()
		if token == nil {
			break
		}
		check(err)
		switch element := token.(type) {
		case xml.CharData:
			elementString := strings.Replace(string(element), "\u200b", "", -1)
			log.Println(elementString)
		}
	}
	return x
}

/*
	Generates a DICOM data dictionary file from XML
*/
func main() {
	xmlfile := os.Args[1]
	f, err := os.Open(xmlfile)
	check(err)
	stat, err := f.Stat()
	check(err)
	buf := make([]byte, stat.Size())
	_, err = f.Read(buf)
	check(err)

	posStart := strings.Index(string(buf), "<tbody>")
	if posStart <= 0 {
		panic("Could not find start of data dictionary inside XML")
	}
	dataElementsBuffer := buf[posStart+7:]

	posEnd := strings.Index(string(buf), "</tbody>")
	if posEnd <= 0 {
		panic("Could not find end of data dictionary inside XML")
	}

	dataElementsBuffer = dataElementsBuffer[:posEnd-8]
	x := ParseDataElements(dataElementsBuffer)
	log.Printf("Found %d items\n", len(x))

	fileMetaElementsBuffer := buf[posEnd:]

	posFMEStart := strings.Index(string(fileMetaElementsBuffer), "<tbody>")
	if posFMEStart <= 0 {
		panic("Could not find start of file meta elements inside XML")
	}
	fileMetaElementsBuffer = fileMetaElementsBuffer[posFMEStart+7:]

	posFMEEnd := strings.Index(string(fileMetaElementsBuffer), "</tbody>")
	if posFMEEnd <= 0 {
		panic("Could not find end of file meta elements inside XML")
	}

	fileMetaElementsBuffer = fileMetaElementsBuffer[:posFMEEnd-8]

	y := ParseDataElements(fileMetaElementsBuffer)
	log.Printf("Found %d file meta elements\n", len(y))

	outF, err := os.Create("../../core/datadict.go")
	check(err)
	outCode := `// Code generated using util:gendatadict. DO NOT EDIT.

package core

// DicomDictionary provides a mapping between uint32 representation of a DICOM Tag and a DictEntry pointer.
var DicomDictionary = map[uint32]*DictEntry{
`
	for _, v := range y {
		outCode += fmt.Sprintf(`    0x%08X: &DictEntry{Tag: 0x%08X, Name: "%s", NameHuman: "%s", VR: "%s", Retired: %v},`, uint32(v.Tag), uint32(v.Tag), v.Name, v.NameHuman, v.VR, v.Retired) + "\n"
	}
	for _, v := range x {
		outCode += fmt.Sprintf(`    0x%08X: &DictEntry{Tag: 0x%08X, Name: "%s", NameHuman: "%s", VR: "%s", Retired: %v},`, uint32(v.Tag), uint32(v.Tag), v.Name, v.NameHuman, v.VR, v.Retired) + "\n"
	}
	outCode += `}
`
	_, err = outF.WriteString(outCode)
	check(err)
	log.Printf("Wrote file OK.")
}
