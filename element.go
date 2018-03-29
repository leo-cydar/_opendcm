// Package opmmdcm provides methods for working with DICOM data
package opendcm

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/b71729/bin"
)

/*
===============================================================================
    DataSet
===============================================================================
*/

// DataSet represents a single Data Set,
// as per: http://dicom.nema.org/dicom/2013/output/chtml/part10/sect_7.2.html
type DataSet map[uint32]Element

// GetElement attempts to write the element indexed by `tag` into `dst`
// its return value indicates whether the DataSet contains said `tag`.
func (ds *DataSet) GetElement(tag uint32, dst *Element) bool {
	if e, found := (*ds)[tag]; found {
		*dst = e
		return true
	}
	return false
}

// AddElement adds Element `e`
func (ds *DataSet) AddElement(e Element) {
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
	e := Element{}
	if found := ds.GetElement(0x00020013, &e); !found {
		return false
	}
	*dst = string(e.GetDataBytes())
	return true
}

// GetElements returns all elements in the data set as a flat slice.
//
// Note: this is quite an expensive operation and may incur many allocations
func (ds *DataSet) GetElements() []Element {
	elements := []Element{}
	for _, e := range *ds {
		elements = append(elements, e)
	}
	return elements
}

// NewDataSet returns a fresh DataSet
func NewDataSet() DataSet {
	return make(DataSet, 0)
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
	unparsed []byte
}

// NewItem returns a fresh Item with a blank data set.
func NewItem() Item {
	return Item{
		dataset: NewDataSet(),
	}
}

// GetUnparsed returns the "unparsed" data within an Item.
//
// An item may be unparsed if for instance its source VR was not SQ.
// Main example being PixelData: This could for instance be of OW VR,
// but have undefined length, and as such, have "Items".
func (i *Item) GetUnparsed() []byte {
	return i.unparsed
}

/*
===============================================================================
    Element
===============================================================================
*/

// Element represents a Data Element,
// as per http://dicom.nema.org/dicom/2013/output/chtml/part05/chapter_7.html#sect_7.1
type Element struct {
	tag     uint32
	vr      string
	data    []byte
	datalen uint32
	items   []Item
}

// GetTag returns the Element's "Tag" component
func (e *Element) GetTag() uint32 {
	return e.tag
}

// GetVR returns the Element's "VR" component
func (e *Element) GetVR() string {
	return e.vr
}

// HasItems returns whether the element contains nested items
func (e *Element) HasItems() bool {
	return len(e.items) > 0
}

// GetItems returns nested items within this element
func (e *Element) GetItems() []Item {
	return e.items
}

// GetDataBytes will likely be removed / modified.
func (e *Element) GetDataBytes() []byte {
	return e.data
}

// NewElement returns a fresh Element
func NewElement() Element {
	return Element{}
}

/*
===============================================================================
    ElementReader
===============================================================================
*/

// ElementReader extends `bin.Reader` to export methods to assist in
// decoding DICOM Elements, i.e. "ReadElement".
type ElementReader struct {
	br       bin.Reader
	implicit bool
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

// readUndefinedLength attempts to read any number of items that are present
// inside the current Element.
//
// If `readElements` is true, each embedded Item will be expected to contain complete Elements
// , as would be expected if the source VR is "SQ".
// If `readElements` is false, the data inside each item will not be decoded.
// This may be the case if the source VR is for instance OB/OW.
//
// Should be careful calling this, as it assumes specific Reader offset.
func (elr *ElementReader) readUndefinedLength(readElements bool, dst *[]Item) error {
	// read undefined length element
	for {
		item := NewItem()
		if elr.err = elr.readTag(&elr.ui32); elr.err != nil {
			return elr.err
		}

		// check if tag is SequenceDelimitationItem (cause for exiting loop)
		if elr.ui32 == 0xFFFEE0DD {
			if elr.err = elr.br.Discard(4); elr.err != nil {
				return elr.err
			}
			break
		}

		// if tag is not StartItem, something has gone wrong
		if elr.ui32 != 0xFFFEE000 {
			return fmt.Errorf("elr.ui32 = %08X (!= 0xFFFEE000)", elr.ui32)
		}

		// read element length (always 4 bytes here)
		if elr.err = elr.br.ReadUint32(&elr.ui32); elr.err != nil {
			return elr.err
		}

		// an item may be of undefined length; in which case, it can contain
		// multiple elements:
		if elr.ui32 == 0xFFFFFFFF {
			for {
				e := Element{}
				if elr.err = elr.ReadElement(&e); elr.err != nil {
					return elr.err
				}
				if e.GetTag() == 0xFFFEE00D {
					// reached end of item
					break
				}
				item.dataset.AddElement(e)
			}
		} else {
			if elr.ui32 == 0 {
				continue
				/* Turns out the data set had bytes:
				   (40 00 08 00) (53 51)  00 00 (FF FF  FF FF) (FE FF  00 E0) (00 00  00 00) (FE FF  DD E0) 00 00
				   (4b: tag)     (2b:SQ)        (4b: un.len)   (4b:itm start) (4b: 0 len)    (4b: seq end)
				   Therefore, the item genuinely had length of zero.
				   This condition accounts for this possibility.
				*/
			}
			if readElements {
				e := Element{}
				if elr.err = elr.ReadElement(&e); elr.err != nil {
					return elr.err
				}
				item.dataset.AddElement(e)
			} else {
				item.unparsed = make([]byte, elr.ui32)
				if elr.err = elr.br.ReadBytes(item.unparsed); elr.err != nil {
					return elr.err
				}
			}
		}
		*dst = append(*dst, item)
	}
	return nil
}

// readElementVR attempts to read/decode the "VR" component of an Element
// into `dst`.
//
// Should be careful calling this, as it assumes specific Reader offset.
func (elr *ElementReader) readElementVR(dst *Element) error {
	if !elr.IsImplicitVR() {
		if elr.err = elr.br.ReadBytes(elr._1kb[:2]); elr.err != nil {
			return elr.err
		}
		dst.vr = string(elr._1kb[:2])
	}
	return nil
}

// readElementTag attempts to read+decode the "Tag" component of an Element
// into `dst`.
//
// Should be careful calling this, as it assumes specific Reader offset.
func (elr *ElementReader) readElementTag(dst *Element) error {
	return elr.readTag(&dst.tag)
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
			if elr.err = elr.br.ReadUint32(&dst.datalen); elr.err != nil {
				return elr.err
			}
		default:
			if elr.err = elr.br.ReadUint16(&elr.ui16); elr.err != nil {
				return elr.err
			}
			dst.datalen = uint32(elr.ui16)
		}
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
	if dst.datalen == 0xFFFFFFFF { // undefined length
		return elr.readUndefinedLength(dst.GetVR() == "SQ", &dst.items)
	}
	dst.data = make([]byte, dst.datalen)
	return elr.br.ReadBytes(dst.data)
}

// ReadElement attempts to completely read an element into `dst`.
//
// All types of elements are expected to be compatible.
func (elr *ElementReader) ReadElement(dst *Element) error {
	// read tag
	if elr.err = elr.readElementTag(dst); elr.err != nil {
		return elr.err
	}

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

// readTagattempts to read/decode a dicom "Tag" from the reader into `dst`.
//
// Should be careful calling this, as it assumes specific Reader offset.
func (elr *ElementReader) readTag(dst *uint32) error {
	if elr.err = elr.br.ReadBytes(elr._1kb[:4]); elr.err != nil {
		return elr.err
	}
	_ = elr._1kb[3] // bounds check hint to compiler; see golang.org/issue/14808
	if elr.IsLittleEndian() {
		*dst = uint32(elr._1kb[2]) |
			uint32(elr._1kb[3])<<8 |
			uint32(elr._1kb[0])<<16 |
			uint32(elr._1kb[1])<<24
	} else {
		*dst = uint32(elr._1kb[3]) |
			uint32(elr._1kb[2])<<8 |
			uint32(elr._1kb[1])<<16 |
			uint32(elr._1kb[0])<<24
	}
	return nil
}

// determineEncoding attempts to determine the current encoding
// (Implicit/Explicit VR, Big/Little Endian)
// `buf` should be of length six.
func (elr *ElementReader) determineEncoding(buf []byte) error {
	if len(buf) != 6 {
		return errors.New("determineEncoding(buf): need six bytes")
	}
	// here we need six bytes: four for tag, and two for VR
	elr.ui16 = binary.LittleEndian.Uint16(buf[0:2])
	elr.SetLittleEndian(elr.ui16 < 2000 || elr.ui16 == 0x7FE0)
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
	elr.SetImplicitVR(elr._bool)
	Debugf("Determined Encoding: ImplicitVR: %v, LittleEndian: %v", elr.IsImplicitVR(), elr.IsLittleEndian())
	return nil
}

// NewElementReader returns a fresh ElementReader set up to use `source`
// for its data.
//
// For futureproofing, it is suggested to use these constructors rather than
// manually creating an instance (i.e. `elr := ElementReader{}`)
func NewElementReader(source bin.Reader) (er ElementReader) {
	er = ElementReader{
		br: source,
	}
	// ElementReader defaults to Implicit VR Little Endian: Default Transfer Syntax for DICOM
	er.SetImplicitVR(true)
	er.SetLittleEndian(true)
	return er
}

/*
===============================================================================
    ElementWriter
===============================================================================
*/

// ElementWriter extends `bin.Writer` to export methods to assist in
// encoding DICOM Elements, i.e. "WriteElement".
type ElementWriter struct {
	bw       bin.Writer
	implicit bool
	tmpBuffers
}

// NewElementWriter returns a fresh ElementWriter set up to write to `dest`.
//
// For futureproofing, it is suggested to use these constructors rather than
// manually creating an instance (i.e. `elw := ElementWriter{}`)
func NewElementWriter(dest io.Writer) ElementWriter {
	ew := ElementWriter{
		bw: bin.NewWriter(dest, binary.LittleEndian),
	}
	// ElementWriter defaults to Implicit VR Little Endian: Default Transfer Syntax for DICOM
	ew.SetImplicitVR(true)
	ew.SetLittleEndian(true)
	return ew
}

// IsLittleEndian returns whether this ElementWriter is set to encode
// data according to Little Endian byte ordering.
func (elw *ElementWriter) IsLittleEndian() bool {
	return elw.bw.GetByteOrder() == binary.LittleEndian
}

// SetLittleEndian setswhether this ElementWriter should encode
// data according to Little Endian byte ordering.
func (elw *ElementWriter) SetLittleEndian(isLittleEndian bool) {
	if isLittleEndian {
		elw.bw.SetByteOrder(binary.LittleEndian)
	} else {
		elw.bw.SetByteOrder(binary.BigEndian)
	}
}

// IsImplicitVR returns whether this ElementWriter is set to encode
// data according to the VR component being implicitly defined
func (elw *ElementWriter) IsImplicitVR() bool {
	return elw.implicit
}

// SetImplicitVR returns whether this ElementWriter should encode
// data according to the VR component being implicitly defined
func (elw *ElementWriter) SetImplicitVR(isImplicitVR bool) {
	elw.implicit = isImplicitVR
}

// writeElementTag attempts to write the "Tag" component of `src`
//
// Should be careful calling this, as it assumes specific Writer offset.
func (elw *ElementWriter) writeElementTag(src Element) error {
	if elw.err = elw.bw.WriteUint16(uint16(src.GetTag() >> 16)); elw.err != nil {
		return elw.err
	}
	return elw.bw.WriteUint16(uint16(src.GetTag()))
}

// writeElementVR attempts to write the "VR" component of `src`
//
// Should be careful calling this, as it assumes specific Writer offset.
func (elw *ElementWriter) writeElementVR(src Element) error {
	if !elw.IsImplicitVR() {
		elw._1kb[0] = src.GetVR()[0]
		elw._1kb[1] = src.GetVR()[1]
		return elw.bw.WriteBytes(elw._1kb[:2])
	}
	return nil
}

// writeElementLength attempts to write the "Length" component of `src`
//
// Should be careful calling this, as it assumes specific Writer offset.
func (elw *ElementWriter) writeElementLength(src Element) error {
	if elw.IsImplicitVR() {
		// ImplicitVR: all length definitions are 32 bits
		return elw.bw.WriteUint32(src.datalen)
	}
	// Is it a special VR?
	switch src.GetVR() {
	case "OB", "OW", "SQ", "UN", "UT":
		// write 2 empty bytes
		if elw.err = elw.bw.ZeroFill(2); elw.err != nil {
			return elw.err
		}
		// then length is 32 bits
		return elw.bw.WriteUint32(src.datalen)
	default:
		return elw.bw.WriteUint16(uint16(src.datalen))
	}
}

// writeElementData attempts to write the "Data" component of `src`
// In the event that the length is 0xFFFFFFFF (undefined), embedded contents will
// be encoded, as per: http://dicom.nema.org/dicom/2013/output/chtml/part05/sect_7.5.html
//
// Should be careful calling this, as it assumes specific Writer offset.
func (elw *ElementWriter) writeElementData(src Element) error {
	if src.datalen == 0xFFFFFFFF {
		return errors.New("unsupported")
	}
	return elw.bw.WriteBytes(src.data)
}

// WriteElement attempts to completely write `src`
//
// All types of elements are expected to be compatible.
func (elw *ElementWriter) WriteElement(src Element) error {
	// write tag
	if elw.err = elw.writeElementTag(src); elw.err != nil {
		return elw.err
	}

	// write vr
	if elw.err = elw.writeElementVR(src); elw.err != nil {
		return elw.err
	}

	// write length
	if elw.err = elw.writeElementLength(src); elw.err != nil {
		return elw.err
	}

	// write contents
	return elw.writeElementData(src)
}
