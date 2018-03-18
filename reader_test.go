package opendcm

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/b71729/opendcm/dictionary"
)

/*
===============================================================================
    Utilities
===============================================================================
*/

// array of bytes representing a valid "SQ" VR element
var validSequenceElementBytes = []byte{0x32, 0x00, 0x64, 0x10, 0x53, 0x51, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFE, 0xFF, 0x00, 0xE0, 0xFF, 0xFF, 0xFF, 0xFF, 0x08, 0x00, 0x00, 0x01, 0x53, 0x48, 0x0E, 0x00, 0x53, 0x4E, 0x47, 0x30, 0x41, 0x47, 0x2F, 0x5A, 0x54, 0x58, 0x30, 0x58, 0x42, 0x20, 0x08, 0x00, 0x02, 0x01, 0x53, 0x48, 0x0A, 0x00, 0x53, 0x45, 0x43, 0x54, 0x52, 0x41, 0x20, 0x52, 0x49, 0x53, 0x08, 0x00, 0x03, 0x01, 0x53, 0x48, 0x04, 0x00, 0x31, 0x2E, 0x30, 0x20, 0x08, 0x00, 0x04, 0x01, 0x4C, 0x4F, 0x0A, 0x00, 0x4D, 0x52, 0x20, 0x4B, 0x6E, 0x65, 0x20, 0x73, 0x69, 0x6E, 0xFE, 0xFF, 0x0D, 0xE0, 0x00, 0x00, 0x00, 0x00, 0xFE, 0xFF, 0xDD, 0xE0, 0x00, 0x00, 0x00, 0x00}

// array of bytes representing a valid "CS" VR element
var validCSElementBytes = []byte{0x28, 0x00, 0x04, 0x00, 0x43, 0x53, 0x0C, 0x00, 0x4D, 0x4F, 0x4E, 0x4F, 0x43, 0x48, 0x52, 0x4F, 0x4D, 0x45, 0x32, 0x20}

var validUNElementULBytes = []byte{0x28, 0x00, 0x04, 0x00, 0x55, 0x4E, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFE, 0xFF, 0x00, 0xE0, 0x02, 0x00, 0x00, 0x00, 0x30, 0x30, 0xFE, 0xFF, 0xDD, 0xE0, 0x00, 0x00, 0x00, 0x00}

var cfg = GetConfig()

// shorthand for parsing an Element from a byte array
func elementFromBuffer(buf []byte) (Element, error) {
	return elementStreamFromBuffer(buf).GetElement()
}

// shorthand for constructing an ElementStream from a byte array
func elementStreamFromBuffer(buf []byte) *ElementStream {
	es := NewElementStream(bufio.NewReader(bytes.NewReader(buf)), int64(len(buf)))
	return &es
}

// valueTypematchesVR performs a quick sanity check as to whether `v` is of expected (type) for the given `vr`
func valueTypeMatchesVR(vr string, v interface{}) bool {
	switch vr {
	case "AE", "AS", "CS", "DA", "DS", "DT", "IS", "LO", "PN", "SH", "TM", "UI": // LT, ST, UT do not support multiVM
		switch v.(type) {
		case []string:
		case string:
		case nil:
		default:
			return false
		}
	case "FL":
		switch v.(type) {
		case []float32:
		case float32:
		case nil:
		default:
			return false
		}
	case "FD":
		switch v.(type) {
		case []float64:
		case float64:
		case nil:
		default:
			return false
		}
	case "SS":
		switch v.(type) {
		case []int16:
		case int16:
		case nil:
		default:
			return false
		}
	case "SL":
		switch v.(type) {
		case []int32:
		case int32:
		case nil:
		default:
			return false
		}
	case "US":
		switch v.(type) {
		case []uint16:
		case uint16:
		case nil:
		default:
			return false
		}
	case "UL":
		switch v.(type) {
		case []uint32:
		case uint32:
		case nil:
		default:
			return false
		}
	}
	return true

}

/*
===============================================================================
    File Parsing: Valid DICOMs
===============================================================================
*/

// TestParseValidFiles tests that, given a valid DICOM input, the parser will correctly parse embedded elements
func TestParseValidFiles(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path        string
		numElements int
	}{
		{
			path:        filepath.Join("testdata", "TCIA", "1.3.6.1.4.1.14519.5.2.1.2744.7002.251446451370536632612663178782.dcm"),
			numElements: 105,
		},
		{
			path:        filepath.Join("testdata", "synthetic", "VRTest.dcm"),
			numElements: 37,
		},
	}
	for _, testCase := range cases {
		dcm, err := ParseDicom(testCase.path)
		if err != nil {
			t.Fatalf("%s: error: %v", testCase.path, err)
		}
		// should have found all elements
		if l := len(dcm.Elements); l != testCase.numElements {
			t.Fatalf("%s: number of elements = %d (!= %d)", testCase.path, l, testCase.numElements)
		}
		stat, err := os.Stat(testCase.path)
		if err != nil {
			t.Fatalf("%s: error: %v", testCase.path, err)
		}
		// should be at end of file
		if pos := dcm.elementStream.GetPosition(); pos != stat.Size() {
			t.Fatalf("%s: reader position = %d (!= %d)", testCase.path, pos, stat.Size())
		}

		// check all elements values match correct type for their VR:
		for _, e := range dcm.Elements {
			val := e.Value()
			if !valueTypeMatchesVR(e.VR, val) {
				t.Fatalf(`%s: type "%s" for element %s is incorrect (VR="%s")`, testCase.path, reflect.TypeOf(val), e.Tag, e.VR)
			}
		}
	}
}

// TestIssue6 attempts to parse a valid file, with a source VR of UN that is matches as non-UN in our dictionary.
func TestIssue6(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "TCIA", "1.3.12.2.1107.5.1.4.1001.30000013072513125762500009613.dcm")
	dcm, err := ParseDicom(path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	// 0028,0107 (LargestImagePixelValue) has VR of US, with value 2766. But source DICOM specifies it as UN.
	// Check that the correct value has been parsed to prevent regression
	e, found := dcm.GetElement(0x00280107)
	if !found {
		t.Fatalf("could not find (0028,0107) in file")
	}

	val := e.Value()
	if !valueTypeMatchesVR(e.VR, val) {
		t.Fatalf(`type "%s" for element %s is incorrect (VR="%s")`, reflect.TypeOf(val), e.Tag, e.VR)
	}

	if val.(uint16) != 2766 {
		t.Fatalf("(0028,0107) returned %d (!= 2766)", val.(uint16))
	}
}

// TestParseFileWIthZeroElementLength tests that the parser can accept a file containing an embedded
// element with a defined length of zero
func TestParseFileWithZeroElementLength(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "synthetic", "ZeroElementLength.dcm")
	dcm, err := ParseDicom(path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	e, found := dcm.GetElement(0x00080005)
	if !found {
		t.Fatalf("could not find (0008,0005) in file")
	}

	if e.ValueLength != 0 {
		t.Fatalf("(0008,0005) has value length of %d (!= 0)", e.ValueLength)
	}
}

func TestParseFileWithMissingMetaLength(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "synthetic", "MissingMetaLength.dcm")
	dcm, err := ParseDicom(path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if dcm.TotalMetaBytes != 340 {
		t.Fatalf("meta length = %d (!= 340)", dcm.TotalMetaBytes)
	}
	e, found := dcm.GetElement(0x7FE00010)
	if !found {
		t.Fatalf("could not find (7FE0,0010) in file")
	}

	if e.ValueLength != 4 {
		t.Fatalf("(7FE0,0010) has value length of %d (!= 4)", e.ValueLength)
	}
}

func TestParseFileWithMissingTransferSyntax(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "synthetic", "MissingTransferSyntax.dcm")
	dcm, err := ParseDicom(path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if dcm.TotalMetaBytes != 324 {
		t.Fatalf("meta length = %d (!= 324)", dcm.TotalMetaBytes)
	}
	// transfer syntax should be the default (ImplicitVR, LittleEndian)
	if dcm.elementStream.TransferSyntax.UIDEntry.UID != "1.2.840.10008.1.2" {
		t.Fatalf(`missing transfer syntax should have defaulted to "1.2.840.10008.1.2" (got "%s")`, dcm.elementStream.TransferSyntax.UIDEntry.UID)
	}
	e, found := dcm.GetElement(0x7FE00010)
	if !found {
		t.Fatalf("could not find (7FE0,0010) in file")
	}
	if e.ValueLength != 4 {
		t.Fatalf("(7FE0,0010) has value length of %d (!= 4)", e.ValueLength)
	}
	if len(dcm.Elements) != 8 {
		t.Fatalf("found %d elements (!= 8)", len(dcm.Elements))
	}
}

func TestParseFileWithMissingPreambleMagic(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "synthetic", "MissingPreambleMagic.dcm")
	dcm, err := ParseDicom(path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if dcm.TotalMetaBytes != 192 {
		t.Fatalf("meta length = %d (!= 192)", dcm.TotalMetaBytes)
	}
	// transfer syntax should be the default (ImplicitVR, LittleEndian)
	if dcm.elementStream.TransferSyntax.UIDEntry.UID != "1.2.840.10008.1.2" {
		t.Fatalf(`missing transfer syntax should have defaulted to "1.2.840.10008.1.2" (got "%s")`, dcm.elementStream.TransferSyntax.UIDEntry.UID)
	}
	e, found := dcm.GetElement(0x7FE00010)
	if !found {
		t.Fatalf("could not find (7FE0,0010) in file")
	}
	if e.ValueLength != 4 {
		t.Fatalf("(7FE0,0010) has value length of %d (!= 4)", e.ValueLength)
	}
	if len(dcm.Elements) != 8 {
		t.Fatalf("found %d elements (!= 8)", len(dcm.Elements))
	}
}

/*
===============================================================================
    File Parsing: Invalid DICOMs
===============================================================================
*/
//TestParsecorruptDicoms tests that, given corrupted inputs, the parser will fail in an controlled manner
func TestParseCorruptDicoms(t *testing.T) {
	t.Parallel()
	// attempt to parse corrupt dicoms
	corruptFiles := []string{
		"CorruptBadTransferSyntax.dcm",
	}
	for _, file := range corruptFiles {
		path := filepath.Join("testdata", "synthetic", file)
		_, err := ParseDicom(path)
		switch err.(type) {
		case *CorruptDicom:
		default:
			t.Fatalf(`parsing corrupt dicom "%s" should have raised a CorruptDicomError (got %v)`, path, err)
		}
	}
}

/*
===============================================================================
    Strict Mode Tests
===============================================================================
*/

func TestStrictModeEnabled(t *testing.T) {
	// in strict mode, certain dodgy inputs should be rejected
	cfg.StrictMode = true
	OverrideConfig(cfg)
	testCases := []string{
		filepath.Join("testdata", "synthetic", "CorruptOverflowElementLength.dcm"),
		filepath.Join("testdata", "synthetic", "MissingPreambleMagic.dcm"),
	}
	for _, testCase := range testCases {
		_, err := ParseDicom(testCase)
		switch err.(type) {
		case *CorruptDicom:
		default:
			t.Fatalf(`with "StrictMode" disabled, parsing "%s" should have raised a CorruptDicomError (got %v)`, testCase, err)
		}
	}
}
func TestStrictModeDisabled(t *testing.T) {
	// in non strict mode, inputs with elements exceeding remaining file size should not be rejected,
	// and should have their length adjusted.
	cfg.StrictMode = false
	OverrideConfig(cfg)
	testCases := []string{
		filepath.Join("testdata", "synthetic", "CorruptOverflowElementLength.dcm"),
		filepath.Join("testdata", "synthetic", "MissingPreambleMagic.dcm"),
	}
	for _, testCase := range testCases {
		_, err := ParseDicom(testCase)
		if err != nil {
			t.Fatalf(`with "StrictMode" disabled, parsing "%s" should not have raised an error (got %v)`, testCase, err)
		}
	}
}

func TestGetElementWithInsufficientBytes(t *testing.T) {
	t.Parallel()
	testCases := [][]byte{
		{},                                               // cannot read lower section of tag
		{0x00, 0x00},                                     // cannot read upper section of tag
		{0x00, 0x00, 0x00, 0x00},                         // cannot read VR
		{0x00, 0x08, 0x00, 0x02, 0x55, 0x54},             // 0800,0200,UT
		{0x00, 0x08, 0x00, 0x02, 0x55, 0x54, 0x00, 0x00}, // 0800,0200,UT,{0x00,0x00}
		{0x00, 0x08, 0x00, 0x02, 0x43, 0x53},             // 0800,0200,CS
	}
	for _, buf := range testCases {
		_, err := elementFromBuffer(buf)
		if errValue, ok := err.(*CorruptElement); ok {
			if !strings.Contains(errValue.Error(), "would exceed") {
				t.Fatalf(`should have contained detail containing "would exceed" (got %s)`, errValue.Error())
			}
		} else {
			t.Fatalf(`should have raised a CorruptElementError (got %v)`, err)
		}
	}
}

func TestImplicitVRVRLengthMissing(t *testing.T) {
	t.Parallel()
	buf := []byte{0x00, 0x08, 0x00, 0x02} // 0800,0200,UT
	es := elementStreamFromBuffer(buf)
	es.SetTransferSyntax("1.2.840.10008.1.2") // ImplicitVR
	_, err := es.GetElement()
	if errValue, ok := err.(*CorruptElement); ok {
		if !strings.Contains(errValue.Error(), "would exceed") {
			t.Fatalf(`should have contained detail containing "would exceed" (got %s)`, errValue.Error())
		}
	} else {
		t.Fatalf(`should have raised a CorruptElementError (got %v)`, err)
	}
}

/*
===============================================================================
    Element Parsing: VRs
===============================================================================
*/

func TestDecodeBytesEmptyCharset(t *testing.T) {
	t.Parallel()
	// should return string([]byte), so utf-8
	val, err := decodeBytes([]byte("parser"), nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if val != "parser" {
		t.Fatalf(`got "%s" (!= "parser")`, val)
	}
}

func TestIsCharacterStringVR(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"AE", "AS", "CS", "DA", "DS", "DT", "IS", "LO", "LT", "PN", "SH", "ST", "TM", "UI", "UT"} {
		if !IsCharacterStringVR(v) {
			t.Fatalf(`VR "%s" should be character string VR`, v)
		}
	}
	for _, v := range []string{"OB", "FL"} {
		if IsCharacterStringVR(v) {
			t.Fatalf(`VR "%s" should not be character string VR`, v)
		}
	}
}

func TestSplitBinaryVM(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		input    []byte
		nsplit   int
		expected [][]byte
	}{
		{
			input:    []byte{0xAB, 0x7F, 0x23, 0x42},
			nsplit:   2,
			expected: [][]byte{{0xAB, 0x7F}, {0x23, 0x42}},
		},
		{
			input:    []byte{0xFF, 0xFE, 0x09},
			nsplit:   1,
			expected: [][]byte{{0xFF}, {0xFE}, {0x09}},
		},
	}
	for _, testCase := range testCases {
		val := splitBinaryVM(testCase.input, testCase.nsplit)
		if len(val) != len(testCase.expected) {
			t.Fatalf("got %d splits (!= %d)", len(val), len(testCase.expected))
		}
	}
}

func TestSplitCharacterStringVM(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		input    []byte
		expected [][]byte
	}{
		{
			input:    []byte(`dicom\parse\9000`),
			expected: [][]byte{[]byte(`dicom`), []byte(`parse`), []byte(`9000`)},
		},
		{
			input:    []byte(`中文\名字\`),
			expected: [][]byte{[]byte(`中文`), []byte(`名字`), nil},
		},
		{
			input:    []byte(`\\中文\名字`),
			expected: [][]byte{nil, nil, []byte(`中文`), []byte(`名字`)},
		},
	}
	for _, testCase := range testCases {
		val := splitCharacterStringVM(testCase.input)
		if len(val) != len(testCase.expected) {
			t.Fatalf("got %d splits (!= %d)", len(val), len(testCase.expected))
		}
		for i, split := range val {
			if bytes.Compare(split, testCase.expected[i]) != 0 {
				t.Fatalf(`got "%v" (!= "%v")`, split, testCase.expected[i])
			}
		}
	}
}

func TestTagSorting(t *testing.T) {
	t.Parallel()
	elements := []Element{
		{DictEntry: &dictionary.DictEntry{Tag: 0x00100020}},
		{DictEntry: &dictionary.DictEntry{Tag: 0x0020008F}},
		{DictEntry: &dictionary.DictEntry{Tag: 0x0001FFFF}},
	}
	sort.Sort(ByTag(elements))
	if elements[0].Tag != 0x0001FFFF {
		t.Fatalf(`sort failed`)
	}
	if elements[1].Tag != 0x00100020 {
		t.Fatalf(`sort failed`)
	}
	if elements[2].Tag != 0x0020008F {
		t.Fatalf(`sort failed`)
	}
}

func TestDescribe(t *testing.T) {
	t.Parallel()
	// describe a SQ element, which contains nested elements for optimal coverage
	lookup, _ := LookupTag(0x00081130) // SQ
	lookupSub, _ := LookupTag(0x001811A2)
	element := Element{DictEntry: lookup}
	element.Items = []Item{
		{Elements: map[uint32]Element{
			0x001811A2: {DictEntry: lookupSub, value: []byte("10"), ValueLength: 2},
		}},
	}
	description := element.Describe(0)
	if len(description) != 2 {
		t.Fatalf("got %d (!= 2) for SQ", len(description))
	}
	// now describe empty SQ
	element = Element{DictEntry: lookup}
	description = element.Describe(0)
	if len(description) != 1 {
		t.Fatalf("got %d (!= 1) for SQ", len(description))
	}
	// now describe Element with undefined length
	element, err := elementFromBuffer(validUNElementULBytes)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	description = element.Describe(0)
	if !strings.Contains(description[1], "2 bytes") {
		t.Fatal(`"2 bytes" not found in description`)
	}

	// now describe Element with > 256 bytes length
	// should not actually attempt to display contents
	element, err = elementFromBuffer(validCSElementBytes)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	element.ValueLength = 1024
	description = element.Describe(0)
	if !strings.Contains(description[0], "1024 bytes") {
		t.Fatal(`"1024 bytes" not found in description`)
	}
}

func TestSupportsMultiVM(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		VM       string
		supports bool
	}{
		{supports: false, VM: "1"},
		{supports: false, VM: "0"},
		{supports: false, VM: "1-1"},
		{supports: true, VM: "1-n"},
		{supports: true, VM: "1-8"},
	}
	for _, testCase := range testCases {
		element := Element{DictEntry: &dictionary.DictEntry{VM: testCase.VM}}
		supports := element.SupportsMultiVM()
		if testCase.supports != supports {
			t.Fail()
		}
	}
}

// TestParseCS attempts to parse CS
func TestParseCS(t *testing.T) {
	t.Parallel()
	element, err := elementFromBuffer(validCSElementBytes)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// tag should match
	if tag := element.Tag; tag != 0x00280004 {
		t.Fatalf("tag = %08X (!= 0x00280004)", uint32(tag))
	}
	// VR should match
	if vr := element.VR; vr != "CS" {
		t.Fatalf(`VR = "%s" (!= "CS")`, vr)
	}
	// Contents should match
	if val, ok := element.Value().(string); ok {
		if val != "MONOCHROME2" {
			t.Fatalf(`"%s" != "MONOCHROME2"`, val)
		}
	} else {
		t.Fatal("wrong type for element 0028,0004 (expected string)")
	}
}

// TestParseSQ attempts to parse SQ
func TestParseSQ(t *testing.T) {
	t.Parallel()
	element, err := elementFromBuffer(validSequenceElementBytes)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// tag should match
	if tag := element.Tag; tag != 0x00321064 {
		t.Fatalf("tag = %08X (!= 0x00321064)", uint32(tag))
	}
	// items should match
	if l := len(element.Items); l != 1 {
		t.Fatalf("len(items) = %d (!= 1)", l)
	}
	item := element.Items[0]
	// should have found four embedded elements
	if l := len(item.Elements); l != 4 {
		t.Fatalf("len(item.Elements) = %d (!= 4)", l)
	}
	if l := len(item.Unparsed); l != 0 {
		t.Fatalf("len(item.Unparsed) = %d (!= 0)", l)
	}
	// embedded element should match
	subelement, found := item.GetElement(0x00080102)
	if !found {
		t.Fatal("could not find subelement 0008,0102 inside sequence")
	}
	if subelement.VR != "SH" {
		t.Fatalf("subelement VR = %s (!= SH)", subelement.VR)
	}
	if val, ok := subelement.Value().(string); ok {
		if val != "SECTRA RIS" {
			t.Fatalf(`"%s" != "SECTRA RIS"`, val)
		}
	} else {
		t.Fatal("wrong type for subelement 0008,0102 (expected string)")
	}
}

// TestUnrecognisedSetFromUID tests that, given an unrecognised UID string, `SetFromUID` returns an error
func TestUrecognisedSetFromUID(t *testing.T) {
	t.Parallel()
	ts := TransferSyntax{}
	err := ts.SetFromUID("1.1.1.1.1.1.1.1")
	if err == nil {
		t.Fatal("SetFromUID with unrecognised UID should return error")
	}
}

// TestRecognisedSetFromUID tests that, given a recognised UID string, `SetFromUID` returns no error and correctly sets encoding
func TestRecognisedSetFromUID(t *testing.T) {
	t.Parallel()
	ts := TransferSyntax{}
	err := ts.SetFromUID("1.2.840.10008.1.2.2")
	if err != nil {
		t.Fatalf("SetFromUID returned error: %v", err)
	}
	if ts.Encoding.ImplicitVR {
		t.Fatalf("1.2.840.10008.1.2.2 should be Explicit VR")
	}
	if ts.Encoding.LittleEndian {
		t.Fatalf("1.2.840.10008.1.2.2 should be Big Endian")
	}
}

// TestEncodingStringRepresentation tests that the .String() method returns the expected string format
func TestEncodingStringRepresentation(t *testing.T) {
	t.Parallel()
	encoding := transferSyntaxToEncodingMap["1.2.840.10008.1.2"]
	str := encoding.String()
	expected := "ImplicitVR + LittleEndian"
	if str != expected {
		t.Fatalf(`got "%s" (!= "%s")`, str, expected)
	}

	encoding = transferSyntaxToEncodingMap["1.2.840.10008.1.2.2"]
	str = encoding.String()
	expected = "ExplicitVR + BigEndian"
	if str != expected {
		t.Fatalf(`got "%s" (!= "%s")`, str, expected)
	}
}

// TestUnrecognisedGetEncodingForTransferSyntax tests that, given an unrecognised TS, `GetEncodingForTransferSyntax` returns a default fallback.
func TestUnrecognisedGetEncodingForTransferSyntax(t *testing.T) {
	t.Parallel()
	ts := TransferSyntax{UIDEntry: &dictionary.UIDEntry{UID: "1.1.1.1.1.1"}}
	encoding := GetEncodingForTransferSyntax(ts)
	if encoding != transferSyntaxToEncodingMap["1.2.840.10008.1.2.1"] {
		t.Fatalf("encoding did not match expected encoding for unrecognised TS")
	}
}

func BenchmarkParseFromBuffer(b *testing.B) {
	f, err := os.Open(filepath.Join("testdata", "TCIA", "1.3.6.1.4.1.14519.5.2.1.2744.7002.251446451370536632612663178782.dcm"))
	if err != nil {
		panic(err)
	}
	stat, err := f.Stat()
	if err != nil {
		panic(err)
	}
	data := make([]byte, stat.Size())
	io.ReadFull(f, data)
	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := ParseFromBytes(data)
			if err != nil {
				b.Fatalf("error parsing dicom: %v", err)
			}
		}
	})

}
