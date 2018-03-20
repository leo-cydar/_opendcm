package opendcm

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

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
		assert.NoError(t, err, testCase.path)

		// should have found all elements
		assert.Len(t, dcm.Elements, testCase.numElements, testCase.path)

		stat, err := os.Stat(testCase.path)
		assert.NoError(t, err, testCase.path)

		// should be at end of file
		assert.Equal(t, stat.Size(), dcm.elementStream.GetPosition(), testCase.path)

		// check all elements values match correct type for their VR:
		for _, e := range dcm.Elements {
			assert.True(t, valueTypeMatchesVR(e.VR, e.Value()))
		}
	}
}

// TestParseValidBuffers tests that, given a valid DICOM buffer, the parser will correctly parse embedded elements
func TestParseValidBuffers(t *testing.T) {
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
		f, err := os.Open(testCase.path)
		assert.NoError(t, err, testCase.path)

		stat, err := os.Stat(testCase.path)
		assert.NoError(t, err, testCase.path)

		buf, err := ioutil.ReadAll(f)
		assert.NoError(t, err, testCase.path)

		dcm, err := ParseFromBytes(buf)
		assert.NoError(t, err, testCase.path)

		// should have found all elements
		assert.Len(t, dcm.Elements, testCase.numElements, testCase.path)

		// should be at end of file
		assert.Equal(t, stat.Size(), dcm.elementStream.GetPosition(), testCase.path)

		// check all elements values match correct type for their VR:
		for _, e := range dcm.Elements {
			assert.True(t, valueTypeMatchesVR(e.VR, e.Value()))
		}
	}
}

// TestIssue6 attempts to parse a valid file, with a source VR of UN that is matches as non-UN in our dictionary.
func TestIssue6(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "TCIA", "1.3.12.2.1107.5.1.4.1001.30000013072513125762500009613.dcm")
	dcm, err := ParseDicom(path)
	assert.NoError(t, err, path)

	// 0028,0107 (LargestImagePixelValue) has VR of US, with value 2766. But source DICOM specifies it as UN.
	// Check that the correct value has been parsed to prevent regression
	e, found := dcm.GetElement(0x00280107)
	assert.True(t, found)

	val := e.Value()
	assert.True(t, valueTypeMatchesVR(e.VR, val))
	assert.IsType(t, uint16(2766), val)
	assert.Equal(t, uint16(2766), val)
}

// TestParseFileWIthZeroElementLength tests that the parser can accept a file containing an embedded
// element with a defined length of zero
func TestParseFileWithZeroElementLength(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "synthetic", "ZeroElementLength.dcm")

	dcm, err := ParseDicom(path)
	assert.NoError(t, err, path)

	e, found := dcm.GetElement(0x00080005)
	assert.True(t, found)
	assert.Equal(t, uint32(0), e.ValueLength)
}

func TestParseFileWithMissingMetaLength(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "synthetic", "MissingMetaLength.dcm")

	dcm, err := ParseDicom(path)
	assert.NoError(t, err, path)

	assert.Equal(t, int64(340), dcm.TotalMetaBytes)

	e, found := dcm.GetElement(0x7FE00010)
	assert.True(t, found)
	assert.Equal(t, uint32(4), e.ValueLength)
}

func TestParseFileWithMissingTransferSyntax(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "synthetic", "MissingTransferSyntax.dcm")

	dcm, err := ParseDicom(path)
	assert.NoError(t, err, path)
	assert.Equal(t, int64(324), dcm.TotalMetaBytes)
	// transfer syntax should be the default (ImplicitVR, LittleEndian)
	assert.Equal(t, "1.2.840.10008.1.2", dcm.elementStream.TransferSyntax.UIDEntry.UID)

	e, found := dcm.GetElement(0x7FE00010)
	assert.True(t, found)
	assert.Equal(t, uint32(4), e.ValueLength)

	assert.Len(t, dcm.Elements, 8)
}

func TestParseFileWithMissingPreambleMagic(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "synthetic", "MissingPreambleMagic.dcm")

	dcm, err := ParseDicom(path)
	assert.NoError(t, err, path)
	assert.Equal(t, int64(192), dcm.TotalMetaBytes)
	// transfer syntax should be the default (ImplicitVR, LittleEndian)
	assert.Equal(t, "1.2.840.10008.1.2", dcm.elementStream.TransferSyntax.UIDEntry.UID)

	e, found := dcm.GetElement(0x7FE00010)
	assert.True(t, found)
	assert.Equal(t, uint32(4), e.ValueLength)

	assert.Len(t, dcm.Elements, 8)
}

/*
===============================================================================
    File Parsing: Invalid DICOMs
===============================================================================
*/
//TestParseUnsupportedDicoms tests that, given unsupported or unrecognised inputs, the parser will fail in an controlled manner
func TestParseUnsupportedDicoms(t *testing.T) {
	t.Parallel()
	// attempt to parse unsupported dicoms
	corruptFiles := []string{
		"UnrecognisedTransferSyntax.dcm",
	}
	for _, file := range corruptFiles {
		path := filepath.Join("testdata", "synthetic", file)
		_, err := ParseDicom(path)
		assert.Error(t, err)
		assert.IsType(t, &UnsupportedDicom{}, err)
	}
}

//TestParseUnsupportedBuffers tests that, given unsupported or unrecognised inputs, the parser will fail in an controlled manner
func TestParseUnsupportedBuffers(t *testing.T) {
	t.Parallel()
	// attempt to parse unsupported dicoms
	corruptFiles := []string{
		"UnrecognisedTransferSyntax.dcm",
	}
	for _, file := range corruptFiles {
		path := filepath.Join("testdata", "synthetic", file)

		f, err := os.Open(path)
		assert.NoError(t, err, path)
		buf, err := ioutil.ReadAll(f)
		assert.NoError(t, err, path)

		_, err = ParseFromBytes(buf)
		assert.Error(t, err, path)
		assert.IsType(t, &UnsupportedDicom{}, err, path)
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
	}
	for _, testCase := range testCases {
		_, err := ParseDicom(testCase)
		assert.Error(t, err)
		assert.IsType(t, &CorruptDicom{}, err, testCase)
	}
}
func TestStrictModeDisabled(t *testing.T) {
	// in non strict mode, inputs with elements exceeding remaining file size should not be rejected,
	// and should have their length adjusted.
	cfg.StrictMode = false
	OverrideConfig(cfg)
	testCases := []string{
		filepath.Join("testdata", "synthetic", "CorruptOverflowElementLength.dcm"),
	}
	for _, testCase := range testCases {
		_, err := ParseDicom(testCase)
		assert.NoError(t, err, testCase)
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
		assert.Error(t, err, string(buf))
		assert.IsType(t, &CorruptElement{}, err, string(buf))
		assert.Contains(t, err.Error(), "would exceed", string(buf))
	}
}

func TestImplicitVRVRLengthMissing(t *testing.T) {
	t.Parallel()
	buf := []byte{0x00, 0x08, 0x00, 0x02} // 0800,0200,UT
	es := elementStreamFromBuffer(buf)
	es.SetTransferSyntax("1.2.840.10008.1.2") // ImplicitVR
	_, err := es.GetElement()
	assert.Error(t, err, buf)
	assert.IsType(t, &CorruptElement{}, err, buf)
	assert.Contains(t, err.Error(), "would exceed", buf)
}

/*
===============================================================================
    ElementStream: Byte-Level Functions
===============================================================================
*/

func TestGetUndefinedLength(t *testing.T) {
	t.Parallel()

	// zero length buffer should return error
	es := elementStreamFromBuffer([]byte{})
	_, err := es.getUndefinedLength(true)
	assert.IsType(t, &CorruptElementStream{}, err)

	// buffer with length 4 should return error, as
	// it will be unable to skip "length" section
	es = elementStreamFromBuffer([]byte{0xFE, 0xFF, 0xDD, 0xE0})
	_, err = es.getUndefinedLength(true)
	assert.IsType(t, &CorruptElementStream{}, err)

	// buffer without start item / end sequence tag should
	// return error, as it indicates corruption.
	es = elementStreamFromBuffer([]byte{0xFE, 0xFF, 0xDE, 0xE1})
	_, err = es.getUndefinedLength(true)
	assert.IsType(t, &CorruptElementStream{}, err)

	// buffer with "Start Item" tag and no following buffer
	// should return error
	es = elementStreamFromBuffer([]byte{
		0xFE, 0xFF, 0x00, 0xE0,
	})
	_, err = es.getUndefinedLength(true)
	assert.IsType(t, &CorruptElementStream{}, err)

	// buffer with "Start Item" element of UL and immediately
	// following buffer being not-an-element should return error
	es = elementStreamFromBuffer([]byte{
		0xFE, 0xFF, 0x00, 0xE0, 0xFF, 0xFF, 0xFF, 0xFF,
	})
	_, err = es.getUndefinedLength(true)
	assert.IsType(t, &CorruptElementStream{}, err)

	// buffer with "Start Item" element of DL and immediately
	// following buffer being not-an-element should return error
	es = elementStreamFromBuffer([]byte{
		0xFE, 0xFF, 0x00, 0xE0, 0x00, 0x01, 0x00, 0x01,
	})
	_, err = es.getUndefinedLength(true)
	assert.IsType(t, &CorruptElementStream{}, err)
}

func TestGetUint16(t *testing.T) {
	t.Parallel()

	// < 2 length buffer should return error
	es := elementStreamFromBuffer(make([]byte, 0))
	_, err := es.getUint16()
	assert.IsType(t, &CorruptElementStream{}, err)

	es = elementStreamFromBuffer(make([]byte, 1))
	_, err = es.getUint16()
	assert.IsType(t, &CorruptElementStream{}, err)

	// cause problem with the elementStream reader
	// should return corruption error
	es = elementStreamFromBuffer(make([]byte, 2))
	es.reader.Discard(4)
	_, err = es.getUint16()
	assert.IsType(t, &CorruptElementStream{}, err)

	// check value returned is as expected for the
	// various encodings
	uint16Bytes := []byte{0x00, 0x01}
	// Little Endian
	es = elementStreamFromBuffer(uint16Bytes)
	es.SetTransferSyntax("1.2.840.10008.1.2.1")
	val, err := es.getUint16()
	assert.NoError(t, err)
	assert.Equal(t, uint16(0x100), val)
	// Big Endian
	es = elementStreamFromBuffer(uint16Bytes)
	es.TransferSyntax.SetFromUID("1.2.840.10008.1.2.2")
	val, err = es.getUint16()
	assert.NoError(t, err)
	assert.Equal(t, uint16(0x1), val)
}

func TestGetUint32(t *testing.T) {
	t.Parallel()

	// < 4 length buffer should return error
	es := elementStreamFromBuffer(make([]byte, 0))
	_, err := es.getUint32()
	assert.IsType(t, &CorruptElementStream{}, err)

	es = elementStreamFromBuffer(make([]byte, 3))
	_, err = es.getUint32()
	assert.IsType(t, &CorruptElementStream{}, err)

	// cause problem with the elementStream reader
	// should return corruption error
	es = elementStreamFromBuffer(make([]byte, 4))
	es.reader.Discard(8)
	_, err = es.getUint32()
	assert.IsType(t, &CorruptElementStream{}, err)

	// check value returned is as expected for the
	// various encodings
	uint32Bytes := []byte{0x00, 0x01, 0x00, 0x01}
	// Little Endian
	es = elementStreamFromBuffer(uint32Bytes)
	es.SetTransferSyntax("1.2.840.10008.1.2.1")
	val, err := es.getUint32()
	assert.NoError(t, err)
	assert.Equal(t, uint32(0x1000100), val)
	// Big Endian
	es = elementStreamFromBuffer(uint32Bytes)
	es.TransferSyntax.SetFromUID("1.2.840.10008.1.2.2")
	val, err = es.getUint32()
	assert.NoError(t, err)
	assert.Equal(t, uint32(0x10001), val)
}

func TestTagFromBytes(t *testing.T) {
	t.Parallel()
	bufLittleEndian := []byte{0x08, 0x00, 0x05, 0x00}
	bufBigEndian := []byte{0x00, 0x08, 0x00, 0x05}
	// little endian
	tag := tagFromBytes(bufLittleEndian, true)
	assert.Equal(t, uint32(0x00080005), tag)

	// big endian
	tag = tagFromBytes(bufBigEndian, false)
	assert.Equal(t, uint32(0x00080005), tag)
}

func TestSkipBytes(t *testing.T) {
	t.Parallel()
	// try to skip bytes in an empty buffer. should return
	// an error.
	es := elementStreamFromBuffer(make([]byte, 0))
	err := es.skipBytes(4)
	assert.IsType(t, &CorruptElementStream{}, err)

	// cause problem with the elementStream reader
	// should return corruption error
	es = elementStreamFromBuffer(make([]byte, 2))
	es.reader.Discard(4)
	err = es.skipBytes(2)
	assert.IsType(t, &CorruptElementStream{}, err)
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
	assert.NoError(t, err)
	assert.Equal(t, "parser", val)
}

func TestIsCharacterStringVR(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"AE", "AS", "CS", "DA", "DS", "DT", "IS", "LO", "LT", "PN", "SH", "ST", "TM", "UI", "UT"} {
		assert.True(t, IsCharacterStringVR(v), v)
	}
	for _, v := range []string{"OB", "FL"} {
		assert.False(t, IsCharacterStringVR(v), v)
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
		assert.Len(t, val, len(testCase.expected), testCase)
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
		assert.Len(t, val, len(testCase.expected), testCase)
		for i, split := range val {
			assert.Zero(t, bytes.Compare(split, testCase.expected[i]))
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
	assert.Equal(t, 0x0001FFFF, int(elements[0].Tag))
	assert.Equal(t, 0x00100020, int(elements[1].Tag))
	assert.Equal(t, 0x0020008F, int(elements[2].Tag))
}

func TestDescribe(t *testing.T) {
	t.Parallel()
	// describe a SQ element, which contains nested elements for optimal coverage
	lookup, found := LookupTag(0x00081130) // SQ
	assert.True(t, found)

	lookupSub, found := LookupTag(0x001811A2)
	assert.True(t, found)

	element := Element{DictEntry: lookup}
	element.Items = []Item{
		{Elements: map[uint32]Element{
			0x001811A2: {DictEntry: lookupSub, value: []byte("10"), ValueLength: 2},
		}},
	}
	description := element.Describe(0)
	assert.Len(t, description, 2)
	// now describe empty SQ
	element = Element{DictEntry: lookup}
	description = element.Describe(0)
	assert.Len(t, description, 1)
	// now describe Element with undefined length
	element, err := elementFromBuffer(validUNElementULBytes)
	assert.NoError(t, err)
	description = element.Describe(0)
	assert.Contains(t, description[1], "2 bytes")

	// now describe Element with > 256 bytes length
	// should not actually attempt to display contents
	element, err = elementFromBuffer(validCSElementBytes)
	assert.NoError(t, err)
	element.ValueLength = 1024
	description = element.Describe(0)
	assert.Contains(t, description[0], "1024 bytes")
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
		if testCase.supports {
			assert.True(t, supports, testCase)
		} else {
			assert.False(t, supports, testCase)
		}
	}
}

// TestParseCS attempts to parse CS
func TestParseCS(t *testing.T) {
	t.Parallel()
	element, err := elementFromBuffer(validCSElementBytes)
	assert.NoError(t, err)
	// tag should match
	assert.Equal(t, 0x00280004, int(element.Tag))
	// VR should match
	assert.Equal(t, "CS", element.VR)
	val := element.Value()
	// contents should match
	assert.IsType(t, "", val)
	assert.Equal(t, val, "MONOCHROME2")
}

// TestParseSQ attempts to parse SQ
func TestParseSQ(t *testing.T) {
	t.Parallel()
	element, err := elementFromBuffer(validSequenceElementBytes)
	assert.NoError(t, err)

	// tag should match
	assert.Equal(t, 0x00321064, int(element.Tag))

	// items should match
	assert.Len(t, element.Items, 1)
	item := element.Items[0]
	// should have found four embedded elements
	assert.Len(t, item.Elements, 4)
	assert.Len(t, item.Unparsed, 0)

	// embedded element should match
	subelement, found := item.GetElement(0x00080102)
	assert.True(t, found)

	assert.Equal(t, "SH", subelement.VR)
	val := subelement.Value()
	assert.IsType(t, "", val)
	assert.Equal(t, val, "SECTRA RIS")
}

// TestGuessTransferSyntaxFromBytes tests whether we are able to guess the correct transfer syntax
// from a series of signautre bytes.
func TestGuessTransferSyntaxFromBytes(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		signature []byte
		expected  Encoding
	}{
		{
			signature: []byte{0x08, 0x00, 0x06, 0x00, 0x53, 0x51},
			expected:  Encoding{ImplicitVR: false, LittleEndian: true},
		},
		{
			signature: []byte{0x08, 0x00, 0x06, 0x00, 0xFF, 0xFF},
			expected:  Encoding{ImplicitVR: true, LittleEndian: true},
		},
		{
			signature: []byte{0x00, 0x08, 0x00, 0x06, 0x53, 0x51},
			expected:  Encoding{ImplicitVR: false, LittleEndian: false},
		},
		{
			signature: []byte{0x00, 0x08, 0x00, 0x06, 0xFF, 0xFF},
			expected:  Encoding{ImplicitVR: true, LittleEndian: false},
		},
	}
	for i, testCase := range testCases {
		encoding, success := guessTransferSyntaxFromBytes(testCase.signature)
		assert.True(t, success, i)
		assert.Equal(t, testCase.expected, encoding, i)
	}
}

// TestUnrecognisedSetFromUID tests that, given an unrecognised UID string, `SetFromUID` returns an error
func TestUrecognisedSetFromUID(t *testing.T) {
	t.Parallel()
	ts := TransferSyntax{}
	err := ts.SetFromUID("1.1.1.1.1.1.1.1")
	assert.Error(t, err)
}

// TestRecognisedSetFromUID tests that, given a recognised UID string, `SetFromUID` returns no error and correctly sets encoding
func TestRecognisedSetFromUID(t *testing.T) {
	t.Parallel()
	ts := TransferSyntax{}
	err := ts.SetFromUID("1.2.840.10008.1.2.2")
	assert.NoError(t, err)

	// ExplicitVR, BigEndian
	assert.False(t, ts.Encoding.ImplicitVR)
	assert.False(t, ts.Encoding.LittleEndian)
}

// TestEncodingStringRepresentation tests that the .String() method returns the expected string format
func TestEncodingStringRepresentation(t *testing.T) {
	t.Parallel()
	encoding := transferSyntaxToEncodingMap["1.2.840.10008.1.2"]
	str := encoding.String()
	assert.Equal(t, "ImplicitVR + LittleEndian", str)

	encoding = transferSyntaxToEncodingMap["1.2.840.10008.1.2.2"]
	str = encoding.String()
	assert.Equal(t, "ExplicitVR + BigEndian", str)
}

// TestUnrecognisedGetEncodingForTransferSyntax tests that, given an unrecognised TS, `GetEncodingForTransferSyntax` returns a default fallback.
func TestUnrecognisedGetEncodingForTransferSyntax(t *testing.T) {
	t.Parallel()
	ts := TransferSyntax{UIDEntry: &dictionary.UIDEntry{UID: "1.1.1.1.1.1"}}
	encoding := GetEncodingForTransferSyntax(ts)
	assert.Equal(t, encoding, transferSyntaxToEncodingMap["1.2.840.10008.1.2.1"])
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
