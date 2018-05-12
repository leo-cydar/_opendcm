package opendcm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/b71729/opendcm/dictionary"

	"github.com/b71729/bin"

	"github.com/stretchr/testify/assert"
)

// Utils

// validUL1 contains one item of defined length
// ImplicitVR, LittleEndian
var validUL1 = []byte{
	0xFE, 0xFF, 0x00, 0xE0, // StartItem Tag
	0x0C, 0x00, 0x00, 0x00, // Item total length: 12 bytesf
	0x01, 0x7F, 0x34, 0x12, // (7F01,1234) Tag
	0x04, 0x00, 0x00, 0x00, // Length: 4 bytes
	0x4C, 0x65, 0x6F, 0x00, // Data: "Leo"+NULL
	0xFE, 0xFF, 0xDD, 0xE0, // SequenceDelimItem
	0x00, 0x00, 0x00, 0x00, // Filler: 4 bytes
}

// validUL2 contains one item of undefined length
// ImplicitVR, LittleEndian
var validUL2 = []byte{
	0xFE, 0xFF, 0x00, 0xE0, // StartItem Tag
	0xFF, 0xFF, 0xFF, 0xFF, // Item total length: undefined
	0x01, 0x7F, 0x34, 0x12, // (7F01,1234) Tag
	0x04, 0x00, 0x00, 0x00, // Length: 4 bytes
	0x4C, 0x65, 0x6F, 0x00, // Data: "Leo"+NULL
	0xFE, 0xFF, 0x0D, 0xE0, // ItemEnd Tag
	0x00, 0x00, 0x00, 0x00, // ItemEnd Length: 0
	0xFE, 0xFF, 0xDD, 0xE0, // SequenceDelimItem
	0x00, 0x00, 0x00, 0x00, // Filler: 4 bytes
}

var bytesVRTest = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x44, 0x49, 0x43, 0x4d, 0x02, 0x00, 0x00, 0x00, 0x55, 0x4c, 0x04, 0x00, 0xd0, 0x00, 0x00, 0x00,
	0x02, 0x00, 0x01, 0x00, 0x4f, 0x42, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x00, 0x02, 0x00,
	0x55, 0x49, 0x1a, 0x00, 0x31, 0x2e, 0x32, 0x2e, 0x38, 0x34, 0x30, 0x2e, 0x31, 0x30, 0x30, 0x30, 0x38, 0x2e,
	0x35, 0x2e, 0x31, 0x2e, 0x34, 0x2e, 0x31, 0x2e, 0x31, 0x2e, 0x36, 0x36, 0x02, 0x00, 0x03, 0x00, 0x55, 0x49,
	0x40, 0x00, 0x31, 0x2e, 0x32, 0x2e, 0x38, 0x32, 0x36, 0x2e, 0x30, 0x2e, 0x31, 0x2e, 0x33, 0x36, 0x38, 0x30,
	0x30, 0x34, 0x33, 0x2e, 0x39, 0x2e, 0x37, 0x34, 0x38, 0x34, 0x2e, 0x31, 0x35, 0x36, 0x34, 0x33, 0x32, 0x31,
	0x32, 0x35, 0x36, 0x38, 0x39, 0x38, 0x36, 0x38, 0x31, 0x36, 0x34, 0x30, 0x36, 0x35, 0x37, 0x37, 0x34, 0x34,
	0x33, 0x39, 0x34, 0x39, 0x37, 0x36, 0x38, 0x31, 0x31, 0x34, 0x37, 0x38, 0x02, 0x00, 0x10, 0x00, 0x55, 0x49,
	0x14, 0x00, 0x31, 0x2e, 0x32, 0x2e, 0x38, 0x34, 0x30, 0x2e, 0x31, 0x30, 0x30, 0x30, 0x38, 0x2e, 0x31, 0x2e,
	0x32, 0x2e, 0x31, 0x00, 0x02, 0x00, 0x12, 0x00, 0x55, 0x49, 0x20, 0x00, 0x31, 0x2e, 0x32, 0x2e, 0x38, 0x32,
	0x36, 0x2e, 0x30, 0x2e, 0x31, 0x2e, 0x33, 0x36, 0x38, 0x30, 0x30, 0x34, 0x33, 0x2e, 0x39, 0x2e, 0x37, 0x34,
	0x38, 0x34, 0x2e, 0x30, 0x2e, 0x31, 0x2e, 0x31, 0x02, 0x00, 0x13, 0x00, 0x53, 0x48, 0x0c, 0x00, 0x6f, 0x70,
	0x65, 0x6e, 0x64, 0x63, 0x6d, 0x2d, 0x30, 0x2e, 0x31, 0x00, 0x72, 0x00, 0x5e, 0x00, 0x41, 0x45, 0x06, 0x00,
	0x41, 0x45, 0x4e, 0x41, 0x4d, 0x45, 0x72, 0x00, 0x5f, 0x00, 0x41, 0x53, 0x04, 0x00, 0x30, 0x31, 0x32, 0x59,
	0x72, 0x00, 0x60, 0x00, 0x41, 0x54, 0x04, 0x00, 0x42, 0x24, 0x01, 0x90, 0x72, 0x00, 0x62, 0x00, 0x43, 0x53,
	0x0c, 0x00, 0x43, 0x4f, 0x44, 0x45, 0x53, 0x54, 0x52, 0x49, 0x4e, 0x47, 0x5f, 0x31, 0x72, 0x00, 0x61, 0x00,
	0x44, 0x41, 0x08, 0x00, 0x32, 0x30, 0x31, 0x38, 0x30, 0x33, 0x31, 0x37, 0x72, 0x00, 0x72, 0x00, 0x44, 0x53,
	0x06, 0x00, 0x33, 0x36, 0x30, 0x2e, 0x38, 0x00, 0x72, 0x00, 0x63, 0x00, 0x44, 0x54, 0x0c, 0x00, 0x32, 0x30,
	0x30, 0x35, 0x30, 0x38, 0x31, 0x30, 0x31, 0x32, 0x31, 0x35, 0x72, 0x00, 0x76, 0x00, 0x46, 0x4c, 0x04, 0x00,
	0x28, 0x04, 0xff, 0x42, 0x72, 0x00, 0x74, 0x00, 0x46, 0x44, 0x08, 0x00, 0x74, 0xd3, 0xad, 0xf9, 0x01, 0x24,
	0xfe, 0x40, 0x72, 0x00, 0x64, 0x00, 0x49, 0x53, 0x0a, 0x00, 0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37,
	0x38, 0x39, 0x72, 0x00, 0x66, 0x00, 0x4c, 0x4f, 0x0c, 0x00, 0x4c, 0x6f, 0x6e, 0x67, 0x20, 0x53, 0x74, 0x72,
	0x69, 0x6e, 0x67, 0x00, 0x72, 0x00, 0x68, 0x00, 0x4c, 0x54, 0x12, 0x00, 0x4c, 0x6f, 0x6e, 0x67, 0x5c, 0x54,
	0x65, 0x78, 0x74, 0x5c, 0x4e, 0x6f, 0x5c, 0x53, 0x70, 0x6c, 0x69, 0x74, 0x72, 0x00, 0x65, 0x00, 0x4f, 0x42,
	0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0xe0, 0x7f, 0x10, 0x00, 0x4f, 0x42, 0x00, 0x00,
	0xff, 0xff, 0xff, 0xff, 0xfe, 0xff, 0x00, 0xe0, 0x04, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0xfe, 0xff,
	0xdd, 0xe0, 0x00, 0x00, 0x00, 0x00, 0x72, 0x00, 0x73, 0x00, 0x4f, 0x44, 0x10, 0x00, 0x37, 0x5e, 0xfb, 0x34,
	0x00, 0x00, 0x00, 0x00, 0x72, 0xf2, 0x5b, 0x2e, 0x00, 0x00, 0x00, 0x00, 0x72, 0x00, 0x67, 0x00, 0x4f, 0x46,
	0x08, 0x00, 0xcd, 0xcc, 0xf6, 0x42, 0x33, 0xf3, 0x0d, 0x44, 0x72, 0x00, 0x69, 0x00, 0x4f, 0x57, 0x00, 0x00,
	0x10, 0x00, 0x00, 0x00, 0xe1, 0x10, 0x00, 0x00, 0x3d, 0x22, 0x00, 0x00, 0x3d, 0x08, 0x00, 0x00, 0x8f, 0x19,
	0x00, 0x00, 0x72, 0x00, 0x6a, 0x00, 0x50, 0x4e, 0x0c, 0x00, 0x41, 0x6e, 0x64, 0x65, 0x72, 0x73, 0x6f, 0x6e,
	0x5e, 0x4c, 0x65, 0x6f, 0x72, 0x00, 0x6c, 0x00, 0x53, 0x48, 0x0c, 0x00, 0x53, 0x68, 0x6f, 0x72, 0x74, 0x20,
	0x53, 0x74, 0x72, 0x69, 0x6e, 0x67, 0x72, 0x00, 0x7c, 0x00, 0x53, 0x4c, 0x04, 0x00, 0x2e, 0xfb, 0xff, 0xff,
	0x72, 0x00, 0x80, 0x00, 0x53, 0x51, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xfe, 0xff, 0x00, 0xe0, 0x04, 0x00,
	0x00, 0x00, 0x72, 0x00, 0x5f, 0x00, 0x41, 0x53, 0x04, 0x00, 0x30, 0x31, 0x32, 0x59, 0xfe, 0xff, 0x00, 0xe0,
	0x0e, 0x00, 0x00, 0x00, 0x72, 0x00, 0x6e, 0x00, 0x55, 0x54, 0x00, 0x00, 0x0e, 0x00, 0x00, 0x00, 0x55, 0x6e,
	0x6c, 0x69, 0x6d, 0x69, 0x74, 0x65, 0x64, 0x5c, 0x54, 0x65, 0x78, 0x74, 0xfe, 0xff, 0xdd, 0xe0, 0x00, 0x00,
	0x00, 0x00, 0x08, 0x00, 0x21, 0x91, 0x53, 0x51, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xfe, 0xff, 0x00, 0xe0,
	0xff, 0xff, 0xff, 0xff, 0x72, 0x00, 0x80, 0x00, 0x53, 0x51, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xfe, 0xff,
	0x00, 0xe0, 0xff, 0xff, 0xff, 0xff, 0x72, 0x00, 0x80, 0x00, 0x53, 0x51, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff,
	0xfe, 0xff, 0x00, 0xe0, 0xff, 0xff, 0xff, 0xff, 0x72, 0x00, 0x80, 0x00, 0x53, 0x51, 0x00, 0x00, 0xff, 0xff,
	0xff, 0xff, 0xfe, 0xff, 0x00, 0xe0, 0xff, 0xff, 0xff, 0xff, 0x72, 0x00, 0x80, 0x00, 0x53, 0x51, 0x00, 0x00,
	0xff, 0xff, 0xff, 0xff, 0xfe, 0xff, 0x00, 0xe0, 0xff, 0xff, 0xff, 0xff, 0x72, 0x00, 0x80, 0x00, 0x53, 0x51,
	0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xfe, 0xff, 0x00, 0xe0, 0x04, 0x00, 0x00, 0x00, 0x72, 0x00, 0x5f, 0x00,
	0x41, 0x53, 0x04, 0x00, 0x30, 0x31, 0x32, 0x59, 0xfe, 0xff, 0xdd, 0xe0, 0x00, 0x00, 0x00, 0x00, 0xfe, 0xff,
	0x0d, 0xe0, 0x00, 0x00, 0x00, 0x00, 0xfe, 0xff, 0xdd, 0xe0, 0x00, 0x00, 0x00, 0x00, 0xfe, 0xff, 0x0d, 0xe0,
	0x00, 0x00, 0x00, 0x00, 0xfe, 0xff, 0xdd, 0xe0, 0x00, 0x00, 0x00, 0x00, 0xfe, 0xff, 0x0d, 0xe0, 0x00, 0x00,
	0x00, 0x00, 0xfe, 0xff, 0xdd, 0xe0, 0x00, 0x00, 0x00, 0x00, 0xfe, 0xff, 0x0d, 0xe0, 0x00, 0x00, 0x00, 0x00,
	0xfe, 0xff, 0xdd, 0xe0, 0x00, 0x00, 0x00, 0x00, 0xfe, 0xff, 0x0d, 0xe0, 0x00, 0x00, 0x00, 0x00, 0xfe, 0xff,
	0xdd, 0xe0, 0x00, 0x00, 0x00, 0x00, 0x72, 0x00, 0x7e, 0x00, 0x53, 0x53, 0x02, 0x00, 0x2e, 0xfb, 0x72, 0x00,
	0x6e, 0x00, 0x53, 0x54, 0x14, 0x00, 0x53, 0x68, 0x6f, 0x72, 0x74, 0x5c, 0x54, 0x65, 0x78, 0x74, 0x5c, 0x4e,
	0x6f, 0x5c, 0x53, 0x70, 0x6c, 0x69, 0x74, 0x00, 0x72, 0x00, 0x6b, 0x00, 0x54, 0x4d, 0x0a, 0x00, 0x31, 0x32,
	0x31, 0x35, 0x33, 0x30, 0x2e, 0x33, 0x35, 0x00, 0x72, 0x00, 0x7f, 0x00, 0x55, 0x49, 0x0a, 0x00, 0x31, 0x32,
	0x37, 0x2e, 0x30, 0x2e, 0x30, 0x2e, 0x31, 0x00, 0x72, 0x00, 0x78, 0x00, 0x55, 0x4c, 0x04, 0x00, 0x15, 0xcd,
	0x5b, 0x07, 0x72, 0x00, 0x6d, 0x00, 0x55, 0x4e, 0x00, 0x00, 0x0b, 0x00, 0x00, 0x00, 0x55, 0x6e, 0x6b, 0x6e,
	0x6f, 0x77, 0x6e, 0x44, 0x61, 0x74, 0x61, 0x72, 0x00, 0x7a, 0x00, 0x55, 0x53, 0x02, 0x00, 0x39, 0x30, 0x72,
	0x00, 0x70, 0x00, 0x55, 0x54, 0x00, 0x00, 0x18, 0x00, 0x00, 0x00, 0x55, 0x6e, 0x6c, 0x69, 0x6d, 0x69, 0x74,
	0x65, 0x64, 0x5c, 0x54, 0x65, 0x78, 0x74, 0x5c, 0x4e, 0x6f, 0x5c, 0x53, 0x70, 0x6c, 0x69, 0x74, 0x00,
}

type devNull int

// devNull implements `io.Reader` and `io.Writer` to remove reader-specific impact on benchmarks
var blackHole = devNull(0)

func (devNull) Read(p []byte) (int, error) {
	return len(p), nil
}

func (devNull) Write(p []byte) (int, error) {
	return len(p), nil
}

type failAfterN struct {
	pos       int
	failAfter int
}

func (w *failAfterN) Write(p []byte) (int, error) {
	if w.failAfter <= w.pos {
		return 0, errors.New("error")
	}
	w.pos += len(p)
	return 0, nil
}

func (w *failAfterN) Read(p []byte) (int, error) {
	if w.failAfter <= w.pos {
		return 0, errors.New("error")
	}
	w.pos += len(p)
	return len(p), nil
}

/*
===============================================================================
    DataSet
===============================================================================
*/

func TestGetElement(t *testing.T) {
	// ensures that the return value of `GetElement`
	// correctly matches the presence of the element
	// within the dataset.
	t.Parallel()
	ds := make(DataSet, 0)
	ds.addElement(NewElementWithTag(0x00010001))
	e := Element{}
	assert.True(t, ds.GetElement(0x00010001, &e))

	// get one that's not in the dataset
	assert.False(t, ds.GetElement(0x10001000, &e))
}

func TestGetElementValue(t *testing.T) {
	// ensures that the value returned by `GetElementValue`
	// correctly matches the contained value.
	t.Parallel()
	ds := make(DataSet, 0)
	e := NewElementWithTag(0x00010001)
	e.data = []byte("testing")
	ds.addElement(e)
	var out string
	found, err := ds.GetElementValue(0x00010001, &out)
	assert.True(t, found)
	assert.NoError(t, err)
	assert.Equal(t, "testing", out)

	// get one that's not in the dataset
	found, err = ds.GetElementValue(0x10001000, &out)
	assert.False(t, found)
}

func TestAddElement(t *testing.T) {
	// ensures that `addElement` does not panic.
	t.Parallel()
	ds := make(DataSet, 0)
	ds.addElement(NewElement())
}

func TestHasElement(t *testing.T) {
	// ensures that `HasElement` correctly identifies
	// elements that are / are not present.
	t.Parallel()
	ds := make(DataSet, 0)
	ds.addElement(NewElementWithTag(0x00010001))
	assert.True(t, ds.HasElement(0x00010001))

	// false
	assert.False(t, ds.HasElement(0x10001000))
}

func TestLen(t *testing.T) {
	// ensures that `Len` correctly calculates the number
	// of contained elements.
	t.Parallel()
	ds := make(DataSet, 0)
	assert.Equal(t, 0, ds.Len())
	ds.addElement(NewElementWithTag(0x00010001))
	assert.Equal(t, 1, ds.Len())
}

func TestGetCharacterSet(t *testing.T) {
	// ensures that `GetCharacterSet` correctly identifies
	// the characterset when such element is / is not present.
	t.Parallel()
	ds := make(DataSet, 0)
	// default
	assert.Equal(t, "Default", ds.GetCharacterSet().Name)
	e := NewElementWithTag(0x00080005)
	e.data = []byte("ISO_IR 192")
	ds.addElement(e)
	assert.Equal(t, "ISO_IR 192", ds.GetCharacterSet().Name)
}

func TestSplitCharacterStringVM(t *testing.T) {
	// ensures that `splitCharacterStringVM` correctly
	// splits a string according to the split character.
	t.Parallel()
	buf := []byte(`test\string\four\splits`)
	split := splitCharacterStringVM(buf)
	assert.Len(t, split, 4)
}

func TestSplitBinaryVM(t *testing.T) {
	// ensures that `splitBinaryVM` correctly splits
	// a stream of bytes according to the paramters.
	t.Parallel()
	buf := []byte("1234123412341234")
	split := splitBinaryVM(buf, 4)
	assert.Len(t, split, 4)
	for _, section := range split {
		assert.Equal(t, []byte("1234"), section)
	}

	// test on extended buffer; should truncate last
	split = splitBinaryVM(append([]byte("12"), buf...), 4)
	assert.Len(t, split, 4)
}

func TestElementGetters(t *testing.T) {
	// ensures that the various getters of `Element`
	// correctly return the hidden values.
	t.Parallel()
	e := NewElementWithTag(0x00080005)
	e.data = []byte("ISO_IR 192")
	e.datalen = uint32(len(e.data))
	assert.Equal(t, uint32(0x00080005), e.GetTag())
	assert.Equal(t, "CS", e.GetVR())
	assert.Equal(t, "1-n", e.GetVM())
	assert.Equal(t, "SpecificCharacterSet", e.GetName())
	assert.Equal(t, false, e.HasItems())
	assert.Len(t, e.GetItems(), 0)
	assert.Equal(t, int(e.datalen), e.Len())
}

func TestSupportsType(t *testing.T) {
	// ensures that `supportsType` correctly identifies which
	// types are supported for the various VRs.
	t.Parallel()
	for vr, typ := range map[string]interface{}{
		"PN": "",
		"FL": float32(0.0),
		"FD": float64(0.0),
		"SS": int16(0),
		"SL": int32(0),
		"US": uint16(0),
		"UL": uint32(0),
	} {
		e := NewElement()
		e.dictEntry.VR = vr
		assert.True(t, e.supportsType(typ))
	}

	// all VRs should support expression as []byte
	for _, vr := range RecognisedVRs {
		e := NewElement()
		e.dictEntry.VR = vr
		assert.True(t, e.supportsType([]byte{}))
	}

	// returns false if can't support
	e := NewElement()
	e.dictEntry.VR = "FL"
	assert.False(t, e.supportsType(float64(0.0)))
}

func TestGetValue(t *testing.T) {
	t.Parallel()
	// both big and little endian:
	for _, isLittle := range []bool{false, true} {
		for _, vr := range RecognisedVRs {
			// for each vr
			// create element with that vr
			e := NewElement()
			e.dictEntry.VR = vr
			e.isLittleEndian = isLittle
			// switch it's VR
			switch vr {
			case "FL":
				e.data = make([]byte, 4)
				dst := float32(0.0)
				assert.NoError(t, e.GetValue(&dst))
				dst2 := []float32{}
				assert.NoError(t, e.GetValue(&dst2))
			case "FD":
				e.data = make([]byte, 8)
				dst := float64(0.0)
				assert.NoError(t, e.GetValue(&dst))
				dst2 := []float64{}
				assert.NoError(t, e.GetValue(&dst2))
			case "SS":
				e.data = make([]byte, 2)
				dst := int16(0.0)
				assert.NoError(t, e.GetValue(&dst))
				dst2 := []int16{}
				assert.NoError(t, e.GetValue(&dst2))
			case "SL":
				e.data = make([]byte, 4)
				dst := int32(0.0)
				assert.NoError(t, e.GetValue(&dst))
				dst2 := []int32{}
				assert.NoError(t, e.GetValue(&dst2))
			case "UN":
				e.data = make([]byte, 4)
				dst := make([]byte, 4)
				assert.NoError(t, e.GetValue(&dst))
			}
			// read accordingly
		}
	}
}

func TestGetValueError(t *testing.T) {
	// ensures that the error condition of `GetValue`
	// responds correctly.
	t.Parallel()
	// returns error if does not support target type
	e := NewElement()
	e.dictEntry.VR = "AS"
	i := int32(0)
	assert.Error(t, e.GetValue(&i))
	e.dictEntry.VR = "UN"
	// returns error if destination is unwritable
	assert.Error(t, e.GetValue(int32(0)))
	// returns error if writing to destination is unimplemented
	assert.Error(t, e.GetValue(struct{}{}))
}

func TestNewElement(t *testing.T) {
	// ensures that, when initialising a new element via
	// `NewElement`, that the initial values are as below.
	t.Parallel()
	e := NewElement()
	assert.Equal(t, "UninitialisedMemory", e.GetName())
	assert.Equal(t, uint32(0xFFFFFFFF), e.GetTag())
	assert.Equal(t, "UN", e.GetVR())
}

func TestNewElementWithTag(t *testing.T) {
	// ensures that `NewElementWithTag` correctly sets
	// the embedded dictionary reference.
	t.Parallel()
	e := NewElementWithTag(0x00080005)
	assert.Equal(t, "SpecificCharacterSet", e.GetName())
}

/*
===============================================================================
    ElementReader
===============================================================================
*/

func newReader() ElementReader {
	return NewElementReader(bin.NewReader(bytes.NewReader(bytesVRTest), binary.LittleEndian))
}

func TestShouldReadEmbeddedElements(t *testing.T) {
	// ensures that, if an element's tag is PixelData,
	// embedded elements will not be parsed.
	t.Parallel()
	assert.False(t, shouldReadEmbeddedElements(NewElementWithTag(pixelDataTag)))
	assert.True(t, shouldReadEmbeddedElements(NewElementWithTag(0x00080005)))
	// and returns true when the tag is not recognised:
	assert.True(t, shouldReadEmbeddedElements(NewElementWithTag(0xDEADBEEF)))
}

func TestLookupTag(t *testing.T) {
	// ensure that, for recognised and unrecognised tags,
	// `lookupTag` will correctly respond.
	t.Parallel()
	_, found := lookupTag(0x00000000)
	assert.False(t, found)
	de, found := lookupTag(pixelDataTag)
	assert.True(t, found)
	assert.IsType(t, &dictionary.DictEntry{}, de)
	// should be PixelData
	assert.Equal(t, "PixelData", de.Name)
}

func TestNewElementReader(t *testing.T) {
	t.Parallel()
	src := bin.NewReader(bytes.NewReader(make([]byte, 64)), binary.LittleEndian)
	assert.IsType(t, ElementReader{}, NewElementReader(src))
}

func TestIsLittleEndian(t *testing.T) {
	// ensures that `IsLittleEndian` correctly
	// reports whether the reader is little endian.
	t.Parallel()
	reader := NewElementReader(bin.NewReader(bytes.NewReader(make([]byte, 64)), binary.LittleEndian))
	assert.True(t, reader.IsLittleEndian())
	reader = NewElementReader(bin.NewReader(bytes.NewReader(make([]byte, 64)), binary.BigEndian))
	assert.False(t, reader.IsLittleEndian())
}

func TestSetLittleEndian(t *testing.T) {
	// ensures that `SetLittleEndian` correctly sets
	// the "little endian" flag
	t.Parallel()
	r := newReader()
	r.SetLittleEndian(true)
	assert.True(t, r.IsLittleEndian())
	r.SetLittleEndian(false)
	assert.False(t, r.IsLittleEndian())
}

func TestIsImplicitVR(t *testing.T) {
	// ensures that `IsImplicitVR` correctly returns
	// whether the reader is in "implicit VR" mode.
	t.Parallel()
	r := newReader()
	assert.True(t, r.IsImplicitVR())
	r.implicit = false
	assert.False(t, r.IsImplicitVR())
}

func TestSetImplicitVR(t *testing.T) {
	// ensures that `SetImplicitVR` correctly sets
	// the "implicit VR" flag
	t.Parallel()
	r := newReader()
	r.SetImplicitVR(true)
	assert.True(t, r.IsImplicitVR())
	r.SetImplicitVR(false)
	assert.False(t, r.IsImplicitVR())
}

func TestReadElementVR(t *testing.T) {
	// ensures that `readElementVR` correctly reads
	// a value representation from the reader.
	t.Parallel()
	buf := []byte("CS")
	reader := NewElementReader(bin.NewReader(bytes.NewReader(buf), binary.LittleEndian))
	reader.SetImplicitVR(false)
	e := NewElementWithTag(0x00010001)
	assert.NoError(t, reader.readElementVR(&e))
	assert.Equal(t, "CS", e.GetVR())
}

func TestReadElementVRError(t *testing.T) {
	// ensures that the error condition of `GetValue`
	// responds correctly.
	t.Parallel()
	// insufficient bytes
	buf := []byte("C")
	reader := NewElementReader(bin.NewReader(bytes.NewReader(buf), binary.LittleEndian))
	reader.SetImplicitVR(false)
	e := NewElementWithTag(0x00010001)
	assert.Error(t, reader.readElementVR(&e))
}

func TestReadElementLength(t *testing.T) {
	// ensures that `readElementLength` correctly reads
	// a length specifier from the reader.
	t.Parallel()

	// implicit VR: all length definitions are 32 bits
	buf := []byte{0x78, 0x56, 0x34, 0x12}
	reader := NewElementReader(bin.NewReader(bytes.NewReader(buf), binary.LittleEndian))
	reader.SetImplicitVR(true)
	e := NewElementWithTag(0x00010001)
	assert.NoError(t, reader.readElementLength(&e))
	assert.Equal(t, uint32(0x12345678), e.datalen)

	// explicit VR, 32 bit length
	buf = []byte{0xFF, 0xFF, 0x78, 0x56, 0x34, 0x12}
	reader = NewElementReader(bin.NewReader(bytes.NewReader(buf), binary.LittleEndian))
	reader.SetImplicitVR(false)
	e = NewElementWithTag(0x00010001)
	assert.NoError(t, reader.readElementLength(&e))
	assert.Equal(t, uint32(0x12345678), e.datalen)

	// explicit VR, 16 bit length
	buf = []byte{0xFF, 0xFF, 0x78, 0x56, 0x34, 0x12}
	reader = NewElementReader(bin.NewReader(bytes.NewReader(buf), binary.LittleEndian))
	reader.SetImplicitVR(false)
	e = NewElementWithTag(0x000100010)
	assert.NoError(t, reader.readElementLength(&e))
	assert.Equal(t, uint32(0xFFFF), e.datalen)
}

func TestReadElementLengthError(t *testing.T) {
	// ensures that the error condition of
	// `readElementLength` responds correctly.
	t.Parallel()
	// implicit VR
	// error reading length (insufficient bytes)
	buf := []byte{0x78, 0x56, 0x34}
	reader := NewElementReader(bin.NewReader(bytes.NewReader(buf), binary.LittleEndian))
	reader.SetImplicitVR(true)
	e := NewElementWithTag(0x00010001)
	assert.Error(t, reader.readElementLength(&e))

	// explicit VR, 32 bit length
	// error reading length (insufficient bytes)
	buf = []byte{0xFF, 0xFF, 0x78, 0x56, 0x34}
	reader = NewElementReader(bin.NewReader(bytes.NewReader(buf), binary.LittleEndian))
	reader.SetImplicitVR(false)
	e = NewElementWithTag(0x00010001)
	assert.Error(t, reader.readElementLength(&e))

	// explicit VR, 16 bit length
	// error reading length (insufficient bytes)
	buf = []byte{0xFF}
	reader = NewElementReader(bin.NewReader(bytes.NewReader(buf), binary.LittleEndian))
	reader.SetImplicitVR(false)
	e = NewElementWithTag(0x00100010)
	assert.Error(t, reader.readElementLength(&e))
}

func TestTagFromBytes(t *testing.T) {
	// ensures that `tagFromBytes` correctly parses a
	// dicom tag from sequences of bytes.
	t.Parallel()
	buf := []byte{0x10, 0x20, 0x10, 0x20}
	// little endian
	reader := NewElementReader(bin.NewReader(bytes.NewReader(buf), binary.LittleEndian))
	dst := uint32(0)
	assert.NoError(t, reader.tagFromBytes(buf, &dst))
	assert.Equal(t, uint32(0x20102010), dst)

	//big endian
	reader = NewElementReader(bin.NewReader(bytes.NewReader(buf), binary.BigEndian))
	assert.NoError(t, reader.tagFromBytes(buf, &dst))
	assert.Equal(t, uint32(0x10201020), dst)
}

func TestTagFromBytesError(t *testing.T) {
	// ensures that the error condition of
	// `tagFromBytes` responds correctly.
	t.Parallel()
	// insufficient bytes
	buf := make([]byte, 3)
	reader := NewElementReader(bin.NewReader(bytes.NewReader(buf), binary.BigEndian))
	dst := uint32(0)
	assert.Error(t, reader.tagFromBytes(buf, &dst))
}

func TestHasReachedTag(t *testing.T) {
	// ensures that `hasReachedTag` correctly recognised
	// that the reader has reached target tag.
	t.Parallel()
	buf := []byte{0x00, 0x00, 0x00, 0x00, 0x11, 0x11, 0x11, 0x11}
	reader := NewElementReader(bin.NewReader(bytes.NewReader(buf), binary.LittleEndian))
	// not reached
	reached, err := reader.hasReachedTag(0x11111111)
	assert.NoError(t, err)
	assert.False(t, reached)

	// reached
	reader.br.Discard(4)
	reached, err = reader.hasReachedTag(0x11111111)
	assert.NoError(t, err)
	assert.True(t, reached)
}

func TestHasReachedTagError(t *testing.T) {
	// ensures that the error condition of
	// `hasReachedTag` responds correctly.
	t.Parallel()
	// insufficient bytes
	buf := []byte{0x11, 0x11, 0x11}
	reader := NewElementReader(bin.NewReader(bytes.NewReader(buf), binary.LittleEndian))
	_, err := reader.hasReachedTag(0x11111111)
	assert.Error(t, err)
}

func TestReadItemUndefLength(t *testing.T) {
	// ensures that `readItemUndefLength` correctly
	// parses an "undefined length" item from the reader.
	t.Parallel()
	r := newReader()
	itm := NewItem()
	// TODO
	r.readItemUndefLength(false, &itm)
	r.readItemUndefLength(true, &itm)
}

/*
===============================================================================
    Dicom
===============================================================================
*/

func TestGetPreamble(t *testing.T) {
	t.Parallel()
	dcm := newDicom()
	assert.Equal(t, [128]byte{}, dcm.GetPreamble())

	// populate preamble and retry
	preamble := [128]byte{}
	preamble[0] = byte(0xEE)
	preamble[1] = byte(0xDD)
	dcm.preamble = preamble
	assert.Equal(t, preamble, dcm.GetPreamble())
}

func TestNewDicom(t *testing.T) {
	t.Parallel()
	assert.IsType(t, Dicom{}, newDicom())
}

func TestAttemptReadPreamble(t *testing.T) {
	t.Parallel()

	// should return true
	dcm := newDicom()
	f, err := os.Open(filepath.Join("testdata", "synthetic", "VRTest.dcm"))
	assert.NoError(t, err)
	r := bin.NewReader(f, binary.LittleEndian)
	b, err := dcm.attemptReadPreamble(&r)
	assert.NoError(t, err)
	assert.True(t, b)
	assert.Equal(t, [128]byte{}, dcm.preamble)

	// should return false
	dcm = newDicom()
	f, err = os.Open(filepath.Join("testdata", "synthetic", "MissingPreambleMagic.dcm"))
	assert.NoError(t, err)
	r = bin.NewReader(f, binary.LittleEndian)
	b, err = dcm.attemptReadPreamble(&r)
	assert.NoError(t, err)
	assert.False(t, b)
}

func TestAttemptReadPreambleError(t *testing.T) {
	t.Parallel()

	// not enough bytes
	dcm := newDicom()
	r := bin.NewReaderBytes([]byte{}, binary.LittleEndian)
	b, err := dcm.attemptReadPreamble(&r)
	assert.Error(t, err)
	assert.False(t, b)
}

func TestFromReader(t *testing.T) {
	t.Parallel()
	// from file reader
	f, err := os.Open(filepath.Join("testdata", "synthetic", "MissingPreambleMagic.dcm"))
	assert.NoError(t, err)
	dcm, err := FromReader(f)
	assert.NoError(t, err)
	assert.Equal(t, 8, dcm.Len())
	f.Close()

	// from byte reader
	f, err = os.Open(filepath.Join("testdata", "synthetic", "MissingPreambleMagic.dcm"))
	stat, err := f.Stat()
	assert.NoError(t, err)

	// read into byte slice
	buf := make([]byte, stat.Size())
	nread, err := f.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, len(buf), nread)
	r := bytes.NewReader(buf)
	dcm, err = FromReader(r)
	assert.NoError(t, err)
	assert.Equal(t, 8, dcm.Len())
	f.Close()
}

func TestFromReaderError(t *testing.T) {
	t.Parallel()

	// dicom bytes that are not enough to peek the preamble component
	notEnoughBytes := make([]byte, 100)
	r := bytes.NewReader(notEnoughBytes)
	_, err := FromReader(r)
	assert.Error(t, err)

	// dicom bytes that make up a valid preamble component
	// but then abruptly ends; should not return error, but still is
	// unusable
	preambleNoElements := make([]byte, 132)
	preambleNoElements[128] = byte('D')
	preambleNoElements[129] = byte('I')
	preambleNoElements[130] = byte('C')
	preambleNoElements[131] = byte('M')
	r = bytes.NewReader(preambleNoElements)
	_, err = FromReader(r)
	assert.NoError(t, err)

	// append one byte, this should cause subsequent element parsing
	// to fail
	preambleNoElements = append(preambleNoElements, byte(0x00))
	r = bytes.NewReader(preambleNoElements)
	_, err = FromReader(r)
	assert.Error(t, err)

	// append another byte, should not be an explicit error
	preambleNoElements = append(preambleNoElements, byte(0x00))
	r = bytes.NewReader(preambleNoElements)
	_, err = FromReader(r)
	assert.NoError(t, err)

	// append another bytes, this should cause subsequent element
	// parsing to fail
	preambleNoElements = append(preambleNoElements, byte(0x00))
	r = bytes.NewReader(preambleNoElements)
	_, err = FromReader(r)
	assert.Error(t, err)

	// append another couple bytes, this also should cause subsequent element
	// parsing to fail
	preambleNoElements = append(preambleNoElements, make([]byte, 2)...)
	r = bytes.NewReader(preambleNoElements)
	_, err = FromReader(r)
	assert.Error(t, err)
}

func TestFromFile(t *testing.T) {
	t.Parallel()
	dcm, err := FromFile(filepath.Join("testdata", "synthetic", "VRTest.dcm"))
	assert.NoError(t, err)
	assert.Equal(t, 27, dcm.Len())
}

func TestFromFileError(t *testing.T) {
	t.Parallel()
	// try to parse dicom from
	// 1: file that does not exist
	// 2: file that exists, but is not a dicom
	for _, path := range []string{"__.__0000", "dicom_test.go"} {
		_, err := FromFile(path)
		assert.Error(t, err)
	}
}

func TestFromFileNoPermission(t *testing.T) {
	// try to parse a dicom from a file that the user has no read
	// permissions
	if runtime.GOOS == "windows" {
		t.Skipf("skip (windows)")
	}
	t.Parallel()
	f, err := ioutil.TempFile("", "")
	assert.NoError(t, err)
	assert.NoError(t, f.Chmod(0333))
	defer os.Remove(f.Name())
	_, err = FromFile(f.Name())
	assert.Error(t, err)
}

func TestCharsetDecode(t *testing.T) {
	// ensure that, given a range of charactersets, the output is as expected
	t.Parallel()
	for _, testCase := range []struct {
		filename             string
		expectedCharacterSet string
		expectedPatientName  string
	}{
		{
			filename:             "ShiftJIS.dcm",
			expectedCharacterSet: "ISO_IR 13",
			expectedPatientName:  "エンコードされたメッセージ",
		},
		{
			filename:             "ISO_IR100.dcm",
			expectedCharacterSet: "ISO_IR 100",
			expectedPatientName:  "Encoded Message",
		},
		{
			filename:             "ISO_IR101.dcm",
			expectedCharacterSet: "ISO_IR 101",
			expectedPatientName:  "kódovanej správy",
		},
		{
			filename:             "ISO_IR109.dcm",
			expectedCharacterSet: "ISO_IR 109",
			expectedPatientName:  "messaġġ kodifikat",
		},
		{
			filename:             "ISO_IR110.dcm",
			expectedCharacterSet: "ISO_IR 110",
			expectedPatientName:  "kodeeritud sõnum",
		},
		{
			filename:             "ISO_IR126.dcm",
			expectedCharacterSet: "ISO_IR 126",
			expectedPatientName:  "κωδικοποιημένο μήνυμα",
		},
		{
			filename:             "ISO_IR127.dcm",
			expectedCharacterSet: "ISO_IR 127",
			expectedPatientName:  "رسالة مشفرة",
		},
		{
			filename:             "ISO_IR138.dcm",
			expectedCharacterSet: "ISO_IR 138",
			expectedPatientName:  "הודעה מקודדת",
		},
		{
			filename:             "ISO_IR144.dcm",
			expectedCharacterSet: "ISO_IR 144",
			expectedPatientName:  "закодированное сообщение",
		},
		{
			filename:             "ISO_IR148.dcm",
			expectedCharacterSet: "ISO_IR 148",
			expectedPatientName:  "kodlanmış mesaj",
		},
		{
			filename:             "ISO_IR166.dcm",
			expectedCharacterSet: "ISO_IR 166",
			expectedPatientName:  "ข้อความที่เข้ารหัส",
		},
		{
			filename:             "ISO_IR192.dcm",
			expectedCharacterSet: "ISO_IR 192",
			expectedPatientName:  "Éncø∂é∂ √ålüÉ",
		},
		{
			filename:             "GB18030.dcm",
			expectedCharacterSet: "GB18030",
			expectedPatientName:  "编码值",
		},
	} {
		dcm, err := FromFile(filepath.Join("testdata", "synthetic", testCase.filename))
		assert.NoError(t, err)
		assert.Equal(t, testCase.expectedCharacterSet, dcm.GetCharacterSet().Name)
		name := ""
		var e = NewElement()
		assert.True(t, dcm.GetElement(0x00100010, &e))
		assert.NoError(t, e.GetValue(&name))
		assert.Equal(t, testCase.expectedPatientName, name)
	}
}

func BenchmarkFromReader(b *testing.B) {
	// from byte reader
	f, err := os.Open(filepath.Join("testdata", "synthetic", "VRTest.dcm"))
	if err != nil {
		b.Fatal(err)
	}
	stat, err := f.Stat()
	if err != nil {
		b.Fatal(err)
	}

	// read into byte slice
	buf := make([]byte, stat.Size())
	nread, err := f.Read(buf)
	if err != nil {
		b.Fatal(err)
	}
	if nread != len(buf) {
		b.Fatal(nread)
	}
	r := bytes.NewReader(buf)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FromReader(r)
		r.Reset(buf)
	}
}
