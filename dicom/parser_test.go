package dicom

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

/*
===============================================================================
	Utilities
===============================================================================
*/

var testDataDirectory = filepath.Join("..", "testdata")

var validSequenceElementBytes = []byte{0x32, 0x00, 0x64, 0x10, 0x53, 0x51, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFE, 0xFF, 0x00, 0xE0, 0xFF, 0xFF, 0xFF, 0xFF, 0x08, 0x00, 0x00, 0x01, 0x53, 0x48, 0x0E, 0x00, 0x53, 0x4E, 0x47, 0x30, 0x41, 0x47, 0x2F, 0x5A, 0x54, 0x58, 0x30, 0x58, 0x42, 0x20, 0x08, 0x00, 0x02, 0x01, 0x53, 0x48, 0x0A, 0x00, 0x53, 0x45, 0x43, 0x54, 0x52, 0x41, 0x20, 0x52, 0x49, 0x53, 0x08, 0x00, 0x03, 0x01, 0x53, 0x48, 0x04, 0x00, 0x31, 0x2E, 0x30, 0x20, 0x08, 0x00, 0x04, 0x01, 0x4C, 0x4F, 0x0A, 0x00, 0x4D, 0x52, 0x20, 0x4B, 0x6E, 0x65, 0x20, 0x73, 0x69, 0x6E, 0xFE, 0xFF, 0x0D, 0xE0, 0x00, 0x00, 0x00, 0x00, 0xFE, 0xFF, 0xDD, 0xE0, 0x00, 0x00, 0x00, 0x00}
var validCSElementBytes = []byte{0x28, 0x00, 0x04, 0x00, 0x43, 0x53, 0x0C, 0x00, 0x4D, 0x4F, 0x4E, 0x4F, 0x43, 0x48, 0x52, 0x4F, 0x4D, 0x45, 0x32, 0x20}

func elementFromBuffer(buf []byte) (Element, error) {
	return elementStreamFromBuffer(buf).GetElement()
}

func elementStreamFromBuffer(buf []byte) *ElementStream {
	es := NewElementStream(bufio.NewReader(bytes.NewReader(buf)), int64(len(buf)))
	return &es
}

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

func TestParseValidFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(testDataDirectory, "TCIA", "1.3.6.1.4.1.14519.5.2.1.2744.7002.251446451370536632612663178782.dcm")
	dcm, err := ParseDicom(path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// should have found all elements
	if l := len(dcm.Elements); l != 105 {
		t.Fatalf("number of elements = %d (!= 105)", l)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// should be at end of file
	if pos := dcm.elementStream.GetPosition(); pos != stat.Size() {
		t.Fatalf("reader position = %d (!= %d)", pos, stat.Size())
	}

	// check all elements values match correct type for their VR:
	for _, e := range dcm.Elements {
		val := e.Value()
		if !valueTypeMatchesVR(e.VR, val) {
			t.Fatalf(`type "%s" for element %s is incorrect (VR="%s")`, reflect.TypeOf(val), e.Tag, e.VR)
		}
	}
}

// TestIssue6 attempts to parse a valid file, with a source VR of UN that is matches as non-UN in our dictionary.
func TestIssue6(t *testing.T) {
	t.Parallel()
	path := filepath.Join(testDataDirectory, "TCIA", "1.3.12.2.1107.5.1.4.1001.30000013072513125762500009613.dcm")
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
		t.Fatalf("(0028,0107) did not return expected value of 2766")
	}
}

func TestParseFileWithZeroElementLength(t *testing.T) {
	t.Parallel()
	path := filepath.Join(testDataDirectory, "synthetic", "ZeroElementLength.dcm")
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

/*
===============================================================================
	File Parsing: Invalid DICOMs
===============================================================================
*/
func TestParseCorruptDicoms(t *testing.T) {
	t.Parallel()
	// attempt to parse corrupt dicoms
	corruptFiles := []string{
		"CorruptBadTransferSyntax.dcm",
		"CorruptMissingMetaLength.dcm",
	}
	for _, file := range corruptFiles {
		path := filepath.Join(testDataDirectory, "synthetic", file)
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

func TestStrictMode(t *testing.T) {
	t.Parallel() // ?
	// in strict mode, inputs with elements exceeding remaining file size should be rejected
	StrictMode = true
	path := filepath.Join(testDataDirectory, "synthetic", "CorruptOverflowElementLength.dcm")
	_, err := ParseDicom(path)
	switch err.(type) {
	case *CorruptDicom:
	default:
		t.Fatalf(`with "StrictMode" disabled, parsing "%s" should have raised a CorruptDicomError (got %v)`, path, err)
	}

	// in non strict mode, inputs with elements exceeding remaining file size should not be rejected,
	// and should have their length adjusted.
	StrictMode = false
	_, err = ParseDicom(path)
	if err != nil {
		t.Fatalf(`with "StrictMode" disabled, parsing "%s" should not have raised an error (got %v)`, path, err)
	}
}

func TestGetElementWithInsufficientBytes(t *testing.T) {
	t.Parallel()
	testCases := [][]byte{
		make([]byte, 0), // cannot read lower section of tag
		make([]byte, 2), // cannot read upper section of tag
		make([]byte, 4), // cannot read VR
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

/*
===============================================================================
	Element Parsing: VRs
===============================================================================
*/

// TestParseSQ attempts to parse CS
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
	if l := len(item.UnknownSections); l != 0 {
		t.Fatalf("len(item.UnknownSections) = %d (!= 0)", l)
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
