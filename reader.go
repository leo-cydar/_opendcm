package opendcm

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/b71729/opendcm/dictionary"
	"github.com/rs/zerolog/log"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

/*
===============================================================================
    Configuration
===============================================================================
*/

// DicomReadBufferSize is the number of bytes to be buffered from disk when parsing dicoms
const DicomReadBufferSize = 2 * 1024 * 1024 // 2MB

/*
  By enabling `StrictMode`, the parser will reject DICOM inputs which either:
    - TODO: Contain an element with a value length exceeding the maximum allowed for its VR
    - Contain an element with a value length exceeding the remaining file size. For example incomplete Pixel Data.
*/

// StrictMode controls whether the parser operates in a restricted manner, rejecting potentially corrupt data.
var StrictMode = false

/*
===============================================================================
    Data Types
===============================================================================
*/

type ReaderPool struct {
	reader *bufio.Reader
}

var Nalloc = 0
var Nused = 0
var ReaderPool256k = &sync.Pool{
	New: func() interface{} {
		Nalloc++
		return bufio.NewReaderSize(nil, 256*1024)
	},
}

var readerPool512k = &sync.Pool{
	New: func() interface{} {
		return bufio.NewReaderSize(nil, 512*1024)
	},
}

var readerPool2mb = &sync.Pool{
	New: func() interface{} {
		return bufio.NewReaderSize(nil, 2*1024*1024)
	},
}

func Get256k(src io.Reader) (r *bufio.Reader) {
	Nused++
	r = ReaderPool256k.Get().(*bufio.Reader)
	r.Reset(src)
	return r
}

func Put256k(r *bufio.Reader) {
	ReaderPool256k.Put(r)
}

func Get512k(src io.Reader) (r *bufio.Reader) {
	Nused++
	r = readerPool512k.Get().(*bufio.Reader)
	r.Reset(src)
	return r
}

func Put512k(r *bufio.Reader) {
	readerPool512k.Put(r)
}

func Get2mb(src io.Reader) (r *bufio.Reader) {
	Nused++
	r = readerPool2mb.Get().(*bufio.Reader)
	r.Reset(src)
	return r
}

func Put2mb(r *bufio.Reader) {
	readerPool2mb.Put(r)
}

// Dicom provides a link between components that make up a parsed DICOM file
type Dicom struct {
	FilePath       string
	reader         *bufio.Reader
	elementStream  ElementStream
	Preamble       [128]byte
	TotalMetaBytes int64
	Elements       map[uint32]Element
}

// Element represents a data element (see: NEMA 7.1 Data Elements)
type Element struct {
	*dictionary.DictEntry
	ValueLength         uint32
	ByteLengthTotal     int64
	FileOffsetStart     int64
	value               []byte
	sourceElementStream *ElementStream
	Items               []Item
}

// ByTag implements a sort interface
type ByTag []Element

// Item represents a nested Item within a Sequence (see: NEMA 7.5 Nesting of Data Sets)
type Item struct {
	Elements        map[uint32]Element
	UnknownSections [][]byte
}

// ElementStream provides an abstraction layer around a `*bytes.Reader` to facilitate easier parsing.
type ElementStream struct {
	reader         *bufio.Reader
	readerPos      int64
	readerSize     int64
	TransferSyntax TransferSyntax
	CharacterSet   *CharacterSet
}

// TransferSyntax provides a link between dictionary `UIDEntry` and encoding (byteorder, implicit/explicit VR)
type TransferSyntax struct {
	UIDEntry *dictionary.UIDEntry
	Encoding *Encoding
}

// Encoding represents the expected encoding of dicom attributes. See transferSyntaxToEncodingMap.
type Encoding struct {
	ImplicitVR   bool
	LittleEndian bool
}

// CharacterSet provides a link between character encoding, description, and decode + encode functions.
type CharacterSet struct {
	Name        string
	Description string
	Encoding    encoding.Encoding
	decoder     *encoding.Decoder
	encoder     *encoding.Encoder
}

// VRSpecification represents a specification for VR, according to NEMA specs.
type VRSpecification struct {
	VR                 string
	MaximumLengthBytes uint32
	FixedLength        bool
	CharsetRe          *regexp.Regexp
}

/*
===============================================================================
    Error Types
===============================================================================
*/

// UnsupportedDicom is an error representing that the `Dicom` is unsupported
type UnsupportedDicom struct {
	error
}

// NotADicom is an error representing that the input is not recognised as a valid dicom
type NotADicom struct {
	error
}

// CorruptDicom is an error representing that a `Dicom` is corrupt
type CorruptDicom struct {
	error
}

// InsufficientBytes is an error representing that there are not enough bytes left in a buffer
type InsufficientBytes struct {
	error
}

// CorruptElement is an error representing that an `Element` is corrupt
type CorruptElement struct {
	error
}

// CorruptElementStream is an error representing that the `ElementStream` encountered a general problem
type CorruptElementStream struct {
	error
}

// CorruptDicomError raises a `CorruptDicom` error
func CorruptDicomError(format string, a ...interface{}) *CorruptDicom {
	return &CorruptDicom{fmt.Errorf(format, a...)}
}

// CorruptElementError raises a `CorruptElement` error
func CorruptElementError(format string, a ...interface{}) *CorruptElement {
	return &CorruptElement{fmt.Errorf(format, a...)}
}

// CorruptElementStreamError raises a `CorruptElementStream` error
func CorruptElementStreamError(format string, a ...interface{}) *CorruptElementStream {
	return &CorruptElementStream{fmt.Errorf(format, a...)}
}

/*
===============================================================================
    `TransferSyntax`: Support For Multiple Transfer Syntaxes
===============================================================================
*/

func checkTransferSyntaxSupport(tsuid string) bool {
	switch tsuid {
	case "1.2.840.10008.1.2", // Implicit VR Little Endian: Default Transfer Syntax for DICOM
		"1.2.840.10008.1.2.1",    // Explicit VR Little Endian,
		"1.2.840.10008.1.2.2",    // Explicit VR Big Endian (Retired)
		"1.2.840.10008.1.2.4.91", // JPEG 2000 Image Compression
		"1.2.840.10008.1.2.4.70": // Default Transfer Syntax for Lossless JPEG Image Compression
		return true
	default:
		return false
	}
}

// SetFromUID sets the `TransferSyntax` UIDEntry and Encoding from the static dictionary
// https://nathanleclaire.com/blog/2014/08/09/dont-get-bitten-by-pointer-vs-non-pointer-method-receivers-in-golang/
func (ts *TransferSyntax) SetFromUID(uidstr string) error {
	uidptr, err := LookupUID(uidstr)
	if err != nil {
		return err
	}
	ts.UIDEntry = uidptr
	ts.Encoding = GetEncodingForTransferSyntax(*ts)
	return nil
}

func (e Encoding) String() string {
	var implicitness = "ImplicitVR"
	var endian = "LittleEndian"
	if !e.ImplicitVR {
		implicitness = "ExplicitVR"
	}
	if !e.LittleEndian {
		endian = "BigEndian"
	}
	return fmt.Sprintf("%s + %s", implicitness, endian)
}

// transferSyntaxToEncodingMap provides a mapping between transfer syntax UID and encoding
// I couldn't find this mapping in the NEMA documents.
var transferSyntaxToEncodingMap = map[string]*Encoding{
	"1.2.840.10008.1.2":      {ImplicitVR: true, LittleEndian: true},
	"1.2.840.10008.1.2.1":    {ImplicitVR: false, LittleEndian: true},
	"1.2.840.10008.1.2.1.99": {ImplicitVR: false, LittleEndian: true},
	"1.2.840.10008.1.2.2":    {ImplicitVR: false, LittleEndian: false},
}

// GetEncodingForTransferSyntax returns the encoding for a given TransferSyntax, or defaults.
func GetEncodingForTransferSyntax(ts TransferSyntax) *Encoding {
	if ts.UIDEntry != nil {
		encoding, found := transferSyntaxToEncodingMap[ts.UIDEntry.UID]
		if found {
			return encoding
		}
	}
	return transferSyntaxToEncodingMap["1.2.840.10008.1.2.1"] // fallback (default)
}

/*
===============================================================================
    `CharacterSet`: Accurate Text Representation
===============================================================================
*/

// CharacterSetMap provides a mapping between character set name, and character set characteristics.
var CharacterSetMap = map[string]*CharacterSet{
	"Default":         {Name: "Default", Description: "Default Character Repertoire", Encoding: unicode.UTF8},
	"ISO_IR 13":       {Name: "ISO_IR 13", Description: "Japanese", Encoding: japanese.ShiftJIS},
	"ISO_IR 100":      {Name: "ISO_IR 100", Description: "Latin alphabet No. 1", Encoding: charmap.ISO8859_1},
	"ISO_IR 101":      {Name: "ISO_IR 101", Description: "Latin alphabet No. 2", Encoding: charmap.ISO8859_2},
	"ISO_IR 109":      {Name: "ISO_IR 109", Description: "Latin alphabet No. 3", Encoding: charmap.ISO8859_3},
	"ISO_IR 110":      {Name: "ISO_IR 110", Description: "Latin alphabet No. 4", Encoding: charmap.ISO8859_4},
	"ISO_IR 126":      {Name: "ISO_IR 144", Description: "Greek", Encoding: charmap.ISO8859_7},
	"ISO_IR 127":      {Name: "ISO_IR 144", Description: "Arabic", Encoding: charmap.ISO8859_6},
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

func decodeBytes(src []byte, charset *CharacterSet) (string, error) {
	if charset == nil {
		return string(src), nil
	}
	if charset.decoder == nil { // lazy instantiation
		charset.decoder = charset.Encoding.NewDecoder()
	}
	decoded, err := charset.decoder.Bytes(src)
	return string(decoded), err
}

// GetElement returns an Element inside the Dicom according to `tag`.
// If the tag is not found, param `bool` will be false.
func (df Dicom) GetElement(tag uint32) (Element, bool) {
	e, ok := df.Elements[tag]
	return e, ok
}

// GetElement returns an Element inside the Dicom according to `tag`.
// If the tag is not found, param `bool` will be false.
func (i Item) GetElement(tag uint32) (Element, bool) {
	e, ok := i.Elements[tag]
	return e, ok
}

/*
===============================================================================
    `Element`: Value Representation
===============================================================================
*/

// IsCharacterStringVR returns whether the VR is of character string type
func IsCharacterStringVR(vr string) bool {
	switch vr {
	case "AE", "AS", "CS", "DA", "DS", "DT", "IS", "LO", "LT", "PN", "SH", "ST", "TM", "UI", "UT":
		return true
	default:
		return false
	}
}

func splitCharacterStringVM(buffer []byte) [][]byte {
	split := bytes.Split(buffer, []byte(`\`))
	return split
}

func splitBinaryVM(buffer []byte, nBytesEach int) [][]byte {
	out := make([][]byte, 0)
	pos := 0
	for len(buffer) >= pos+nBytesEach {
		out = append(out, buffer[pos:(pos+nBytesEach)])
		pos += nBytesEach
	}
	return out
}

// LookupTag searches for the corresponding `dictionary.DicomDictionary` entry for the given tag uint32
func LookupTag(t uint32) (*dictionary.DictEntry, bool) {
	val, ok := dictionary.DicomDictionary[t]
	if !ok {
		tag := dictionary.Tag(t)
		name := fmt.Sprintf("Unknown%s", tag)
		return &dictionary.DictEntry{Tag: tag, Name: name, NameHuman: name, VR: "UN", VM: "1", Retired: false}, false
	}
	return val, ok
}

// LookupUID searches for the corresponding `dictionary.UIDDictionary` entry for given uid string
func LookupUID(uid string) (*dictionary.UIDEntry, error) {
	val, ok := dictionary.UIDDictionary[uid]
	if !ok {
		return &dictionary.UIDEntry{}, errors.New("could not find UID")
	}
	return val, nil
}

func (a ByTag) Len() int           { return len(a) }
func (a ByTag) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByTag) Less(i, j int) bool { return a[i].Tag < a[j].Tag }

// Describe returns a string array of human-readable element description
func (e Element) Describe() []string {
	var description []string

	if len(e.Items) > 0 {
		description = append(description, fmt.Sprintf("[%s] %s %s:", e.VR, e.Tag, e.Name))
		for _, item := range e.Items {
			for _, e := range item.Elements {
				if e.ValueLength <= 256 {
					description = append(description, fmt.Sprintf("     - %s [%s] %v", e.Tag, e.VR, e.Value()))
				} else {
					description = append(description, fmt.Sprintf("     - %s [%s] (%d bytes)", e.Tag, e.VR, e.ValueLength))
				}
			}

			for _, b := range item.UnknownSections {
				description = append(description, fmt.Sprintf("     - (%d bytes) (not parsed)", len(b)))
			}
		}
	} else {
		if e.ValueLength <= 256 {
			description = append(description, fmt.Sprintf("[%s] %s %s: %v", e.VR, e.Tag, e.Name, e.Value()))
		} else {
			description = append(description, fmt.Sprintf("[%s] %s %s: (%d bytes)", e.VR, e.Tag, e.Name, e.ValueLength))
		}
	}

	// if !e.CheckConformance() {
	//     description[0] = fmt.Sprintf("!! %s", description[0])
	// }
	return description
}

// SupportsMultiVM returns whether the Element can contain multiple values
func (e Element) SupportsMultiVM() bool {
	return e.VM != "" && e.VM != "1"
}

func decodeContents(buffer []byte, e *Element) interface{} {
	switch e.VR { // string
	case "SH", "LO", "ST", "PN", "LT", "UT":
		decoded, err := decodeBytes(buffer, e.sourceElementStream.CharacterSet)
		if err != nil {
			return nil
		}
		return decoded
	case "IS", "DS", "TM", "DA", "DT", "UI", "CS", "AS", "AE":
		return string(buffer)
	case "AT":
		if len(buffer) != 4 || len(buffer)%2 != 0 { // this should never happen, but if it does, return the original bytes
			return buffer
		}
		var lower uint16
		var upper uint16
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			lower = binary.LittleEndian.Uint16(buffer[0:2])
			upper = binary.LittleEndian.Uint16(buffer[2:4])
		} else {
			lower = binary.BigEndian.Uint16(buffer[0:2])
			upper = binary.BigEndian.Uint16(buffer[2:4])
		}
		tagUint32 := (uint32(lower) << 16) | uint32(upper)
		tag := dictionary.Tag(tagUint32)
		return tag.String()
	case "FL": // float
		if len(buffer) < 4 {
			goto InsufficientBytes
		}
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return float32(binary.LittleEndian.Uint32(buffer))
		}
		return float32(binary.BigEndian.Uint32(buffer))
	case "FD": // double
		if len(buffer) < 8 {
			goto InsufficientBytes
		}
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return float64(binary.LittleEndian.Uint64(buffer))
		}
		return float64(binary.BigEndian.Uint64(e.value))
	case "SS": // short
		if len(buffer) < 2 {
			goto InsufficientBytes
		}
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return int16(binary.LittleEndian.Uint16(buffer))
		}
		return int16(binary.BigEndian.Uint16(buffer))
	case "SL": // long
		if len(buffer) < 4 {
			goto InsufficientBytes
		}
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return int32(binary.LittleEndian.Uint32(buffer))
		}
		return int32(binary.BigEndian.Uint32(buffer))
	case "US": // ushort
		if len(buffer) < 2 {
			goto InsufficientBytes
		}
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return binary.LittleEndian.Uint16(buffer)
		}
		return binary.BigEndian.Uint16(buffer)
	case "UL": // ulong
		if len(buffer) < 4 {
			goto InsufficientBytes
		}
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return binary.LittleEndian.Uint32(buffer)
		}
		return binary.BigEndian.Uint32(buffer)
	// TODO: OW and OF require byteswapping too, but am unable to find sample datasets to validate method.
	// Other libraries seem to not byteswap, and instead defer to consumer.
	default:
		return buffer
	}
InsufficientBytes:
	return nil
}

// Value returns an abstraction layer to the underlying bytestream according to VR
func (e Element) Value() interface{} {
	if e.value == nil || e.ValueLength == 0 { // check both to be sure
		if len(e.Items) > 0 {
			return e.Items
		}
		return nil // neither value nor items are set: contents are empty
	}

	/*
	   Psuedocode for parsing VM =
	   1: Check whether element supports multivm
	       1.1: Yes:
	           1.1.1: Switch VR
	             - For each VR, Split into 2D array according to Character string delimiter / binary spacing
	             - For each split, append decoded contents with correct type casting
	           1.1.2: Return 2D array
	       1.2: No:
	           1.2.2: Return decoded contents
	*/
	// TODO: Check whether ValueBytes() is necessary, or whether e.value would suffice
	valueBytes := e.ValueBytes()
	if e.SupportsMultiVM() {
		switch e.VR {
		case "AE", "AS", "CS", "DA", "DS", "DT", "IS", "LO", "PN", "SH", "TM", "UI": // LT, ST, UT do not support multiVM
			var outBuf []string
			for _, v := range splitCharacterStringVM(valueBytes) {
				outBuf = append(outBuf, decodeContents(v, &e).(string))
			}
			return outBuf
		case "FL":
			var outBuf []float32
			for _, v := range splitBinaryVM(valueBytes, 4) {
				outBuf = append(outBuf, decodeContents(v, &e).(float32))
			}
			return outBuf
		case "FD":
			var outBuf []float64
			for _, v := range splitBinaryVM(valueBytes, 8) {
				outBuf = append(outBuf, decodeContents(v, &e).(float64))
			}
			return outBuf
		case "SS":
			var outBuf []int16
			for _, v := range splitBinaryVM(valueBytes, 2) {
				outBuf = append(outBuf, decodeContents(v, &e).(int16))
			}
			return outBuf
		case "SL":
			var outBuf []int32
			for _, v := range splitBinaryVM(valueBytes, 4) {
				outBuf = append(outBuf, decodeContents(v, &e).(int32))
			}
			return outBuf
		case "US":
			var outBuf []uint16
			for _, v := range splitBinaryVM(valueBytes, 2) {
				outBuf = append(outBuf, decodeContents(v, &e).(uint16))
			}
			return outBuf
		case "UL":
			var outBuf []uint32
			for _, v := range splitBinaryVM(valueBytes, 4) {
				outBuf = append(outBuf, decodeContents(v, &e).(uint32))
			}
			return outBuf
		}
	}
	return decodeContents(valueBytes, &e)
}

// ValueBytes returns *all* bytes contained within an element's value, including sequences
func (e Element) ValueBytes() []byte {
	var buffer []byte
	if e.value != nil && e.ValueLength > 0 {
		return e.value
	}
	for _, item := range e.Items {
		if len(item.UnknownSections) > 0 {
			for _, v := range item.UnknownSections {
				buffer = append(buffer, v...)
			}
			continue
		}
		for _, v := range item.Elements {
			//log.Printf("Found element: %s", v.Tag)
			buffer = append(buffer, v.ValueBytes()...)
		}
	}

	return buffer
}

/*
===============================================================================
    `ElementStream`: Element Parser
===============================================================================
*/

// GetElement yields an `Element` from the active stream, and an `error` if something went wrong.
func (elementStream *ElementStream) GetElement() (Element, error) {
	element := Element{}
	element.sourceElementStream = elementStream
	element.sourceElementStream = elementStream

	startBytePos := elementStream.GetPosition()
	element.FileOffsetStart = startBytePos
	lower, err := elementStream.getUint16()
	if err != nil {
		return element, CorruptElementError("GetElement(): %v", err)
	}
	upper, err := elementStream.getUint16()
	if err != nil {
		return element, CorruptElementError("GetElement(): %v", err)
	}
	tagUint32 := (uint32(lower) << 16) | uint32(upper)
	tag, _ := LookupTag(tagUint32)
	element.DictEntry = tag
	if elementStream.TransferSyntax.Encoding.ImplicitVR {
		// implicit VR -- all VR length definitions are 32 bits
		element.ValueLength, err = elementStream.getUint32()
		if err != nil {
			return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
		}
	} else {
		VRbytes, err := elementStream.getBytes(2)
		if err != nil {
			return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
		}
		VRstring := string(VRbytes)
		if element.VR == "UN" { // only use source VR if we dont already have VR from dictionary (more reliable this way)
			element.VR = string(VRbytes)
		}
		// issue #6: use *source* VR as basis for deciding whether to skip / size of length integer.
		// in explicit VR mode, if the VR is OB, OW, SQ, UN or UT, skip two bytes and read as uint32, else uint16.
		if VRstring == "OB" || VRstring == "OW" || VRstring == "SQ" || VRstring == "UN" || VRstring == "UT" {
			err := elementStream.skipBytes(2)
			if err != nil {
				return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
			}
			element.ValueLength, err = elementStream.getUint32()
			if err != nil {
				return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
			}
		} else {
			length, err := elementStream.getUint16()
			if err != nil {
				return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
			}
			element.ValueLength = uint32(length)
		}
	}
	if element.ValueLength == 0xFFFFFFFF {
		var parseElements = (element.VR == "SQ")
		items, err := elementStream.getSequence(parseElements)
		if err != nil {
			return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
		}
		element.Items = items
	} else {
		// issue #4: Parser allows for element value length to exceed file size
		if int64(element.ValueLength) > elementStream.readerSize {
			return element, CorruptElementError("GetElement(): value length (%d) exceeds file size (%d)", element.ValueLength, elementStream.readerSize)
		}
		// string padding: should remove trailing+leading 0x00 / 0x20 bytes (see: http://dicom.nema.org/dicom/2013/output/chtml/part05/sect_6.2.html)
		// NOTE: some vendors pad with 0x20, some 0x00 -- seems to contradict NEMA spec. Let's account for both then:
		if element.ValueLength > 0 {
			valuebuf, err := elementStream.getBytes(uint(element.ValueLength))
			if err != nil {
				switch err.(type) {
				case InsufficientBytes:
					if StrictMode {
						return element, err
					}
					// not running in safe mode, we can truncate the buffer to remaining bytes
					log.Warn().
						Str("tag", element.Tag.String()).
						Uint32("from", element.ValueLength).
						Int64("to", elementStream.GetRemainingBytes()).
						Msg("element value length truncated due to reaching end of the file. use with caution.")
					element.ValueLength = uint32(elementStream.GetRemainingBytes())
					valuebuf, err = elementStream.getBytes(uint(element.ValueLength))
					if err != nil {
						return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
					}
				default:
					return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
				}
			}
			padchars := []byte{0x00, 0x20}
			if element.ValueLength > 1 { // cannot strip padding characters if it would leave the bytestream with length of 0
				switch element.VR {
				case "UI", "OB", "CS", "DS", "IS", "AE", "AS", "DA", "DT", "LO", "LT", "OD", "OF", "OW", "PN", "SH", "ST", "TM", "UT":
					for _, chr := range padchars {
						if valuebuf[len(valuebuf)-1] == chr {
							valuebuf = valuebuf[:len(valuebuf)-1]
							element.ValueLength--
						} else if valuebuf[0] == chr { // NOTE: assumes padding will only take place on one side. Should be fine.
							valuebuf = valuebuf[1:]
							element.ValueLength--
						}
					}
				}
			}
			element.value = valuebuf
		} else {
			element.value = []byte{}
		}
	}

	element.ByteLengthTotal = (elementStream.GetPosition() - startBytePos)
	return element, nil
}

// getSequence parses a sequence of "unlimited length" from the bytestream
func (elementStream *ElementStream) getSequence(parseElements bool) ([]Item, error) {
	var items []Item
	for {
		lower, err := elementStream.getUint16()
		if err != nil {
			return items, CorruptElementStreamError("getSequence(%v): %v", parseElements, err)
		}
		upper, err := elementStream.getUint16()
		if err != nil {
			return items, CorruptElementStreamError("getSequence(%v): %v", parseElements, err)
		}
		tagUint32 := (uint32(lower) << 16) | uint32(upper)
		if tagUint32 == 0xFFFEE0DD {
			err := elementStream.skipBytes(4)
			if err != nil {
				return items, CorruptElementStreamError("getSequence(%v): %v", parseElements, err)
			}
			break
		}
		if tagUint32 != 0xFFFEE000 {
			return items, CorruptElementStreamError("getSequence(%v): 0x%08X != 0xFFFEE000", parseElements, tagUint32)
		}
		length, err := elementStream.getUint32()
		if err != nil {
			return items, CorruptElementStreamError("getSequence(%v): %v", parseElements, err)
		}

		var elements = make(map[uint32]Element)
		var unknownBuffers [][]byte
		if length == 0xFFFFFFFF { // unlimited length item
			// find next FFFE, E00D = data for item ends
			var delimitationItemBytes []byte
			if elementStream.TransferSyntax.Encoding.LittleEndian {
				delimitationItemBytes = []byte{0xFE, 0xFF, 0x0D, 0xE0}
			} else {
				delimitationItemBytes = []byte{0xFF, 0xFE, 0xE0, 0x0D}
			}

			for {
				// try to grab an element according to current TransferSyntax
				e, err := elementStream.GetElement()
				if err != nil {
					return items, CorruptDicomError("getSequence(%v): %v", parseElements, err)
				}
				elements[uint32(e.Tag)] = e
				check, err := elementStream.reader.Peek(4)
				if err != nil {
					return items, CorruptElementStreamError("getSequence(%v): %v", parseElements, err)
				}
				if bytes.Equal(check, delimitationItemBytes) {
					// end
					break
				}
			}

			// now we must skip eight bytes (delimitation item + 0x00{4}) (see: NEMA Table 7.5-3)
			err = elementStream.skipBytes(8)
			if err != nil {
				return items, CorruptElementStreamError("getSequence(%v): %v", parseElements, err)
			}
		} else {
			// try to grab an element according to current TransferSyntax
			if parseElements {
				if length == 0 {
					continue
					/* Turns out the data set had bytes:
					   (40 00 08 00) (53 51)  00 00 (FF FF  FF FF) (FE FF  00 E0) (00 00  00 00) (FE FF  DD E0) 00 00
					   (4b: tag)     (2b:SQ)        (4b: un.len)   (4b:itm start) (4b: 0 len)    (4b: seq end)
					   Therefore, the item genuinely had length of zero.
					   This condition accounts for this possibility.
					*/
				}
				element, err := elementStream.GetElement()
				if err != nil {
					return items, CorruptDicomError("getSequence(%v): %v", parseElements, err)
				}
				elements[uint32(element.Tag)] = element

			} else {
				valuebuffer, err := elementStream.getBytes(uint(length))
				if err != nil {
					return items, CorruptElementStreamError("getSequence(%v): %v", parseElements, err)
				}
				unknownBuffers = append(unknownBuffers, valuebuffer)
			}
		}
		item := Item{Elements: elements, UnknownSections: unknownBuffers}
		items = append(items, item)
	}

	return items, nil
}

func (elementStream *ElementStream) skipBytes(num int) error {
	if num == 0 {
		return nil
	}
	if numRemaining := elementStream.GetRemainingBytes(); numRemaining < int64(num) {
		return CorruptElementStreamError("skipBytes(%d): would exceed buffer size (%d bytes)", num, numRemaining)
	}
	nseek, err := elementStream.reader.Discard(num)
	if err != nil {
		return CorruptElementStreamError("skipBytes(%d): %v", num, err)
	}
	elementStream.readerPos += int64(nseek)
	if nseek < num {
		return CorruptElementStreamError("skipBytes(%d): nseek = %d", num, nseek)
	}
	return nil
}

// GetPosition returns the current buffer position
func (elementStream *ElementStream) GetPosition() (pos int64) {
	pos = elementStream.readerPos
	return
}

// GetRemainingBytes returns the number of remaining unread bytes
func (elementStream *ElementStream) GetRemainingBytes() (num int64) {
	num = elementStream.readerSize - elementStream.readerPos
	return
}

func (elementStream *ElementStream) getUint16() (uint16, error) {
	if numRemaining := elementStream.GetRemainingBytes(); numRemaining < 2 {
		return 0, CorruptElementStreamError("getUint16(): would exceed buffer size (%d bytes)", numRemaining)
	}
	buf := make([]byte, 2)
	nread, err := io.ReadFull(elementStream.reader, buf)
	if err != nil {
		return 0, CorruptElementStreamError("getUint16(): %v", err)
	}
	elementStream.readerPos += int64(nread)
	if nread != 2 {
		return 0, CorruptElementStreamError("getUint16(): nread = %d (!= 2)", nread)
	}
	if elementStream.TransferSyntax.Encoding.LittleEndian {
		return binary.LittleEndian.Uint16(buf), nil
	}
	return binary.BigEndian.Uint16(buf), nil
}

func (elementStream *ElementStream) getUint32() (uint32, error) {
	if numRemaining := elementStream.GetRemainingBytes(); numRemaining < 4 {
		return 0, CorruptElementStreamError("getUint32(): would exceed buffer size (%d bytes)", numRemaining)
	}
	buf := make([]byte, 4)
	nread, err := io.ReadFull(elementStream.reader, buf)
	if err != nil {
		return 0, CorruptElementStreamError("getUint32(): %v", err)
	}
	elementStream.readerPos += int64(nread)
	if nread != 4 {
		return 0, CorruptElementStreamError("getUint32(): nread = %d (!= 4)", nread)
	}

	if elementStream.TransferSyntax.Encoding.LittleEndian {
		return binary.LittleEndian.Uint32(buf), nil
	}
	return binary.BigEndian.Uint32(buf), nil
}

func (elementStream *ElementStream) getBytes(num uint) ([]byte, error) {
	if num == 0 {
		return []byte{}, nil
	}
	if num > uint(elementStream.GetRemainingBytes()) {
		return nil, InsufficientBytes{fmt.Errorf("getBytes(%d): (offset 0x%X): would exceed buffer size (%d bytes)", num, elementStream.GetPosition(), elementStream.GetRemainingBytes())}
	}
	buf := make([]byte, num)
	nread, err := io.ReadFull(elementStream.reader, buf)
	if err != nil {
		return buf, CorruptElementStreamError("getBytes(%d): %v", num, err)
	}
	elementStream.readerPos += int64(nread)
	if uint(nread) != num {
		return buf, CorruptElementStreamError("getBytes(%d): nread = %d (!= %d)", num, nread, num)
	}
	return buf, nil
}

// NewElementStream sets up a new `ElementStream`
func NewElementStream(readerPtr *bufio.Reader, readerSize int64) (stream ElementStream) {
	stream = ElementStream{TransferSyntax: TransferSyntax{}}
	stream.CharacterSet = CharacterSetMap["Default"]
	stream.reader = readerPtr
	stream.readerSize = readerSize
	stream.SetTransferSyntax("1.2.840.10008.1.2.1")
	return
}

// SetTransferSyntax sets the `ElementStream`s TransferSyntax according to uid string
func (elementStream *ElementStream) SetTransferSyntax(transferSyntaxUID string) {
	elementStream.TransferSyntax.SetFromUID(transferSyntaxUID)
	elementStream.TransferSyntax.Encoding = GetEncodingForTransferSyntax(elementStream.TransferSyntax)
}

/*
===============================================================================
    `Dicom`: DICOM Parser
===============================================================================
*/

func (df *Dicom) crawlMeta() error {
	preamble, err := df.elementStream.getBytes(128)
	if err != nil {
		return CorruptDicomError("crawlMeta(): %v", err)
	}
	copy(df.Preamble[:], preamble)
	dicmTestString, err := df.elementStream.getBytes(4)
	if err != nil {
		return CorruptDicomError("crawlMeta(): %v", err)
	}
	if string(dicmTestString) != "DICM" {
		return &NotADicom{}
	}

	metaLengthElement, err := df.elementStream.GetElement()
	if err != nil {
		return CorruptDicomError("crawlMeta: %v", err)
	}
	df.Elements[uint32(metaLengthElement.Tag)] = metaLengthElement

	if val, ok := metaLengthElement.Value().(uint32); ok {
		df.TotalMetaBytes = df.elementStream.GetPosition() + int64(val)
	} else {
		return CorruptDicomError("meta length element is corrupt")
	}

	for {
		element, err := df.elementStream.GetElement()

		if err != nil {
			return CorruptDicomError("crawlMeta: %v", err)
		}
		df.Elements[uint32(element.Tag)] = element

		if df.elementStream.GetPosition() >= df.TotalMetaBytes {
			break
		}
	}

	return nil
}

func (df *Dicom) crawlElements() error {
	transfersyntaxuid := "1.2.840.10008.1.2.1"
	// change transfer syntax if necessary
	tsElement, found := df.GetElement(0x0020010)
	if found {
		if transfersyntaxuid, ok := tsElement.Value().(string); ok {
			supported := checkTransferSyntaxSupport(transfersyntaxuid)
			if !supported {
				return &UnsupportedDicom{fmt.Errorf("unsupported transfer syntax: %s", transfersyntaxuid)}
			}
		} else {
			return CorruptDicomError("TransferSyntaxUID is corrupt")
		}
	}
	df.elementStream.SetTransferSyntax(transfersyntaxuid)

	for {
		if df.elementStream.GetPosition() >= df.elementStream.readerSize {
			break
		}
		element, err := df.elementStream.GetElement()
		if err != nil {
			return CorruptDicomError("crawlElements(): %v", err)
		}
		df.Elements[uint32(element.Tag)] = element

		switch element.Tag {
		case 0x00080005:
			if val, ok := element.Value().([]string); ok {
				if len(val) > 0 {
					df.elementStream.CharacterSet = CharacterSetMap[val[0]]
				}
			} // TODO: Should bad CharacterSet result in CorruptDicom, or instead use UTF8?
		}
	}
	return nil
}

// ParseDicom takes a relative/absolute path to a dicom file and returns a parsed `Dicom` [+ error]
func ParseDicom(path string) (Dicom, error) {
	dcm := Dicom{}
	dcm.FilePath = path
	dcm.Elements = make(map[uint32]Element)

	f, err := os.Open(path)
	if err != nil {
		return dcm, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return dcm, err
	}
	fileSize := stat.Size()

	if fileSize < 256*1024 {
		dcm.reader = Get256k(f)
		defer func() {
			Put256k(dcm.reader)
		}()
	} else if fileSize < 512*1024 {
		dcm.reader = Get512k(f)
		defer func() {
			Put512k(dcm.reader)
		}()
	} else {
		dcm.reader = Get2mb(f)
		defer func() {
			Put2mb(dcm.reader)
		}()
	}
	dcm.elementStream = NewElementStream(dcm.reader, fileSize)
	if err := dcm.crawlMeta(); err != nil {
		switch err.(type) {
		case *NotADicom:
			return dcm, &NotADicom{fmt.Errorf(`The file "%s" is not a valid dicom`, filepath.Base(path))}
		default:
			return dcm, CorruptDicomError(`The file "%s" is corrupt: %v`, filepath.Base(path), err)
		}
	}
	if err := dcm.crawlElements(); err != nil {
		return dcm, CorruptDicomError(`The dicom "%s" is corrupt: %v`, filepath.Base(path), err)
	}

	return dcm, nil
}

// ParseFromBytes parses a dicom from a bytestream
func ParseFromBytes(source []byte) (Dicom, error) {
	dcm := Dicom{}
	if len(source) < 256*1024 {
		dcm.reader = Get256k(bytes.NewReader(source))
		defer func() {
			Put256k(dcm.reader)
		}()
	} else if len(source) < 512*1024 {
		dcm.reader = Get512k(bytes.NewReader(source))
		defer func() {
			Put512k(dcm.reader)
		}()
	} else {
		dcm.reader = Get2mb(bytes.NewReader(source))
		defer func() {
			Put2mb(dcm.reader)
		}()
	}
	dcm.elementStream = NewElementStream(dcm.reader, int64(len(source)))
	dcm.Elements = make(map[uint32]Element)

	if err := dcm.crawlMeta(); err != nil {
		switch err.(type) {
		case *NotADicom:
			return dcm, &NotADicom{fmt.Errorf(`The bytes do not form a valid dicom`)}
		default:
			return dcm, CorruptDicomError(`The bytes are corrupt: %v`, err)
		}
	}
	if err := dcm.crawlElements(); err != nil {
		return dcm, CorruptDicomError(`The bytes are corrupt: %v`, err)
	}
	return dcm, nil
}

// ParseDicomChannel wraps `ParseDicom` in a channel for parsing in a goroutine
func ParseDicomChannel(path string, dicomchannel chan Dicom, errorchannel chan error) {
	dcm, err := ParseDicom(path)

	if err != nil {
		errorchannel <- err
	}
	dicomchannel <- dcm
}
