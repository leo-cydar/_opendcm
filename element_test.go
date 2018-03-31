package opendcm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

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

func TestNewDataSet(t *testing.T) {
	t.Parallel()
	ds := NewDataSet()
	assert.IsType(t, DataSet{}, ds)
	assert.Equal(t, 0, ds.Len())
}

func TestGetElement(t *testing.T) {
	t.Parallel()
	ds := NewDataSet()
	ds[0x00020013] = Element{vr: "CS"}
	e := Element{}
	assert.True(t, ds.GetElement(0x00020013, &e))
	assert.Equal(t, "CS", e.GetVR())
}

func TestGetElementError(t *testing.T) {
	t.Parallel()
	ds := NewDataSet()
	ds[uint32(0x00020013)] = Element{vr: "CS"}
	e := Element{}
	assert.False(t, ds.GetElement(0x0002FFFF, &e))
}

func TestAddElement(t *testing.T) {
	t.Parallel()
	ds := NewDataSet()
	e := NewElement()
	e.vr = "UT"
	e.tag = uint32(0xFFEE0088)
	ds.AddElement(e)
	assert.True(t, ds.GetElement(0xFFEE0088, &e))
}

func TestHasElement(t *testing.T) {
	t.Parallel()
	ds := NewDataSet()
	assert.False(t, ds.HasElement(0xFFEE0088))
	assert.False(t, ds.HasElement(0x00000000))
	ds.AddElement(Element{tag: 0x11223344})
	assert.True(t, ds.HasElement(0x11223344))
}

func TestLen(t *testing.T) {
	t.Parallel()
	ds := NewDataSet()
	assert.Equal(t, 0, ds.Len())
	for i := 1; i <= 16; i++ {
		ds.AddElement(Element{tag: 0x00000000 + uint32(i)})
		assert.Equal(t, i, ds.Len())
	}
}

func TestGetImplementationVersionName(t *testing.T) {
	t.Parallel()
	ds := NewDataSet()
	name := ""
	assert.False(t, ds.GetImplementationVersionName(&name))
	e := NewElement()
	e.tag = 0x00020013
	e.data = []byte("1234")
	ds.AddElement(e)
	assert.True(t, ds.GetImplementationVersionName(&name))
	assert.Equal(t, "1234", name)
}

func TestGetElements(t *testing.T) {
	t.Parallel()
	ds := NewDataSet()
	// add ten elements
	for i := 0; i < 10; i++ {
		e := NewElement()
		e.tag = 0x07F0000 + uint32(i)
		e.vr = "UN"
		ds.AddElement(e)
	}
	elements := ds.GetElements()
	assert.IsType(t, []Element{}, elements)
	assert.Len(t, elements, 10)
}

/*
===============================================================================
   Item
===============================================================================
*/

func TestNewItem(t *testing.T) {
	t.Parallel()
	item := NewItem()
	assert.IsType(t, Item{}, item)
	assert.NotNil(t, item.dataset)
}

func TestGetUnparsed(t *testing.T) {
	t.Parallel()
	item := NewItem()
	item.unparsed = []byte("1234")
	unparsed := item.GetUnparsed()
	assert.Equal(t, []byte("1234"), unparsed)
}

/*
===============================================================================
   Element
===============================================================================
*/

func TestNewElement(t *testing.T) {
	t.Parallel()
	e := NewElement()
	assert.IsType(t, Element{}, e)
}

func TestGetTag(t *testing.T) {
	t.Parallel()
	e := NewElement()
	e.tag = 0x007F1234
	assert.Equal(t, uint32(0x007F1234), e.GetTag())
}

func TestGetVR(t *testing.T) {
	t.Parallel()
	e := NewElement()
	e.vr = "CS"
	assert.Equal(t, "CS", e.GetVR())
}

func TestHasItems(t *testing.T) {
	t.Parallel()
	e := NewElement()
	assert.False(t, e.HasItems())
	e.items = append(e.items, NewItem())
	assert.True(t, e.HasItems())
}

func TestGetItems(t *testing.T) {
	t.Parallel()
	e := NewElement()
	for i := 0; i < 10; i++ {
		e.items = append(e.items, NewItem())
	}
	items := e.GetItems()
	assert.IsType(t, []Item{}, items)
	assert.Len(t, items, 10)
}

func TestGetDataBytes(t *testing.T) {
	t.Parallel()
	e := NewElement()
	e.data = []byte("1234")
	assert.Equal(t, []byte("1234"), e.GetDataBytes())
}

/*
===============================================================================
    ElementReader
===============================================================================
*/

func TestNewElementReader(t *testing.T) {
	t.Parallel()
	r := bin.NewReaderBytes([]byte{}, binary.LittleEndian)
	elr := NewElementReader(r)
	assert.IsType(t, ElementReader{}, elr)
	// should have set implicitvr, littleendian as default
	assert.True(t, elr.IsImplicitVR())
	assert.True(t, elr.IsLittleEndian())
}

func TestIsAndSetLittleEndian(t *testing.T) {
	t.Parallel()
	r := bin.NewReaderBytes([]byte{}, binary.LittleEndian)
	elr := NewElementReader(r)
	elr.SetLittleEndian(true)
	assert.True(t, elr.IsLittleEndian())
	elr.SetLittleEndian(false)
	assert.False(t, elr.IsLittleEndian())
}

func TestIsAndSetImplicitVR(t *testing.T) {
	t.Parallel()
	r := bin.NewReaderBytes([]byte{}, binary.LittleEndian)
	elr := NewElementReader(r)
	elr.SetImplicitVR(true)
	assert.True(t, elr.IsImplicitVR())
	elr.SetImplicitVR(false)
	assert.False(t, elr.IsImplicitVR())
}

// func TestreadULData(t *testing.T) {
// 	t.Parallel()
// 	testCases := []struct {
// 		name                string
// 		expectedItemsLength int
// 		shouldParseElements bool
// 		expectedItems       []Item
// 		byteOrder           binary.ByteOrder
// 		bytes               []byte
// 	}{
// 		{
// 			name:                "WithUndefinedLengthItem",
// 			shouldParseElements: true,
// 			expectedItems: []Item{
// 				{
// 					dataset: DataSet{
// 						0x7F011234: {
// 							tag:     0x7F011234,
// 							vr:      "",
// 							data:    []byte("Leo\x00"),
// 							datalen: 4,
// 						},
// 					},
// 				},
// 			},
// 			bytes:     validUL2,
// 			byteOrder: binary.LittleEndian,
// 		},
// 		{
// 			name:                "WithDefinedLengthItem",
// 			shouldParseElements: true,
// 			expectedItems: []Item{
// 				{
// 					dataset: DataSet{
// 						0x7F011234: {
// 							tag:     0x7F011234,
// 							vr:      "",
// 							data:    []byte("Leo\x00"),
// 							datalen: 4,
// 						},
// 					},
// 				},
// 			},
// 			bytes:     validUL1,
// 			byteOrder: binary.LittleEndian,
// 		},
// 		{
// 			name:                "WithZeroLengthItem",
// 			shouldParseElements: true,
// 			expectedItems:       []Item{},
// 			bytes: []byte{
// 				0xFE, 0xFF, 0x00, 0xE0, // StartItem Tag
// 				0x00, 0x00, 0x00, 0x00, // Item total length: 0
// 				0xFE, 0xFF, 0xDD, 0xE0, // SequenceDelimItem
// 				0x00, 0x00, 0x00, 0x00, // Filler: 4 bytes
// 			},
// 			byteOrder: binary.LittleEndian,
// 		},
// 	}
// 	for _, testCase := range testCases {
// 		t.Run(t.Name()+testCase.name, func(t *testing.T) {
// 			r := bin.NewReaderBytes(testCase.bytes, binary.LittleEndian)
// 			elr := NewElementReader(r)
// 			items := make([]Item, 0)
// 			assert.NoError(t, elr.readULData(testCase.shouldParseElements, &items))
// 			assert.Equal(t, len(testCase.expectedItems), len(items))

// 			for i, item := range testCase.expectedItems {
// 				// item should be matching
// 				assert.Equal(t, item, items[i])
// 			}

// 		})

// 	}
// }
// func TestreadULDataError(t *testing.T) {
// 	t.Parallel()

// 	testCases := []struct {
// 		name                string
// 		bytes               []byte
// 		shouldParseElements bool
// 		byteOrder           binary.ByteOrder
// 	}{
// 		{
// 			// missing the initial ItemTag
// 			name:                "NoItemTag",
// 			bytes:               validUL1[:3],
// 			shouldParseElements: true,
// 			byteOrder:           binary.LittleEndian,
// 		},
// 		{
// 			// first four bytes are not ItemTag
// 			name:                "NotItemTag",
// 			bytes:               make([]byte, 4),
// 			shouldParseElements: true,
// 			byteOrder:           binary.LittleEndian,
// 		},
// 		{
// 			// ItemTag is OK, but its length cannot be read
// 			name:                "NoItemTagLength",
// 			bytes:               validUL1[:4],
// 			shouldParseElements: true,
// 			byteOrder:           binary.LittleEndian,
// 		},
// 		{
// 			// ItemTag accepted, but its contained element (of defined length) cannot be read
// 			name:                "CorruptDLElement",
// 			bytes:               validUL1[:8],
// 			shouldParseElements: true,
// 			byteOrder:           binary.LittleEndian,
// 		},
// 		{
// 			// cannot discard last four bytes following SeqDelimItem
// 			name:                "MissingSeqDelimItemLength",
// 			bytes:               validUL1[:len(validUL1)-4],
// 			shouldParseElements: true,
// 			byteOrder:           binary.LittleEndian,
// 		},
// 		{
// 			// item with undefined length has an invalid element
// 			name:                "CorruptULElement",
// 			bytes:               validUL2[:16],
// 			shouldParseElements: true,
// 			byteOrder:           binary.LittleEndian,
// 		},
// 		{
// 			// element length exceeding reader size
// 			name: "OverflowElementLength",
// 			bytes: []byte{
// 				0xFE, 0xFF, 0x00, 0xE0, // StartItem Tag
// 				0x0C, 0x00, 0x00, 0x00, // Item total length: 12 bytes
// 				0x01, 0x7F, 0x34, 0x12, // (7F01,1234) Tag
// 				0xFF, 0x00, 0x00, 0x00, // Length: 256 bytes
// 				0x4C, 0x65, 0x6F, 0x00, // Data: "Leo"+NULL
// 				0xFE, 0xFF, 0xDD, 0xE0, // SequenceDelimItem
// 				0x00, 0x00, 0x00, 0x00, // Filler: 4 bytes
// 			},
// 			shouldParseElements: true,
// 			byteOrder:           binary.LittleEndian,
// 		},
// 		{
// 			// unparsed data length exceeding reader size
// 			name: "OverflowDataLength",
// 			bytes: []byte{
// 				0xFE, 0xFF, 0x00, 0xE0, // StartItem Tag
// 				0xFF, 0x00, 0x00, 0x00, // Item total length: 256 bytes
// 				0xFE, 0xFF, 0xDD, 0xE0, // SequenceDelimItem
// 				0x00, 0x00, 0x00, 0x00, // Filler: 4 bytes
// 			},
// 			shouldParseElements: false,
// 			byteOrder:           binary.LittleEndian,
// 		},
// 	}

// 	for _, testCase := range testCases {
// 		t.Run(t.Name()+testCase.name, func(t *testing.T) {
// 			r := bin.NewReaderBytes(testCase.bytes, testCase.byteOrder)
// 			elr := NewElementReader(r)
// 			items := make([]Item, 0)
// 			assert.Error(t, elr.readULData(testCase.shouldParseElements, &items))
// 		})
// 	}
// }

func TestReadElementVR(t *testing.T) {
	t.Parallel()
	// without implicit VR
	for _, vr := range RecognisedVRs {
		t.Run(t.Name()+"Explicit("+vr+")", func(t *testing.T) {
			r := bin.NewReaderBytes([]byte(vr), binary.LittleEndian)
			elr := NewElementReader(r)
			elr.SetImplicitVR(false)
			e := NewElement()
			assert.NoError(t, elr.readElementVR(&e))
			assert.Equal(t, vr, e.GetVR())
		})
	}

	// with implicit VR
	for _, vr := range RecognisedVRs {
		t.Run(t.Name()+"Implicit("+vr+")", func(t *testing.T) {
			r := bin.NewReaderBytes([]byte(vr), binary.LittleEndian)
			elr := NewElementReader(r)
			elr.SetImplicitVR(true)
			e := NewElement()
			assert.NoError(t, elr.readElementVR(&e))
			assert.Equal(t, "", e.GetVR())
		})
	}
}

func TestReadElementVRError(t *testing.T) {
	t.Parallel()
	// Reached EOF
	r := bin.NewReaderBytes([]byte{}, binary.LittleEndian)
	elr := NewElementReader(r)
	elr.SetImplicitVR(false)
	e := NewElement()
	assert.Error(t, elr.readElementVR(&e))

	// Reached EOF during read
	r = bin.NewReaderBytes([]byte{0x51}, binary.LittleEndian)
	elr = NewElementReader(r)
	elr.SetImplicitVR(false)
	e = NewElement()
	assert.Error(t, elr.readElementVR(&e))
}

func TestReadElementTag(t *testing.T) {
	// Note, this also fully covers `ElementReader.readTag`
	t.Parallel()
	testCases := []struct {
		littleEndian bool
		bytes        []byte
		expectedTag  uint32
	}{
		{
			littleEndian: true,
			bytes:        []byte{0xFF, 0xFF, 0xFF, 0xFF},
			expectedTag:  0xFFFFFFFF,
		},
		{
			littleEndian: true,
			bytes:        []byte{0x00, 0x00, 0x34, 0x12},
			expectedTag:  0x00001234,
		},
		{
			littleEndian: true,
			bytes:        []byte{0x21, 0x43, 0x00, 0x00},
			expectedTag:  0x43210000,
		},
		{
			littleEndian: false,
			bytes:        []byte{0xFF, 0xFF, 0xFF, 0xFF},
			expectedTag:  0xFFFFFFFF,
		},
		{
			littleEndian: false,
			bytes:        []byte{0x00, 0x00, 0x12, 0x34},
			expectedTag:  0x00001234,
		},
		{
			littleEndian: false,
			bytes:        []byte{0x43, 0x21, 0x00, 0x00},
			expectedTag:  0x43210000,
		},
	}
	for _, testCase := range testCases {
		r := bin.NewReaderBytes(testCase.bytes, binary.LittleEndian)
		elr := NewElementReader(r)
		elr.SetLittleEndian(testCase.littleEndian)
		e := NewElement()
		assert.NoError(t, elr.readElementTag(&e))
		assert.Equal(t, testCase.expectedTag, e.GetTag())
	}
}

func TestReadElementLength(t *testing.T) {
	t.Parallel()

	// Implicit VR:
	// In implicit VR mode, all length definitions are 32 bits
	r := bin.NewReaderBytes([]byte{
		0xC, 0x00, 0x00, 0x00, // Length: 12 bytes
	}, binary.LittleEndian)
	elr := NewElementReader(r)
	elr.SetImplicitVR(true)
	e := NewElement()
	assert.NoError(t, elr.readElementLength(&e))
	assert.Equal(t, uint32(12), e.datalen)

	// Explicit VR:

	// These VRs have two bytes discarded, then length is uint32
	for _, vr := range []string{"OB", "OW", "SQ", "UN", "UT"} {
		r := bin.NewReaderBytes([]byte{
			0x00, 0x00, // discarded
			0xC, 0x00, 0x00, 0x00, // Length: 12 bytes
		}, binary.LittleEndian)
		elr := NewElementReader(r)
		elr.SetImplicitVR(false)
		e := NewElement()
		e.vr = vr
		assert.NoError(t, elr.readElementLength(&e))
		assert.Equal(t, uint32(12), e.datalen)
	}

	// Other VRs have length as uint16
	r = bin.NewReaderBytes([]byte{
		0xC, 0x00, // Length: 12 bytes
	}, binary.LittleEndian)
	elr = NewElementReader(r)
	elr.SetImplicitVR(false)
	e = NewElement()
	e.vr = "CS"
	assert.NoError(t, elr.readElementLength(&e))
	assert.Equal(t, uint32(12), e.datalen)
}

func TestReadElementLengthError(t *testing.T) {
	t.Parallel()
	// Implicit VR, reader error
	r := bin.NewReaderBytes([]byte{
		0xC, 0x00, // Missing last two bytes of length component
	}, binary.LittleEndian)
	elr := NewElementReader(r)
	elr.SetImplicitVR(true)
	e := NewElement()
	assert.Error(t, elr.readElementLength(&e))

	// Explicit VR, cannot skip two bytes of special VR
	r = bin.NewReaderBytes([]byte{
		0x00,
	}, binary.LittleEndian)
	elr = NewElementReader(r)
	elr.SetImplicitVR(false)
	e = NewElement()
	e.vr = "OW"
	assert.Error(t, elr.readElementLength(&e))

	// Explicit VR, cannot read 32-bit length of special VR
	r = bin.NewReaderBytes([]byte{
		0x00, 0x00, // discarded
		0x01, // Missing last three bytes of length component
	}, binary.LittleEndian)
	elr = NewElementReader(r)
	elr.SetImplicitVR(false)
	e = NewElement()
	e.vr = "OW"
	assert.Error(t, elr.readElementLength(&e))

	// Explicit VR, cannot read 16-bit length of normal VR
	r = bin.NewReaderBytes([]byte{
		0x01, // Missing last byte of length component
	}, binary.LittleEndian)
	elr = NewElementReader(r)
	elr.SetImplicitVR(false)
	e = NewElement()
	e.vr = "CS"
	assert.Error(t, elr.readElementLength(&e))
}

func TestReadItem(t *testing.T) {
	t.Parallel()

	// item with defined length, implicit VR
	// http://dicom.nema.org/dicom/2013/output/chtml/part05/sect_7.5.html#table_7.5-1
	item1 := []byte{
		// 0x88, 0x7F, 0x34, 0x12, // Element Tag: (7F88, 1234)
		// 0x53, 0x51, 0x00, 0x00, // VR: "SQ" + 2 filler bytes
		// 0x18, 0x00, 0x00, 0x00, // Element Length: 24 bytes
		0xFE, 0xFF, 0x00, 0xE0, // 1: ItemStartTag
		0x0C, 0x00, 0x00, 0x00, // 1: Item Length: 12 bytes
		0x66, 0x7F, 0x34, 0x12, // 1: Emb. Element Tag: (7F66, 1234)
		0x04, 0x00, 0x00, 0x00, // 1: Emb. Element Length: 4 bytes,
		0x4C, 0x65, 0x6F, 0x00, // 1: Emb. Element Data: Leo+NULL
	}

	// item with defined length, explicit VR
	item2 := []byte{
		0xFE, 0xFF, 0x00, 0xE0, // 1: ItemStartTag
		0x0C, 0x00, 0x00, 0x00, // 1: Item Length: 12 bytes
		0x66, 0x7F, 0x34, 0x12, // 1: Emb. Element Tag: (7F66, 1234)
		0x53, 0x48, // 1: VR: "SH"
		0x04, 0x00, // 1: Emb. Element Length: 4 bytes,
		0x4C, 0x65, 0x6F, 0x00, // 1: Emb. Element Data: Leo+NULL
	}

	// item with defined length, explicit VR containing two elements
	item3 := []byte{
		0xFE, 0xFF, 0x00, 0xE0, // 1: ItemStartTag
		0x1A, 0x00, 0x00, 0x00, // 1: Item Length: 26 bytes
		0x66, 0x7F, 0x34, 0x12, // 1: Emb. Element Tag: (7F66, 1234)
		0x53, 0x48, // 1: VR: "SH"
		0x04, 0x00, // 1: Emb. Element Length: 4 bytes,
		0x4C, 0x65, 0x6F, 0x00, // 1: Emb. Element Data: Leo+NULL

		0x44, 0x7F, 0x34, 0x12, // 2: Emb. Element Tag: (7F44, 1234)
		0x53, 0x48, // 2: VR: "CS"
		0x06, 0x00, // 2: Emb. Element Length: 6 bytes,
		0x31, 0x31, 0x31, 0x31, // 2: Emb. Element Data: "1111"
		0x31, 0x31,
	}
	r := bytes.NewReader(item1)
	elr := NewElementReader(bin.NewReader(r, binary.LittleEndian))
	elr.SetImplicitVR(true)
	item := NewItem()
	assert.NoError(t, elr.readItem(true, &item))
	assert.Equal(t, 1, item.dataset.Len())
	expectedElement := NewElement()
	expectedElement.tag = 0x7F661234
	expectedElement.data = []byte("Leo\x00")
	expectedElement.datalen = uint32(len(expectedElement.data))
	assert.Equal(t, expectedElement, item.dataset.GetElements()[0])

	r = bytes.NewReader(item2)
	elr = NewElementReader(bin.NewReader(r, binary.LittleEndian))
	elr.SetImplicitVR(false)
	item = NewItem()
	assert.NoError(t, elr.readItem(true, &item))
	assert.Equal(t, 1, item.dataset.Len())
	expectedElement.vr = "SH"
	assert.Equal(t, expectedElement, item.dataset.GetElements()[0])

	r = bytes.NewReader(item3)
	elr = NewElementReader(bin.NewReader(r, binary.LittleEndian))
	elr.SetImplicitVR(false)
	item = NewItem()
	assert.NoError(t, elr.readItem(true, &item))
	assert.Equal(t, 2, item.dataset.Len())
	e := NewElement()
	assert.True(t, item.dataset.GetElement(0x7F661234, &e))
	assert.True(t, item.dataset.GetElement(0x7F441234, &e))
}

func TestReadItemError(t *testing.T) {
	t.Parallel()
	// cannot read start tag
	r := bytes.NewReader([]byte{})
	elr := NewElementReader(bin.NewReader(r, binary.LittleEndian))
	item := NewItem()
	assert.Error(t, elr.readItem(false, &item))

	// tag is not StartItem
	r = bytes.NewReader(make([]byte, 4))
	elr = NewElementReader(bin.NewReader(r, binary.LittleEndian))
	item = NewItem()
	assert.Error(t, elr.readItem(false, &item))

	// cannot read item length
	r = bytes.NewReader([]byte{0xFE, 0xFF, 0x00, 0xE0})
	elr = NewElementReader(bin.NewReader(r, binary.LittleEndian))
	item = NewItem()
	assert.Error(t, elr.readItem(false, &item))

	// cannot read embedded element
	r = bytes.NewReader([]byte{0xFE, 0xFF, 0x00, 0xE0, 0x04, 0x00, 0x00, 0x00})
	elr = NewElementReader(bin.NewReader(r, binary.LittleEndian))
	item = NewItem()
	assert.Error(t, elr.readItem(false, &item))
}

func TestReadElementData(t *testing.T) {
	t.Parallel()
	// Non-SQ:
	e := NewElement()
	e.vr = "SH"
	e.datalen = 4
	buf := []byte("1234")
	elr := NewElementReader(bin.NewReaderBytes(buf, binary.LittleEndian))
	assert.NoError(t, elr.readElementData(&e))
	assert.Equal(t, []byte("1234"), e.GetDataBytes())

	// SQ:
	e = NewElement()
	e.vr = "SQ"
	e.datalen = 20
	buf = []byte{
		0xFE, 0xFF, 0x00, 0xE0, // 1: ItemStartTag
		0x0C, 0x00, 0x00, 0x00, // 1: Item Length: 12 bytes
		0x66, 0x7F, 0x34, 0x12, // 1: Emb. Element Tag: (7F66, 1234)
		0x53, 0x48, // 1: VR: "SH"
		0x04, 0x00, // 1: Emb. Element Length: 4 bytes,
		0x4C, 0x65, 0x6F, 0x00, // 1: Emb. Element Data: Leo+NULL
	}
	elr = NewElementReader(bin.NewReaderBytes(buf, binary.LittleEndian))
	elr.SetImplicitVR(false)
	assert.NoError(t, elr.readElementData(&e))
	assert.Len(t, e.GetItems(), 1)
}

func TestReadElementDataError(t *testing.T) {
	t.Parallel()
	// Undefined Length
	// With no actual data
	r := bin.NewReaderBytes([]byte{}, binary.LittleEndian)
	elr := NewElementReader(r)
	e := NewElement()
	e.datalen = 0xFFFFFFFF
	assert.Error(t, elr.readElementData(&e))

	// Defined Length
	//With no actual data
	r = bin.NewReaderBytes([]byte{}, binary.LittleEndian)
	elr = NewElementReader(r)
	e = NewElement()
	e.datalen = 0x12
	assert.Error(t, elr.readElementData(&e))
}

func TestReadElement(t *testing.T) {
	t.Parallel()

	// ExplicitVR, LittleEndian
	validElementBytes := []byte{
		0x01, 0xFE, 0x88, 0x88, // Tag (FE01, 8888)
		0x53, 0x48, // VR: "SH"
		0x04, 0x00, 0x00, 0x00, // Length: 4 bytes
		0x4C, 0x65, 0x6F, 0x00, // Data: "Leo"+NULL
	}
	r := bin.NewReaderBytes(validElementBytes, binary.LittleEndian)
	elr := NewElementReader(r)
	elr.SetImplicitVR(false)
	elr.SetLittleEndian(true)
	e := NewElement()
	assert.NoError(t, elr.ReadElement(&e))
}

func TestReadElementError(t *testing.T) {
	t.Parallel()

	validElementBytes := []byte{
		0x01, 0xFE, 0x88, 0x88, // Tag (FE01, 8888)
		0x53, 0x48, // VR: "SH"
		0x04, 0x00, // Length: 2 bytes
		0x4C, 0x65, 0x6F, 0x00, // Data: "Leo"+NULL
	}

	// error reading tag
	r := bin.NewReaderBytes(validElementBytes[:3], binary.LittleEndian)
	elr := NewElementReader(r)
	elr.SetImplicitVR(false)
	elr.SetLittleEndian(true)
	e := NewElement()
	assert.Error(t, elr.ReadElement(&e))

	// error reading VR
	r = bin.NewReaderBytes(validElementBytes[:4], binary.LittleEndian)
	elr = NewElementReader(r)
	elr.SetImplicitVR(false)
	elr.SetLittleEndian(true)
	e = NewElement()
	assert.Error(t, elr.ReadElement(&e))

	// error reading length
	r = bin.NewReaderBytes(validElementBytes[:7], binary.LittleEndian)
	elr = NewElementReader(r)
	elr.SetImplicitVR(false)
	elr.SetLittleEndian(true)
	e = NewElement()
	assert.Error(t, elr.ReadElement(&e))

	// error reading data
	r = bin.NewReaderBytes(validElementBytes[:10], binary.LittleEndian)
	elr = NewElementReader(r)
	elr.SetImplicitVR(false)
	elr.SetLittleEndian(true)
	e = NewElement()
	assert.Error(t, elr.ReadElement(&e))
}

func TestDetermineEncoding(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name         string
		implicitVR   bool
		littleEndian bool
		input        []byte
	}{
		{
			name:         "ImplicitVRLittleEndian",
			implicitVR:   true,
			littleEndian: true,
			// 0x 00 18 51 00: Patient Position
			input: []byte{0x18, 0x00, 0x00, 0x51, 0x00, 0x00},
		},
		{
			name:         "ExplicitVRLittleEndian",
			implicitVR:   false,
			littleEndian: true,
			// 0x 00 18 51 00: Patient Position
			input: []byte{0x18, 0x00, 0x00, 0x51, 0x53, 0x48},
		},
		{
			name:         "ImplicitVRBigEndian",
			implicitVR:   true,
			littleEndian: false,
			// 0x 00 18 51 00: Patient Position
			input: []byte{0x00, 0x18, 0x51, 0x00, 0x00, 0x00},
		},
		{
			name:         "ExplicitVRBigEndian",
			implicitVR:   false,
			littleEndian: false,
			// 0x 00 18 51 00: Patient Position
			input: []byte{0x00, 0x18, 0x51, 0x00, 0x53, 0x48},
		},
	}
	for _, testCase := range testCases {
		t.Run(t.Name()+testCase.name, func(t *testing.T) {
			elr := NewElementReader(bin.NewReader(blackHole, binary.LittleEndian))
			assert.NoError(t, elr.determineEncoding(testCase.input))
			assert.Equal(t, testCase.implicitVR, elr.IsImplicitVR())
			assert.Equal(t, testCase.littleEndian, elr.IsLittleEndian())
		})
	}
}

func TestDetermineEncodingError(t *testing.T) {
	t.Parallel()
	// not enough bytes to determine encoding
	elr := NewElementReader(bin.NewReader(blackHole, binary.LittleEndian))
	assert.Error(t, elr.determineEncoding(make([]byte, 5)))
	assert.Error(t, elr.determineEncoding([]byte{}))
}

/*
===============================================================================
    ElementWriter
===============================================================================
*/

func TestNewElementWriter(t *testing.T) {
	t.Parallel()
	w := bytes.NewBuffer([]byte{})
	elw := NewElementWriter(w)
	assert.IsType(t, ElementWriter{}, elw)
	// should have set implicitvr, littleendian as default
	assert.True(t, elw.IsImplicitVR())
	assert.True(t, elw.IsLittleEndian())
}

func TestWriterIsAndSetLittleEndian(t *testing.T) {
	t.Parallel()
	w := bytes.NewBuffer([]byte{})
	elw := NewElementWriter(w)
	elw.SetLittleEndian(true)
	assert.True(t, elw.IsLittleEndian())
	elw.SetLittleEndian(false)
	assert.False(t, elw.IsLittleEndian())
}

func TestWriterIsAndSetImplicitVR(t *testing.T) {
	t.Parallel()
	w := bytes.NewBuffer([]byte{})
	elw := NewElementWriter(w)
	elw.SetImplicitVR(true)
	assert.True(t, elw.IsImplicitVR())
	elw.SetImplicitVR(false)
	assert.False(t, elw.IsImplicitVR())
}

func TestWriteElementTag(t *testing.T) {
	t.Parallel()

	// Little Endian
	w := bytes.NewBuffer([]byte{})
	elw := NewElementWriter(w)
	e := NewElement()
	e.tag = uint32(0x7FE00080)
	assert.NoError(t, elw.writeElementTag(e))

	// assert tag is equal
	output := w.Bytes()
	elr := NewElementReader(bin.NewReaderBytes(output, binary.LittleEndian))
	outEl := NewElement()
	assert.NoError(t, elr.readElementTag(&outEl))
	assert.Equal(t, e.GetTag(), outEl.GetTag())

	// Big Endian
	elw.SetLittleEndian(false)
	w.Reset()
	assert.NoError(t, elw.writeElementTag(e))
	output = w.Bytes()
	elr = NewElementReader(bin.NewReaderBytes(output, binary.LittleEndian))
	elr.SetLittleEndian(false)
	outEl = NewElement()
	assert.NoError(t, elr.readElementTag(&outEl))
	assert.Equal(t, e.GetTag(), outEl.GetTag())
}

func TestWriteElementTagError(t *testing.T) {
	t.Parallel()
	elw := NewElementWriter(&failAfterN{failAfter: 0})
	e := NewElement()
	e.tag = 0xFFFF0000
	assert.Error(t, elw.writeElementTag(e))
}

func TestWriteElementVR(t *testing.T) {
	t.Parallel()

	// Explicit VR
	w := bytes.NewBuffer([]byte{})
	elw := NewElementWriter(w)
	elw.SetImplicitVR(false)
	e := NewElement()
	e.vr = "SH"
	assert.NoError(t, elw.writeElementVR(e))

	// assert vr is equal
	output := w.Bytes()
	elr := NewElementReader(bin.NewReaderBytes(output, binary.LittleEndian))
	elr.SetImplicitVR(false)
	outEl := NewElement()
	assert.NoError(t, elr.readElementVR(&outEl))
	assert.Equal(t, e.GetVR(), outEl.GetVR())

	// Implicit VR
	elw.SetImplicitVR(true)
	w.Reset()
	assert.NoError(t, elw.writeElementVR(e))
	output = w.Bytes()
	elr = NewElementReader(bin.NewReaderBytes(output, binary.LittleEndian))
	elr.SetImplicitVR(true)
	outEl = NewElement()
	assert.NoError(t, elr.readElementVR(&outEl))
	assert.Equal(t, "", outEl.GetVR())
}

func TestWriteElementLength(t *testing.T) {
	t.Parallel()
	/*
		TODO:
			ImplicitVR
			ExplicitVR, OB/OW/SQ/UN/UT (special VRs)
			ExplicitVR, other VR (normals)
	*/
	// ImplicitVR
	w := bytes.NewBuffer([]byte{})
	elw := NewElementWriter(w)
	elw.SetImplicitVR(true)
	e := NewElement()
	e.datalen = 200
	assert.NoError(t, elw.writeElementLength(e))

	// assert vr is equal
	output := w.Bytes()
	elr := NewElementReader(bin.NewReaderBytes(output, binary.LittleEndian))
	elr.SetImplicitVR(true)
	outEl := NewElement()
	assert.NoError(t, elr.readElementLength(&outEl))
	assert.Equal(t, e.datalen, outEl.datalen)

	// ExplicitVR, OB/OW/SQ/UN/UT (special VRs)
	elw.SetImplicitVR(false)
	for _, vr := range []string{"OB", "OW", "SQ", "UN", "UT"} {
		w.Reset()
		e.vr = vr
		outEl.vr = vr
		assert.NoError(t, elw.writeElementLength(e))
		output := w.Bytes()
		elr = NewElementReader(bin.NewReaderBytes(output, binary.LittleEndian))
		elr.SetImplicitVR(false)
		assert.NoError(t, elr.readElementLength(&outEl))
		assert.Equal(t, e.datalen, outEl.datalen)
	}

	// ExplicitVR, other VR (normals)
	w.Reset()
	e.vr = "SH"
	outEl.vr = "SH"
	assert.NoError(t, elw.writeElementLength(e))
	output = w.Bytes()
	elr = NewElementReader(bin.NewReaderBytes(output, binary.LittleEndian))
	elr.SetImplicitVR(false)
	assert.NoError(t, elr.readElementLength(&outEl))
	assert.Equal(t, e.datalen, outEl.datalen)
}

func TestWriteElementLengthError(t *testing.T) {
	t.Parallel()
	e := NewElement()
	e.vr = "OW" // special VR
	e.datalen = 9000
	elw := NewElementWriter(&failAfterN{failAfter: 0})
	elw.SetImplicitVR(false)
	assert.Error(t, elw.writeElementLength(e))
}

// func TestWriteElement(t *testing.T) {
// 	t.Parallel()

// 	e := NewElement()
// 	e.vr = "SH"
// 	e.tag = 0xFFE81234
// 	e.datalen = 4
// 	e.data = []byte("1234")

// 	w := bytes.NewBuffer([]byte{})
// 	elw := NewElementWriter(w)
// 	elw.SetImplicitVR(false)
// 	assert.NoError(t, elw.WriteElement(e))

// 	// assert data is equal to that obtained via ElementReader
// 	output := w.Bytes()
// 	outEl := NewElement()
// 	elr := NewElementReader(bin.NewReaderBytes(output, binary.LittleEndian))
// 	elr.SetImplicitVR(false)
// 	assert.NoError(t, elr.ReadElement(&outEl))
// 	assert.Equal(t, e, outEl)
// }

func TestWriteElementError(t *testing.T) {
	t.Parallel()

	e := NewElement()
	e.vr = "SH"
	e.tag = 0xFFE81234
	e.datalen = 4
	e.data = []byte("1234")

	// error writing tag
	elw := NewElementWriter(&failAfterN{failAfter: 0})
	elw.SetImplicitVR(false)
	assert.Error(t, elw.WriteElement(e))

	// error writing vr
	elw = NewElementWriter(&failAfterN{failAfter: 4})
	elw.SetImplicitVR(false)
	assert.Error(t, elw.WriteElement(e))

	// error writing length
	elw = NewElementWriter(&failAfterN{failAfter: 6})
	elw.SetImplicitVR(false)
	assert.Error(t, elw.WriteElement(e))

	// // error writing contents
	// TODO
	// elw = NewElementWriter(&failAfterN{failAfter: 8})
	// elw.SetImplicitVR(false)
	// assert.Error(t, elw.WriteElement(e))
}

// func TestWriteULData(t *testing.T) {
// 	t.Parallel()

// 	// element with undefined length, containing an item with
// 	// unparsed data
// 	e := NewElement()
// 	item := NewItem()
// 	item.unparsed = []byte("1234")
// 	e.items = append(e.items, item)

// 	w := bytes.NewBuffer([]byte{})
// 	elw := NewElementWriter(w)
// 	assert.NoError(t, elw.writeULData(e))
// 	output := w.Bytes()

// 	elr := NewElementReader(bin.NewReaderBytes(output, binary.LittleEndian))
// 	items := make([]Item, 0)
// 	assert.NoError(t, elr.readULData(false, &items))
// }
