package opendcm

import (
	"errors"
	"testing"

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
	t.Parallel()
	ds := make(DataSet, 0)
	ds.addElement(NewElementWithTag(0x00010001))
	e := Element{}
	assert.True(t, ds.GetElement(0x00010001, &e))

	// get one that's not in the dataset
	assert.False(t, ds.GetElement(0x10001000, &e))
}

func TestGetElementValue(t *testing.T) {
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
	t.Parallel()
	ds := make(DataSet, 0)
	ds.addElement(NewElement())
}

func TestHasElement(t *testing.T) {
	t.Parallel()
	ds := make(DataSet, 0)
	ds.addElement(NewElementWithTag(0x00010001))
	assert.True(t, ds.HasElement(0x00010001))

	// false
	assert.False(t, ds.HasElement(0x10001000))
}

func TestLen(t *testing.T) {
	t.Parallel()
	ds := make(DataSet, 0)
	assert.Equal(t, 0, ds.Len())
	ds.addElement(NewElementWithTag(0x00010001))
	assert.Equal(t, 1, ds.Len())
}

func TestGetCharacterSet(t *testing.T) {
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
	t.Parallel()
	buf := []byte(`test\string\four\splits`)
	split := splitCharacterStringVM(buf)
	assert.Len(t, split, 4)
}

func TestSplitBinaryVM(t *testing.T) {
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
}

func TestGetValueError(t *testing.T) {
	t.Parallel()
	// returns error if does not support target type
	e := NewElement()
	e.dictEntry.VR = "AS"
	i := int32(0)
	assert.Error(t, e.GetValue(&i))
}

func TestNewElement(t *testing.T) {
	t.Parallel()
	e := NewElement()
	assert.Equal(t, "UninitialisedMemory", e.GetName())
	assert.Equal(t, uint32(0xFFFFFFFF), e.GetTag())
	assert.Equal(t, "UN", e.GetVR())
}

func TestNewElementWithTag(t *testing.T) {
	t.Parallel()
	// ensure that, when `NewElementWithTag` is called,
	// the `dictEntry` is pre-populated.
	e := NewElementWithTag(0x00080005)
	assert.Equal(t, "SpecificCharacterSet", e.GetName())
}

func TestShouldReadEmbeddedElements(t *testing.T) {
	t.Parallel()
	// ensure that, if an element's tag is PixelData,
	// embedded elements will not be parsed.
	assert.False(t, shouldReadEmbeddedElements(NewElementWithTag(pixelDataTag)))
	assert.True(t, shouldReadEmbeddedElements(NewElementWithTag(0x00080005)))
}
