// Package opendcm provides methods for working with DICOM data
package opendcm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"

	"github.com/b71729/bin"
	"github.com/b71729/opendcm/dictionary"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

var (
	// Sequence Delimitation Item (FFFE,E0DD)
	seqDelimTag = uint32(0xFFFEE0DD)

	// Item Delimitation Item (FFFE,E00D),
	itemDelimTag = uint32(0xFFFEE00D)

	// Item (FFFE,E000)
	itemTag = uint32(0xFFFEE000)

	// PixelData (7FE0,0010)
	pixelDataTag = uint32(0x7FE00010)
)

/*
===============================================================================
    `CharacterSet`: Accurate Text Representation
===============================================================================
*/

// CharacterSet provides a link between character encoding, description, and decode + encode functions.
type CharacterSet struct {
	Name        string
	Description string
	Encoding    encoding.Encoding
}

// CharacterSetMap provides a mapping between character set name, and character set characteristics.
var CharacterSetMap = map[string]*CharacterSet{
	"Default":         {Name: "Default", Description: "Unicode (UTF-8)", Encoding: unicode.UTF8},
	"ISO_IR 13":       {Name: "ISO_IR 13", Description: "Japanese", Encoding: japanese.ShiftJIS},
	"ISO_IR 100":      {Name: "ISO_IR 100", Description: "Latin alphabet No. 1", Encoding: charmap.ISO8859_1},
	"ISO_IR 101":      {Name: "ISO_IR 101", Description: "Latin alphabet No. 2", Encoding: charmap.ISO8859_2},
	"ISO_IR 109":      {Name: "ISO_IR 109", Description: "Latin alphabet No. 3", Encoding: charmap.ISO8859_3},
	"ISO_IR 110":      {Name: "ISO_IR 110", Description: "Latin alphabet No. 4", Encoding: charmap.ISO8859_4},
	"ISO_IR 126":      {Name: "ISO_IR 126", Description: "Greek", Encoding: charmap.ISO8859_7},
	"ISO_IR 127":      {Name: "ISO_IR 127", Description: "Arabic", Encoding: charmap.ISO8859_6},
	"ISO_IR 138":      {Name: "ISO_IR 138", Description: "Hebrew", Encoding: charmap.ISO8859_8},
	"ISO_IR 144":      {Name: "ISO_IR 144", Description: "Cyrillic", Encoding: charmap.ISO8859_5},
	"ISO_IR 148":      {Name: "ISO_IR 148", Description: "Latin alphabet No. 5", Encoding: charmap.ISO8859_9},
	"ISO_IR 166":      {Name: "ISO_IR 166", Description: "Thai", Encoding: charmap.Windows874},
	"ISO_IR 192":      {Name: "ISO_IR 192", Description: "Unicode (UTF-8)", Encoding: unicode.UTF8},
	"ISO 2022 IR 6":   {Name: "ISO 2022 IR 6", Description: "ASCII", Encoding: unicode.UTF8},
	"ISO 2022 IR 13":  {Name: "ISO 2022 IR 13", Description: "Japanese (Shift JIS)", Encoding: japanese.ShiftJIS},
	"ISO 2022 IR 87":  {Name: "ISO 2022 IR 87", Description: "Japanese (Kanji)", Encoding: japanese.ISO2022JP},
	"ISO 2022 IR 100": {Name: "ISO 2022 IR 100", Description: "Latin alphabet No. 1", Encoding: charmap.ISO8859_1},
	"ISO 2022 IR 101": {Name: "ISO 2022 IR 101", Description: "Latin alphabet No. 2", Encoding: charmap.ISO8859_2},
	"ISO 2022 IR 109": {Name: "ISO 2022 IR 109", Description: "Latin alphabet No. 3", Encoding: charmap.ISO8859_3},
	"ISO 2022 IR 110": {Name: "ISO 2022 IR 110", Description: "Latin alphabet No. 4", Encoding: charmap.ISO8859_4},
	"ISO 2022 IR 127": {Name: "ISO 2022 IR 127", Description: "Arabic", Encoding: charmap.ISO8859_6},
	"ISO 2022 IR 138": {Name: "ISO 2022 IR 138", Description: "Hebrew", Encoding: charmap.ISO8859_8},
	"ISO 2022 IR 144": {Name: "ISO 2022 IR 144", Description: "Cyrillic", Encoding: charmap.ISO8859_5},
	"ISO 2022 IR 148": {Name: "ISO 2022 IR 148", Description: "Latin alphabet No. 5", Encoding: charmap.ISO8859_9},
	"ISO 2022 IR 149": {Name: "ISO 2022 IR 149", Description: "Korean", Encoding: korean.EUCKR}, // TODO: verify
	"ISO 2022 IR 159": {Name: "ISO 2022 IR 159", Description: "Japanese (Supplementary Kanji)", Encoding: japanese.ISO2022JP},
	"ISO 2022 IR 166": {Name: "ISO 2022 IR 166", Description: "Thai", Encoding: charmap.Windows874},
	"GB18030":         {Name: "GB18030", Description: "Chinese (Simplified)", Encoding: simplifiedchinese.GB18030},
}

/*
===============================================================================
    DataSet
===============================================================================
*/

// DataSet represents a single Data Set,
// as per: http://dicom.nema.org/dicom/2013/output/chtml/part10/sect_7.2.html
type DataSet map[uint32]Element

// GetElement  writes the element indexed by `tag` into `dst`
// its return value indicates whether the DataSet contains said `tag`.
func (ds *DataSet) GetElement(tag uint32, dst *Element) bool {
	if e, found := (*ds)[tag]; found {
		*dst = e
		return true
	}
	return false
}

// GetElementValue writes the element's value indexed by `tag` into `dst`
// its return value (bool) indicates whether the DataSet contains said `tag`.
// its return value (error) indicates whether there are any other problems.
func (ds *DataSet) GetElementValue(tag uint32, dst interface{}) (bool, error) {
	if e, found := (*ds)[tag]; found {
		return true, e.GetValue(dst)
	}

	return false, nil
}

// addElement adds Element `e` to the data set.
func (ds *DataSet) addElement(e Element) {
	(*ds)[e.GetTag()] = e
}

// HasElement returns whether the element indexed by `tag` exists.
func (ds *DataSet) HasElement(tag uint32) bool {
	return ds.GetElement(tag, &Element{})
}

// Len returns the number of elements.
func (ds *DataSet) Len() int {
	return len((*ds))
}

// GetImplementationVersionName is an experimental method to debug
// retrieval of elements from the DataSet. Will likely be removed.
func (ds *DataSet) GetImplementationVersionName(dst *string) bool {
	// initialise a new element to hold version name data
	e := NewElement()
	// check whether the element exists in the dataset map
	if found := ds.GetElement(0x00020013, &e); !found {
		return false
	}
	// if it exists, write the data directly into destination
	*dst = string(e.GetDataBytes())
	return true
}

// GetCharacterSet returns either the character set as defined in (0008,0005),
// or ISO_IR 100 (default character set)
func (ds *DataSet) GetCharacterSet() (cs *CharacterSet) {
	// initialise new element to hold character set value
	e := NewElement()
	var found bool
	// check whether element exists in the dataset map
	if ds.GetElement(0x00080005, &e) {
		sa := []string{}
		e.GetValue(&sa)
		if cs, found = CharacterSetMap[sa[len(sa)-1]]; found {
			return
		}
	}

	cs, _ = CharacterSetMap["Default"]
	return
}

/*
===============================================================================
    Item
===============================================================================
*/

// Item represents an Item, as may be found within nested data sequences,
// as per http://dicom.nema.org/dicom/2013/output/chtml/part05/sect_7.5.html
type Item struct {
	dataset  DataSet
	fragment []byte
}

// NewItem returns a fresh Item with a blank data set.
func NewItem() Item {
	return Item{
		dataset: make(DataSet, 0),
	}
}

// GetFragment returns the "fragment" data within an Item.
//
// An item will have fragment data if, when reading the source data,
// "shouldParseEmbeddedElements" returned false.
//
// The Dicom spec could be clearer of _when_ items will have
// fragments, rather than elements.
// As far as I can see, the only case in practice will be with the PixelData tag.
func (i *Item) GetFragment() []byte {
	return i.fragment
}

/*
===============================================================================
    Element
===============================================================================
*/

// splitCharacterStringVM splits `buffer` using "\" as delimiter.
func splitCharacterStringVM(buffer []byte) [][]byte {
	return bytes.Split(buffer, []byte(`\`))
}

// splitBinaryVM splits `buffer` at `nBytesEach`.
func splitBinaryVM(buffer []byte, nBytesEach int) (splitted [][]byte) {
	pos := 0
	for len(buffer) >= pos+nBytesEach {
		splitted = append(splitted, buffer[pos:(pos+nBytesEach)])
		pos += nBytesEach
	}
	return
}

// Element represents a Data Element,
// as per http://dicom.nema.org/dicom/2013/output/chtml/part05/chapter_7.html#sect_7.1
type Element struct {
	dictEntry      *dictionary.DictEntry
	data           []byte
	isLittleEndian bool
	datalen        uint32
	items          []Item
}

// GetTag returns the Element's "Tag" component
func (e *Element) GetTag() uint32 {
	return e.dictEntry.Tag
}

// GetVR returns the Element's "VR" component
func (e *Element) GetVR() string {
	return e.dictEntry.VR
}

// GetVM returns the Element's "VM" component
func (e *Element) GetVM() string {
	return e.dictEntry.VM
}

// GetName returns the Element's "Name" component
func (e *Element) GetName() string {
	return e.dictEntry.Name
}

// HasItems returns whether the element contains nested items
func (e *Element) HasItems() bool {
	return len(e.items) > 0
}

// GetItems returns nested items within this element
func (e *Element) GetItems() []Item {
	return e.items
}

func (e *Element) supportsType(typ interface{}) bool {
	/*
			TODO:
			"OD", "OF", "OW",
		    "SQ",
	*/
	// in the case that the VR is unknown, take the less disruptive choice: respond with true
	// in practice, we don't know whether it supports, but we need a way of allowing the value to be retrieved.
	if e.GetVR() == "UN" {
		return true
	}
	switch typ.(type) {
	case string, *string, []string, *[]string:
		switch e.GetVR() {
		case "SH", "LO", "ST", "PN", "LT", "UT",
			"IS", "DS", "TM", "DA", "DT", "UI", "CS", "AS", "AE": // These shouldnt be parsed using charset btw
			return true
		}
	case float32, *float32, []float32, *[]float32:
		if e.GetVR() == "FL" {
			return true
		}
	case float64, *float64, []float64, *[]float64:
		if e.GetVR() == "FD" {
			return true
		}
	case int16, *int16, []int16, *[]int16:
		if e.GetVR() == "SS" {
			return true
		}
	case int32, *int32, []int32, *[]int32:
		if e.GetVR() == "SL" {
			return true
		}
	case uint16, *uint16, []uint16, *[]uint16:
		if e.GetVR() == "US" {
			return true
		}
	case uint32, *uint32, []uint32, *[]uint32:
		if e.GetVR() == "UL" || e.GetVR() == "AT" {
			return true
		}
	case []byte, *[]byte:
		// every VR can be expressed as a sequence of bytes
		return true
	}
	return false
}

// GetValue writes the element's "value" component to "dst".
// "dst" should be writable (pointer type)
func (e *Element) GetValue(dst interface{}) error {
	// check whether the VR supports expression as target type
	if !e.supportsType(dst) {
		return fmt.Errorf("GetValue(%s): value of %s cannot be expressed as a %s", reflect.TypeOf(dst), e.dictEntry, reflect.TypeOf(dst))
	}
	switch typedDst := dst.(type) {
	case *string:
		// if VR is textual just return UTF8 string (when a dicom is parsed, using `FromReader`, all text elements
		// are re-encoded into UTF-8 as before the function returns.)
		Debugf("String: %s", e.GetDataBytes())
		*typedDst = string(e.GetDataBytes())
	case *[]string:
		for _, v := range splitCharacterStringVM(e.GetDataBytes()) {
			*typedDst = append(*typedDst, string(v))
		}
	case *[]byte:
		*typedDst = e.GetDataBytes()
	case *[]float32:
		for _, v := range splitBinaryVM(e.GetDataBytes(), 4) {
			if e.isLittleEndian {
				*typedDst = append(*typedDst, math.Float32frombits(binary.LittleEndian.Uint32(v)))
			} else {
				*typedDst = append(*typedDst, math.Float32frombits(binary.BigEndian.Uint32(v)))
			}
		}
	case *float32:
		*typedDst = math.Float32frombits(binary.LittleEndian.Uint32(e.GetDataBytes()[:4]))
	case *[]float64:
		for _, v := range splitBinaryVM(e.GetDataBytes(), 8) {
			if e.isLittleEndian {
				*typedDst = append(*typedDst, math.Float64frombits(binary.LittleEndian.Uint64(v)))
			} else {
				*typedDst = append(*typedDst, math.Float64frombits(binary.BigEndian.Uint64(v)))
			}
		}
	case *float64:
		*typedDst = math.Float64frombits(binary.LittleEndian.Uint64(e.GetDataBytes()[:8]))
	case *[]int16:
		for _, v := range splitBinaryVM(e.GetDataBytes(), 2) {
			if e.isLittleEndian {
				*typedDst = append(*typedDst, int16(binary.LittleEndian.Uint16(v)))
			} else {
				*typedDst = append(*typedDst, int16(binary.BigEndian.Uint16(v)))
			}
		}
	case *int16:
		if e.isLittleEndian {
			*typedDst = int16(binary.LittleEndian.Uint16(e.GetDataBytes()))
		} else {
			*typedDst = int16(binary.BigEndian.Uint16(e.GetDataBytes()))
		}
	case *[]int32:
		for _, v := range splitBinaryVM(e.GetDataBytes(), 4) {
			if e.isLittleEndian {
				*typedDst = append(*typedDst, int32(binary.LittleEndian.Uint32(v)))
			} else {
				*typedDst = append(*typedDst, int32(binary.BigEndian.Uint32(v)))
			}
		}
	case *int32:
		if e.isLittleEndian {
			*typedDst = int32(binary.LittleEndian.Uint32(e.GetDataBytes()))
		} else {
			*typedDst = int32(binary.BigEndian.Uint32(e.GetDataBytes()))
		}
	// if not writable type (pointer), return error
	case bool, string,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, uintptr,
		float32, float64, complex64, complex128:
		return fmt.Errorf("GetValue(%s): destination is not writable", reflect.TypeOf(dst))
	default:
		return fmt.Errorf(`writing to type "%v" is not yet implemented`, reflect.TypeOf(dst))
	}
	return nil
}

// GetDataBytes will likely be removed / modified.
func (e *Element) GetDataBytes() []byte {
	return e.data
}

// NewElement returns a fresh Element
func NewElement() Element {
	// by default, it will be Little Endian
	e := Element{isLittleEndian: true}
	// dictionary entry should be initialised to provide deterministic behaviour
	// in the case that the tag hasn't been assigned.
	// a tag cannot be "FFFFFFFF" according to the dicom spec, so this should be easy to detect.
	e.dictEntry = &dictionary.DictEntry{
		Tag:       0xFFFFFFFF,
		Name:      "UninitialisedMemory",
		NameHuman: "UninitialisedMemory",
		VR:        "UN",
		VM:        "1",
		Retired:   false}
	return e
}

// NewElementWithTag returns a fresh Element with
// its VR, VM, Name and NameHuman pre-looked up according to "t".
func NewElementWithTag(t uint32) Element {
	e := NewElement()
	e.dictEntry, _ = lookupTag(t)
	return e
}

/*
===============================================================================
    ElementReader
===============================================================================
*/

// shouldReadEmbeddedElements is used to determine whether given element "e"
// should, theoretically, contain embedded elements. (if false, it indicates
// that the element will contain "data fragments")
func shouldReadEmbeddedElements(e Element) bool {
	// if tag is PixelData, return false
	return e.GetTag() != pixelDataTag
	// else return true
}

// lookupTag searches for the corresponding `dictionary.DicomDictionary` entry for the given tag uint32
func lookupTag(t uint32) (entry *dictionary.DictEntry, found bool) {
	// attempt to lookup tag in the dictionary
	entry, found = dictionary.DicomDictionary[t]
	// if not found, default to sensible values
	if !found {
		name := fmt.Sprintf("Unknown(%04X,%04X)", uint16(t>>16), uint16(t))
		entry = &dictionary.DictEntry{Tag: t, Name: name, NameHuman: name, VR: "UN", VM: "1", Retired: false}
	}
	return
}

// ElementReader extends `bin.Reader` to export methods to assist in
// decoding DICOM Elements, i.e. "ReadElement".
type ElementReader struct {
	br       bin.Reader
	implicit bool
	charSet  *CharacterSet
	tmpBuffers
}

// IsLittleEndian returns whether this ElementReader is set to parse
// data according to Little Endian byte ordering.
func (elr *ElementReader) IsLittleEndian() bool {
	return elr.br.GetByteOrder() == binary.LittleEndian
}

// SetLittleEndian setswhether this ElementReader should parse
// data according to Little Endian byte ordering.
func (elr *ElementReader) SetLittleEndian(isLittleEndian bool) {
	// set using the "encoding/binary" package
	if isLittleEndian {
		elr.br.SetByteOrder(binary.LittleEndian)
	} else {
		elr.br.SetByteOrder(binary.BigEndian)
	}
}

// IsImplicitVR returns whether this ElementReader is set to parse
// data according to the VR component being implicitly defined
func (elr *ElementReader) IsImplicitVR() bool {
	return elr.implicit
}

// SetImplicitVR returns whether this ElementReader should parse
// data according to the VR component being implicitly defined
func (elr *ElementReader) SetImplicitVR(isImplicitVR bool) {
	elr.implicit = isImplicitVR
}

// readElementVR attempts to read/decode the "VR" component of an Element
// into `dst`.
//
// Should be careful calling this, as it assumes specific Reader offset.
func (elr *ElementReader) readElementVR(dst *Element) error {
	// if Implicit VR, nothing needs to be read
	if elr.IsImplicitVR() {
		return nil
	}
	// otherwise take two bytes from the reader
	if elr.err = elr.br.ReadBytes(elr._1kb[:2]); elr.err != nil {
		return elr.err
	}
	// only overwrite the existing dictionary entry's VR if we have UN
	// and source has something else (has added value)
	if (dst.GetVR() == "UN" || dst.GetVR() == "") && string(elr._1kb[:2]) != "UN" {
		dst.dictEntry.VR = string(elr._1kb[:2])
	}
	return nil
}

// readElementLength attempts to read/decode the "Length" component of an Element
// into `dst`.
//
// Should be careful calling this, as it assumes specific Reader offset.
func (elr *ElementReader) readElementLength(dst *Element) error {
	if elr.IsImplicitVR() {
		// ImplicitVR: all length definitions are 32 bits
		if elr.err = elr.br.ReadUint32(&dst.datalen); elr.err != nil {
			return elr.err
		}
	} else {
		// issue #6: use *source* VR as basis for deciding whether to skip / size of length integer.
		// in explicit VR mode, if the VR is OB, OW, SQ, UN or UT, skip two bytes and read as uint32, else uint16.
		switch dst.GetVR() {
		case "OB", "OW", "SQ", "UN", "UT":
			// skip 2 bytes
			if elr.err = elr.br.Discard(2); elr.err != nil {
				return elr.err
			}
			// and read length as 32 bits
			if elr.err = elr.br.ReadUint32(&dst.datalen); elr.err != nil {
				return elr.err
			}
		default:
			// read length as 16 bits
			if elr.err = elr.br.ReadUint16(&elr.ui16); elr.err != nil {
				return elr.err
			}
			dst.datalen = uint32(elr.ui16)
		}
	}
	return nil
}

// tagFromBytes parses a dicom tag from a block of four bytes.
// If "src" is not of length four, an error will be returned.
func (elr *ElementReader) tagFromBytes(src []byte, dst *uint32) error {
	if len(src) != 4 {
		return errors.New("tagFromBytes requires four bytes")
	}
	if elr.IsLittleEndian() {
		*dst = uint32(src[2]) |
			uint32(src[3])<<8 |
			uint32(src[0])<<16 |
			uint32(src[1])<<24
	} else {
		*dst = uint32(src[3]) |
			uint32(src[2])<<8 |
			uint32(src[1])<<16 |
			uint32(src[0])<<24
	}
	return nil
}

// hasReachedTag returns whether the underlying reader has reached "tag".
// "tag" should be a dicom tag in uin32 format.
// In determining this, it does not forward the reader.
func (elr *ElementReader) hasReachedTag(tag uint32) (bool, error) {
	// peek 4 bytes
	if elr.err = elr.br.Peek(elr._1kb[:4]); elr.err != nil {
		return false, elr.err
	}
	// decode tag from those four bytes
	if elr.err = elr.tagFromBytes(elr._1kb[:4], &elr.ui32); elr.err != nil {
		return false, elr.err
	}
	// return tag == "input_tag"
	return (elr.ui32 == tag), nil
}

// readItemUndefLength attempts to read the "data" component of an item that is of
// "undefined length" from the reader.
// "readEmbeddedElements" specifies whether the method should parse embedded datas as "elements",
// or "data fragments" (i.e. as would be the case with PixelData).
func (elr *ElementReader) readItemUndefLength(readEmbeddedElements bool, dst *Item) error {
	// for
	for {
		// check if we have reached item delimitation tag
		if elr._bool, elr.err = elr.hasReachedTag(itemDelimTag); elr.err != nil {
			return elr.err
		}
		// if so, exit the loop
		if elr._bool == true {
			break
		}
		if readEmbeddedElements {
			// initialise empty element
			e := NewElement()
			if !elr.IsLittleEndian() {
				e.isLittleEndian = false
			}
			// read element(empty_element)
			if elr.err = elr.ReadElement(&e); elr.err != nil {
				return elr.err
			}
			// add element to item.dataset
			dst.dataset.addElement(e)
			continue
		}
		// we are not reading embedded elemebts, instead extend "fragment" by four bytes
		dst.fragment = append(dst.fragment, make([]byte, 4)...)
		// and read from the stream
		if elr.err = elr.br.ReadBytes(dst.fragment[len(dst.fragment)-4:]); elr.err != nil {
			return elr.err
		}
	}
	// discard 8
	return elr.br.Discard(8)
	// finished
}

// readItem attempts to read an item from the reader.
// "readEmbeddedElements" specifies whether the method should parse embedded datas as "elements",
// or "data fragments" (i.e. as would be the case with PixelData).
// This method handles both undefined length and defined length items.
func (elr *ElementReader) readItem(readEmbeddedElements bool, dst *Item) error {
	// read item-tag
	if elr.err = elr.readTag(&elr.ui32); elr.err != nil {
		return elr.err
	}
	// is item-tag not ItemStartTag?
	// not ItemStartTag:
	if elr.ui32 != itemTag {
		// 	raise error
		return errors.New("did not find ItemStartTag")
	}

	// read item-length
	if elr.err = elr.br.ReadUint32(&elr.ui32); elr.err != nil {
		return elr.err
	}
	// is item of undef. length?
	if elr.ui32 == 0xFFFFFFFF {
		// yes:
		// read_item_undefined_length(input)
		if elr.err = elr.readItemUndefLength(readEmbeddedElements, dst); elr.err != nil {
			return elr.err
		}
		return nil
	}

	if elr.ui32 == 0 {
		return nil
		/* Turns out the data set had bytes:
		   (40 00 08 00) (53 51)  00 00 (FF FF  FF FF) (FE FF  00 E0) (00 00  00 00) (FE FF  DD E0) 00 00
		   (4b: tag)     (2b:SQ)        (4b: un.len)   (4b:itm start) (4b: 0 len)    (4b: seq end)
		   Therefore, the item genuinely had length of zero.
		   This condition accounts for this possibility.
		*/
	}

	// if "read_elements":
	if readEmbeddedElements {
		// end_pos = cur_pos + item.length
		endPos := elr.br.GetPosition() + int64(elr.ui32)
		// for cur_pos < end_pos:
		for elr.br.GetPosition() < endPos {
			// 	initialise empty element
			e := NewElement()
			if !elr.IsLittleEndian() {
				e.isLittleEndian = false
			}
			// 	read element(empty element)
			if elr.err = elr.ReadElement(&e); elr.err != nil {
				return elr.err
			}
			// 	add element to "dest".dataset
			dst.dataset.addElement(e)
			// 	continue
		}
		return nil
	}

	// # not reading elements - read bytes and store
	// initialise "dest".fragment to length of element
	dst.fragment = make([]byte, elr.ui32)
	// "dest".fragment <- read len X bytes
	return elr.br.ReadBytes(dst.fragment)
}

// readElementDataUndefLength attempts to read the "data" component of
// an element that is of "undefined length" from the reader.
func (elr *ElementReader) readElementDataUndefLength(dst *Element) error {
	// for
	for {
		// if has_reached_tag(SeqDelimTag), break.
		if elr._bool, elr.err = elr.hasReachedTag(seqDelimTag); elr.err != nil {
			return elr.err
		}
		if elr._bool {
			break
		}
		// initialise empty_item
		item := NewItem()
		// read_item(should_read_embedded_elements("dest"), empty_item)
		elr.readItem(shouldReadEmbeddedElements(*dst), &item)
		// add empty_item to "dest".items
		dst.items = append(dst.items, item)
	}
	// discard 8
	if elr.err = elr.br.Discard(8); elr.err != nil {
		return elr.err
	}
	return nil
}

// readElementData attempts to read/decode the "Data" component of an Element
// into `dst`.
// In the event that the length is 0xFFFFFFFF (undefined), embedded contents will
// be decoded, as per: http://dicom.nema.org/dicom/2013/output/chtml/part05/sect_7.5.html
//
// Should be careful calling this, as it assumes specific Reader offset.
func (elr *ElementReader) readElementData(dst *Element) error {

	// is "dst" of zero length?
	if dst.datalen == 0 {
		return nil
	}

	// is "dest" of undef. length?
	if dst.datalen == 0xFFFFFFFF {
		// read_element_data_undef_length("dest")
		// return
		return elr.readElementDataUndefLength(dst)
	}
	// is "dest" instead a SQ with defined length?
	if dst.GetVR() == "SQ" {
		endPos := elr.br.GetPosition() + int64(dst.datalen)
		for elr.br.GetPosition() < endPos {
			// initialise empty_item
			item := NewItem()
			// read_item(should_read_embedded_elements("dest"), empty_item)
			if elr.err = elr.readItem(shouldReadEmbeddedElements(*dst), &item); elr.err != nil {
				return elr.err
			}
			// add empty_item to "dest".items
			dst.items = append(dst.items, item)
		}
		return nil
	}
	// otherwise, its "defined length, non-SQ", read as arbitrary bytes
	// initialise dest to length of element
	dst.data = make([]byte, dst.datalen)

	// "dest" <- read len X bytes
	if elr.err = elr.br.ReadBytes(dst.data); elr.err != nil {
		return elr.err
	}

	padchars := []byte{0x00, 0x20}
	switch dst.GetVR() {
	case "UI", "OB", "CS", "DS", "IS", "AE", "AS", "DA", "DT", "LO", "LT", "OD", "OF", "OW", "PN", "SH", "ST", "TM", "UT":
		for _, chr := range padchars {
			if dst.data[len(dst.data)-1] == chr {
				dst.data = dst.data[:len(dst.data)-1]
				dst.datalen--
			} else if dst.data[0] == chr { // NOTE: assumes padding will only take place on one side. Should be fine.
				dst.data = dst.data[1:]
				dst.datalen--
			}
		}
	}
	return nil
}

// ReadElement attempts to completely read an element into `dst`.
//
// All types of elements are expected to be compatible.
func (elr *ElementReader) ReadElement(dst *Element) error {
	// read tag
	if elr.err = elr.readTag(&elr.ui32); elr.err != nil {
		return elr.err
	}
	// set element.dictentry to an entry in dictionary
	dst.dictEntry, elr._bool = lookupTag(elr.ui32)

	// read vr
	if elr.err = elr.readElementVR(dst); elr.err != nil {
		return elr.err
	}

	// read length
	if elr.err = elr.readElementLength(dst); elr.err != nil {
		return elr.err
	}

	// read contents
	return elr.readElementData(dst)
}

// readTag attempts to read/decode a dicom "Tag" from the reader into `dst`.
//
// Should be careful calling this, as it assumes specific Reader offset.
func (elr *ElementReader) readTag(dst *uint32) error {
	if elr.err = elr.br.ReadBytes(elr._1kb[:4]); elr.err != nil {
		return elr.err
	}
	return elr.tagFromBytes(elr._1kb[:4], dst)
}

// determineEncoding attempts to determine the current encoding
// (Implicit/Explicit VR, Big/Little Endian)
// `buf` should be of length six.
func (elr *ElementReader) determineEncoding(buf []byte) error {
	// check for six bytes: four for tag, and two for VR
	if len(buf) != 6 {
		return errors.New("determineEncoding(buf): need six bytes")
	}

	// if the upper component of tag is less than 2000, or is 7FE0
	// (PixelData), we can be fairly certain the file is using
	// little endian encoding
	elr.ui16 = binary.LittleEndian.Uint16(buf[0:2])
	elr.SetLittleEndian(elr.ui16 < 2000 || elr.ui16 == 0x7FE0)

	// to determine implicit / explicit VR, check the next two
	// bytes against known VRs
	vrfrombytes := string(buf[4:6])
	elr.SetImplicitVR(true)
	elr._bool = true
	for _, vr := range RecognisedVRs {
		if vr == vrfrombytes {
			// VR found in `buf` matches a known VR -- is likely explicit
			elr._bool = false
			break
		}
	}
	// encoding should have been determined by this stage
	elr.SetImplicitVR(elr._bool)
	//Debugf("Determined Encoding: ImplicitVR: %v, LittleEndian: %v", elr.IsImplicitVR(), elr.IsLittleEndian())
	return nil
}

// NewElementReader returns a fresh ElementReader set up to use `source`
// for its data.
//
// For futureproofing, it is suggested to use these constructors rather than
// manually creating an instance (i.e. `elr := ElementReader{}`)
func NewElementReader(source bin.Reader) (er ElementReader) {
	// create an instance of the element reader with the source set
	er = ElementReader{
		br: source,
	}
	// default to "Implicit VR Little Endian: Default Transfer Syntax for DICOM"
	er.SetImplicitVR(true)
	er.SetLittleEndian(true)
	return er
}
