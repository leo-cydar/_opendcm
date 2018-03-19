package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/b71729/opendcm" // yes, dot imports are discouraged, but otherwise prefixing everything is a pain in the arse
	"github.com/b71729/opendcm/dictionary"
)

var baseFile = filepath.Base(os.Args[0])

func check(err error) {
	if err != nil {
		FatalfDepth(3, "error: %v", err)
	}
}

func usage() {
	fmt.Printf("OpenDCM version %s\n", OpenDCMVersion)
	fmt.Printf("usage: %s [%s] [flags]\n", baseFile, strings.Join([]string{"view", "reduce", "gendatadict", "createdicom", "simulate"}, " / "))
	os.Exit(1)
}

func main() {
	GetConfig()
	if len(os.Args) == 1 || (os.Args[1] == "--help" || os.Args[1] == "-h") {
		usage()
	}
	cmd := os.Args[1]
	switch cmd {
	case "view":
		StartViewDicom()
	case "reduce":
		StartReduce()
	case "simulate":
		StartSimulate()
	case "gendatadict":
		StartGenDataDict()
	case "createdicom":
		StartCreateDicom()
	default:
		usage()
	}
	return
}

/*
===============================================================================
    Mode: Simulate Load Over Time
===============================================================================
*/

// StartSimulate simulates load over time
func StartSimulate() {
	var files []string
	ConcurrentlyWalkDir(os.Args[2], func(file string) {
		files = append(files, file)
	})
	flen := len(files)
	ntotal := 0
	start := time.Now()
	go func() {
		for {
			time.Sleep(time.Second * 3)

			elapsed := time.Now().Sub(start)
			Debugf("running... apd=%v, dps=%v", math.Round(float64(Nalloc)/elapsed.Seconds()), math.Round(float64(ntotal)/elapsed.Seconds()))

			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			Debugf("memory: %d kB / %d kB", memStats.Alloc/1024, memStats.Sys/1024)
		}
	}()
	for {
		n := rand.Intn(flen)
		ParseDicom(files[n])
		ntotal++
	}
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
				Warnf(`using "UN" as VR instead of "%s" for tag "%s"`, token, elements[index].Tag)
			}
		case 5:
			orIndex := strings.Index(token, " or")
			if orIndex > -1 {
				token = token[:orIndex]
			}
			if !acceptibleVM.Match([]byte(token)) {
				Warnf(`using "n" as VM instead of "%s" for tag "%s"`, token, elements[index].Tag)
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

// StartGenDataDict generates data dictionary
func StartGenDataDict() {
	if len(os.Args) != 3 {
		fmt.Printf("OpenDCM version %s\n", OpenDCMVersion)
		fmt.Printf("usage: %s gendatadict dictFromNEMA.xml", baseFile)
		os.Exit(1)
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
	Infof("found %d data elements", len(dataElements))

	// file meta elements
	data = data[posEnd+8:]
	posStart, posEnd, err = tableBodyPosition(data)
	check(err)

	fileMetaElements := parseDataElements(data[posStart+7 : posEnd])
	Infof("found %d file meta elements", len(fileMetaElements))

	// directory structure elements
	data = data[posEnd+8:]
	posStart, posEnd, err = tableBodyPosition(data)
	check(err)

	dirStructElements := parseDataElements(data[posStart+7 : posEnd])
	Infof("found %d directory structure elements", len(dirStructElements))

	// UIDs
	data = data[posEnd+8:]
	posStart, posEnd, err = tableBodyPosition(data)
	check(err)

	UIDs := parseUIDs(data[posStart+7 : posEnd])
	Infof("found %d unique identifiers (UIDs)", len(UIDs))

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
	outCode += "	// File Meta Elements\n"
	for _, v := range fileMetaElements {
		outCode += fmt.Sprintf(`	0x%08X: {Tag: 0x%08X, Name: "%s", NameHuman: "%s", VR: "%s", Retired: %v},`, uint32(v.Tag), uint32(v.Tag), v.Name, v.NameHuman, v.VR, v.Retired) + "\n"
	}

	outCode += "	// Directory Structure Elements\n"
	for _, v := range dirStructElements {
		outCode += fmt.Sprintf(`	0x%08X: {Tag: 0x%08X, Name: "%s", NameHuman: "%s", VR: "%s", VM: "%s", Retired: %v},`, uint32(v.Tag), uint32(v.Tag), v.Name, v.NameHuman, v.VR, v.VM, v.Retired) + "\n"
	}

	outCode += "	// Data Elements\n"
	for _, v := range dataElements {
		outCode += fmt.Sprintf(`	0x%08X: {Tag: 0x%08X, Name: "%s", NameHuman: "%s", VR: "%s", VM: "%s", Retired: %v},`, uint32(v.Tag), uint32(v.Tag), v.Name, v.NameHuman, v.VR, v.VM, v.Retired) + "\n"
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
	Info(`wrote dictionary to "datadict.go"`)
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

	if length > 0 && length < 0xFFFFFFFF {
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
	if length == 0xFFFFFFFF {
		ret = append(ret, []byte{
			0xFE, 0xFF, 0xDD, 0xE0, // 4b: sequence end tag
			0x00, 0x00, 0x00, 0x00, // 4b: filler
		}...)
	}
	return ret, nil
}

// TODO: move to common
func elementFromBuffer(buf []byte) (Element, error) {
	r := bufio.NewReader(bytes.NewReader(buf))
	es := NewElementStream(r, int64(len(buf)))
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
	randUID, err := NewRandInstanceUID()
	check(err)
	elementBytes, err = generateElement("0002,0003", []byte(randUID), "UI")
	check(err)
	buffer = append(buffer, elementBytes...)

	// 0002,0010 Transfer Syntax UID
	elementBytes, err = generateElement("0002,0010", []byte("1.2.840.10008.1.2.1"), "UI")
	check(err)
	buffer = append(buffer, elementBytes...)

	// 0002,0012 Implementation Class UID
	elementBytes, err = generateElement("0002,0012", []byte(GetImplementationUID(true)), "UI")
	check(err)
	buffer = append(buffer, elementBytes...)

	// (0002,0013)    Implementation Version Name    opendcm-0.1
	elementBytes, err = generateElement("0002,0013", []byte(fmt.Sprintf("opendcm-%s", OpenDCMVersion)), "SH")
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
		fmt.Printf("OpenDCM version %s\n", OpenDCMVersion)
		fmt.Printf("usage: %s createdicom out_file", baseFile)
		os.Exit(1)
	}
	outFileName := os.Args[2]
	if _, err := os.Stat(outFileName); err == nil {
		Fatalf(`file "%s" already exists`, outFileName)
	}

	buffer := writeMeta()

	// write output
	f, err := os.Create(outFileName)
	check(err)
	nwrite, err := f.Write(buffer)
	check(err)
	if nwrite != len(buffer) {
		Fatalf("could not write all meta elements to disk. nwrite=%d bytes, size=%d bytes", nwrite, len(buffer))
	}

	Info("wrote meta information to disk")

	elementBuffer := make([]byte, 0)

	/// VRs with defined length
	// AE
	elementBytes, err := generateElement("0072,005E", []byte("AENAME"), "AE")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// AS
	elementBytes, err = generateElement("0072,005F", []byte("012Y"), "AS")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// AT
	elementBytes, err = generateElement("0072,0060", []byte{0x42, 0x24, 0x01, 0x90}, "AT")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// CS
	elementBytes, err = generateElement("0072,0062", []byte("CODESTRING_1"), "CS")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// DA
	elementBytes, err = generateElement("0072,0061", []byte("20180317"), "DA")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// DS
	elementBytes, err = generateElement("0072,0072", []byte("360.8"), "DS")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// DT
	elementBytes, err = generateElement("0072,0063", []byte("200508101215"), "DT")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// FL
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, math.Float32bits(127.50812))
	elementBytes, err = generateElement("0072,0076", buf, "FL")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// FD
	buf = make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, math.Float64bits(123456.123456789))
	elementBytes, err = generateElement("0072,0074", buf, "FD")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// IS
	elementBytes, err = generateElement("0072,0064", []byte("0123456789"), "IS")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// LO
	elementBytes, err = generateElement("0072,0066", []byte(`Long String`), "LO")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// LT
	elementBytes, err = generateElement("0072,0068", []byte(`Long\Text\No\Split`), "LT")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// OB
	elementBytes, err = generateElement("0072,0065", []byte{0x01, 0x02, 0x03, 0x04}, "OB")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// OB of undefined length
	buf = genItemBytesRaw([]byte{0x01, 0x02, 0x03, 0x04}, 4)
	elementBytes, err = generateElementWithLength("7FE0,0010", buf, "OB", 0xFFFFFFFF)
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// OD
	buf = make([]byte, 16)
	binary.LittleEndian.PutUint64(buf[0:], 888888887)
	binary.LittleEndian.PutUint64(buf[8:], 777777778)
	elementBytes, err = generateElement("0072,0073", buf, "OD")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// OF
	buf = make([]byte, 8)
	binary.LittleEndian.PutUint32(buf[0:], math.Float32bits(123.4))
	binary.LittleEndian.PutUint32(buf[4:], math.Float32bits(567.8))
	elementBytes, err = generateElement("0072,0067", buf, "OF")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// OW
	buf = make([]byte, 16)
	binary.LittleEndian.PutUint32(buf[0:], 4321)
	binary.LittleEndian.PutUint32(buf[4:], 8765)
	binary.LittleEndian.PutUint32(buf[8:], 2109)
	binary.LittleEndian.PutUint32(buf[12:], 6543)
	elementBytes, err = generateElement("0072,0069", buf, "OW")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// PN
	elementBytes, err = generateElement("0072,006A", []byte(`Anderson^Leo`), "PN")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// SH
	elementBytes, err = generateElement("0072,006C", []byte(`Short String`), "SH")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// SL
	buf = make([]byte, 4)
	v := int32(-1234)
	binary.LittleEndian.PutUint32(buf[0:], uint32(v))
	elementBytes, err = generateElement("0072,007C", buf, "SL")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// SQ
	buf = make([]byte, 0)

	// SQ Encoding 5.12.1: undefined-len SQ with defined-len items
	asBytes := genItemBytes("0072,005F", []byte("012Y"), "AS", 4)
	stBytes := genItemBytes("0072,006E", []byte(`Unlimited\Text`), "UT", 14)
	buf = append(buf, asBytes...)
	buf = append(buf, stBytes...)
	elementBytes, err = generateElementWithLength("0072,0080", buf, "SQ", 0xFFFFFFFF)
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// SQ Encoding 5.12.3: undefined-len SQ with undefined-len items
	nestedAS := genItemBytes("0072,005F", []byte("012Y"), "AS", 4)
	sequenceItem := genItemBytes("0072,0080", nestedAS, "SQ", 0xFFFFFFFF)
	for i := 0; i < 4; i++ {
		sequenceItem = genItemBytes("0072,0080", sequenceItem, "SQ", 0xFFFFFFFF)
	}

	elementBytes, err = generateElementWithLength("0008,9121", sequenceItem, "SQ", 0xFFFFFFFF)
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// SS
	buf = make([]byte, 2)
	v2 := int16(-1234)
	binary.LittleEndian.PutUint16(buf[0:], uint16(v2))
	elementBytes, err = generateElement("0072,007E", buf, "SS")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// ST
	elementBytes, err = generateElement("0072,006E", []byte(`Short\Text\No\Split`), "ST")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// TM
	elementBytes, err = generateElement("0072,006B", []byte(`121530.35`), "TM")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// UI
	elementBytes, err = generateElement("0072,007F", []byte(`127.0.0.1`), "UI")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// UL
	buf = make([]byte, 4)
	v3 := uint32(123456789)
	binary.LittleEndian.PutUint32(buf[0:], v3)
	elementBytes, err = generateElement("0072,0078", buf, "UL")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// UN
	elementBytes, err = generateElement("0072,006D", []byte("UnknownData"), "UN")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// US
	buf = make([]byte, 2)
	v4 := uint16(12345)
	binary.LittleEndian.PutUint16(buf[0:], v4)
	elementBytes, err = generateElement("0072,007A", buf, "US")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// UT
	elementBytes, err = generateElement("0072,0070", []byte(`Unlimited\Text\No\Split`), "UT")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	nwrite, err = f.Write(elementBuffer)
	check(err)
	if nwrite != len(elementBuffer) {
		Fatalf("could not write all elements to disk. nwrite=%d bytes, size=%d bytes", nwrite, len(elementBuffer))
	}

	Info("wrote elements to disk")

	defer f.Close()
}

func genItemBytesRaw(value []byte, length uint32) []byte {
	outBytes := []byte{0xFE, 0xFF, 0x00, 0xE0}
	outBytes = append(outBytes, make([]byte, 4)...)
	binary.LittleEndian.PutUint32(outBytes[4:], length)
	outBytes = append(outBytes, value...)
	if length == 0xFFFFFFFF {
		outBytes = append(outBytes, []byte{
			0xFE, 0xFF, 0x0D, 0xE0, // 4b: item #1 end tag
			0x00, 0x00, 0x00, 0x00, // 4b: filler
		}...)
	}
	return outBytes
}

func genItemBytes(tagString string, value []byte, VR string, length uint32) []byte {
	el, err := generateElementWithLength(tagString, value, VR, length)
	if err != nil {
		panic(err)
	}
	outBytes := genItemBytesRaw(el, length)
	return outBytes

}

/*
===============================================================================
    Mode: Reduce DICOM Directory
===============================================================================
*/

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

// StartReduce enters "reduce" directory mode.
// This scans the input directory for unique dicoms (unique SeriesInstanceUID) and copies those dicoms
//   to the output directory.
func StartReduce() {
	if len(os.Args) != 4 {
		fmt.Printf("OpenDCM version %s\n", OpenDCMVersion)
		fmt.Printf("usage: %s reduce in_dir out_dir\n", baseFile)
		os.Exit(1)
	}
	dirIn := os.Args[2]
	dirOut := os.Args[3]

	statIn, err := os.Stat(dirIn)
	check(err)
	if !statIn.IsDir() {
		Fatalf(`"%s" is not a directory. please provide a directory`, dirIn)
	}

	statOut, err := os.Stat(dirOut)
	check(err)
	if !statOut.IsDir() {
		Fatalf(`"%s" is not a directory. please provide a directory`, dirOut)
	}

	seriesInstanceUIDs := make(map[string]bool, 0)
	ConcurrentlyWalkDir(dirIn, func(filePath string) {
		dcm, err := ParseDicom(filePath)
		check(err)
		if e, found := dcm.GetElement(0x0020000E); found {
			if val, ok := e.Value().(string); ok {
				_, found := seriesInstanceUIDs[val]
				if !found {
					Infof("found unique: %s", val)
					seriesInstanceUIDs[val] = true
					outputFilePath := filepath.Join(dirOut, fmt.Sprintf("%s.dcm", val))
					if _, err := os.Stat(outputFilePath); os.IsNotExist(err) {
						// file does not exist - lets create it
						err := copy(dcm.FilePath, outputFilePath)
						check(err)
					} else {
						Infof(`skip "%s": file exists`, outputFilePath)
					}
				}
			}
		}
	})
}

/*
===============================================================================
    Mode: View DICOM File
===============================================================================
*/

// StartViewDicom enters "view" mode.
// This allows for viewing of a dicom file (listing of its elements and their values)
func StartViewDicom() {
	if len(os.Args) != 3 {
		fmt.Printf("OpenDCM version %s\n", OpenDCMVersion)
		fmt.Printf("usage: %s view file_or_dir\n", baseFile)
		os.Exit(1)
	}
	stat, err := os.Stat(os.Args[2])
	check(err)
	if isDir := stat.IsDir(); !isDir {
		dcm, err := ParseDicom(os.Args[2])
		check(err)
		var elements []Element
		for _, v := range dcm.Elements {
			elements = append(elements, v)
		}
		sort.Sort(ByTag(elements))
		for _, element := range elements {
			description := element.Describe(0)
			for _, line := range description {
				fmt.Println(line)
			}
		}
	} else {
		errorCount := 0
		successCount := 0
		err := ConcurrentlyWalkDir(os.Args[2], func(path string) {
			_, err := ParseDicom(path)
			basePath := filepath.Base(path)
			if err != nil {
				Errorf(`error parsing "%s": %v`, basePath, err)
				errorCount++
				return
			}
			successCount++
			Debugf(`parsed "%s"`, basePath)
		})
		check(err)
		if errorCount == 0 {
			Infof("parsed %d files without errors", successCount)
		} else {
			Infof("parsed %d files without errors, and failed to parse %d files", successCount, errorCount)
		}
	}
}
