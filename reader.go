package opendcm

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/b71729/opendcm/dictionary"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

/*
===============================================================================
    Data Types
===============================================================================
*/

// readerPool wraps a `sync.Pool` to allow for custom Get/Put methods
type readerPool struct {
	pool *sync.Pool
}

// Nalloc is used in `opendcm-util` for showing number of reader allocations
var Nalloc = 0

// ReaderPool is a pool of `bufio.Reader` with a buffer size set to `Config`
var ReaderPool = readerPool{pool: &sync.Pool{
	New: func() interface{} {
		Nalloc++
		return bufio.NewReaderSize(nil, GetConfig().DicomReadBufferSize)
	},
}}

// Get selects an arbitrary item from the Pool, removes it from the
// Pool, and returns it to the caller.
func (rp *readerPool) Get(src io.Reader) (r *bufio.Reader) {
	r = rp.pool.Get().(*bufio.Reader)
	r.Reset(src)
	return
}

// Put adds `r` to the pool.
func (rp *readerPool) Put(r *bufio.Reader) {
	rp.pool.Put(r)
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
	Elements map[uint32]Element
	Unparsed []byte
}

// ElementStream provides an abstraction layer around a `*bytes.Reader` to facilitate easier parsing.
type ElementStream struct {
	reader         *bufio.Reader
	readerPos      int64
	readerSize     int64
	TransferSyntax TransferSyntax
	CharacterSet   *CharacterSet
	buffers
}

// buffers holds variables for temporary use
// this is to ease pressure off the GC
type buffers struct {
	ui16b [2]byte
	ui32b [4]byte
	nread int
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

// RecognisedVRs lists all recognised VRs.
// See ``6.2 Value Representation (VR)`` for more information
var RecognisedVRs = []string{
	"AE", "AS", "AT", "CS", "DA", "DS", "DT", "FL", "FD", "IS", "LO", "LT", "OB", "OD",
	"OF", "OW", "PN", "SH", "SL", "SQ", "SS", "ST", "TM", "UI", "UL", "UN", "US", "UT",
}

/*
===============================================================================
    Error Types
===============================================================================
*/

// UnsupportedDicom is an error indicating that the `Dicom` is unsupported
type UnsupportedDicom struct {
	error
}

// NotADicom is an error indicating that the input is not recognised as a valid dicom
type NotADicom struct {
	error
}

// CorruptDicom is an error indicating that a `Dicom` is corrupt
type CorruptDicom struct {
	error
}

// InsufficientBytes is an error indicating that there are not enough bytes left in a buffer
type InsufficientBytes struct {
	error
}

// CorruptElement is an error indicating that an `Element` is corrupt
type CorruptElement struct {
	error
}

// CorruptElementStream is an error indicating that the `ElementStream` encountered a general problem
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

// UnsupportedDicomError raises a `UnsupportedDicom` error
func UnsupportedDicomError(format string, a ...interface{}) *UnsupportedDicom {
	return &UnsupportedDicom{fmt.Errorf(format, a...)}
}

/*
===============================================================================
    `TransferSyntax`: Support For Multiple Transfer Syntaxes
===============================================================================
*/

// checkTransferSyntaxSupport checks whether the given Transfer Syntax UID `tsuid`
// is listed as supported by OpenDCM.
func checkTransferSyntaxSupport(tsuid string) (supported bool) {
	switch tsuid {
	case "1.2.840.10008.1.2", // Implicit VR Little Endian: Default Transfer Syntax for DICOM
		"1.2.840.10008.1.2.1",    // Explicit VR Little Endian,
		"1.2.840.10008.1.2.2",    // Explicit VR Big Endian (Retired)
		"1.2.840.10008.1.2.4.91", // JPEG 2000 Image Compression,
		"1.2.840.10008.1.2.4.90", // JPEG 2000 Image Compression (Lossless Only)
		"1.2.840.10008.1.2.4.70": // Default Transfer Syntax for Lossless JPEG Image Compression
		supported = true
	}
	return
}

// SetFromUID sets the `TransferSyntax` UIDEntry and Encoding from the static dictionary
// https://nathanleclaire.com/blog/2014/08/09/dont-get-bitten-by-pointer-vs-non-pointer-method-receivers-in-golang/
func (ts *TransferSyntax) SetFromUID(uidstr string) error {
	uidptr, found := LookupUID(uidstr)
	if !found {
		return errors.New("could not find UID in records")
	}
	ts.UIDEntry = uidptr
	ts.Encoding = GetEncodingForTransferSyntax(*ts)
	//Debugf("switched transfer syntax to %s", uidstr)
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
		if encoding, found := transferSyntaxToEncodingMap[ts.UIDEntry.UID]; found {
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

// decodeBytes attempts to decode `src` using `charset.decoder` (i.e. UTF-8 or ShiftJIS).
// If there arises an issue decoding `src`, `error` will be non-nil.
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

/*
===============================================================================
    `Element`: Value Representation
===============================================================================
*/

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

// IsCharacterStringVR returns whether the VR is of character string type
func IsCharacterStringVR(vr string) bool {
	switch vr {
	case "AE", "AS", "CS", "DA", "DS", "DT", "IS", "LO", "LT", "PN", "SH", "ST", "TM", "UI", "UT":
		return true
	default:
		return false
	}
}

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

// LookupTag searches for the corresponding `dictionary.DicomDictionary` entry for the given tag uint32
func LookupTag(t uint32) (entry *dictionary.DictEntry, found bool) {
	entry, found = dictionary.DicomDictionary[t]
	if !found {
		tag := dictionary.Tag(t)
		name := "Unknown" + tag.String()
		entry = &dictionary.DictEntry{Tag: tag, Name: name, NameHuman: name, VR: "UN", VM: "1", Retired: false}
	}
	return
}

// LookupUID searches for the corresponding `dictionary.UIDDictionary` entry for given uid string
func LookupUID(uid string) (entry *dictionary.UIDEntry, found bool) {
	entry, found = dictionary.UIDDictionary[uid]
	if !found {
		entry = &dictionary.UIDEntry{}
	}
	return
}

func (a ByTag) Len() int           { return len(a) }
func (a ByTag) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByTag) Less(i, j int) bool { return a[i].Tag < a[j].Tag }

// Describe returns a string array of human-readable element description
func (e Element) Describe(indentLevel int) []string {
	var description []string
	indentStr := strings.Repeat(" ", indentLevel)
	if len(e.Items) > 0 {
		description = append(description, fmt.Sprintf("%s[%s] %s %s:", indentStr, e.VR, e.Tag, e.Name))
		for _, item := range e.Items {
			if len(item.Unparsed) > 0 { // the element contains an unparsed buffer.
				description = append(description, fmt.Sprintf("%s    (%d bytes)", indentStr, len(item.Unparsed)))
			} else {
				for _, e := range item.Elements {
					description = append(description, e.Describe(indentLevel+4)...)
				}
			}
		}
	} else if e.ValueLength > 0 {
		if e.ValueLength <= 256 {
			description = append(description, fmt.Sprintf("%s[%s] %s %s: %v", indentStr, e.VR, e.Tag, e.Name, e.Value()))
		} else {
			description = append(description, fmt.Sprintf("%s[%s] %s %s: (%d bytes)", indentStr, e.VR, e.Tag, e.Name, e.ValueLength))
		}
	} else { // no value, nor items; return empty
		return append(description, fmt.Sprintf("%s[%s] %s %s: (empty)", indentStr, e.VR, e.Tag, e.Name))
	}
	return description
}

// SupportsMultiVM returns whether the Element can contain multiple values
func (e Element) SupportsMultiVM() bool {
	return e.VM != "" && e.VM != "1" && e.VM != "1-1" && e.VM != "0"
}

func decodeContents(buffer []byte, e *Element) interface{} {
	switch e.VR { // string
	case "SH", "LO", "ST", "PN", "LT", "UT":
		decoded, err := decodeBytes(buffer, e.sourceElementStream.CharacterSet)
		if err != nil {
			Warnf("error decoding %s with CharacterSet %s: %v", e.Tag, e.sourceElementStream.CharacterSet.Name)
			return nil
		}
		return decoded
	case "IS", "DS", "TM", "DA", "DT", "UI", "CS", "AS", "AE":
		return string(buffer)
	case "AT":
		if len(buffer) != 4 || len(buffer)%2 != 0 { // this should never happen, but if it does, return the original bytes
			Warnf(`found "AT" element bad length (%d bytes). use with caution.`, len(buffer))
			return buffer
		}
		tagUint32 := tagFromBytes(buffer[:4], e.sourceElementStream.TransferSyntax.Encoding.LittleEndian)
		tag := dictionary.Tag(tagUint32)
		return tag.String()
	case "FL": // float
		if len(buffer) < 4 {
			return nil
		}
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return math.Float32frombits(binary.LittleEndian.Uint32(buffer))
		}
		return math.Float32frombits(binary.BigEndian.Uint32(buffer))
	case "FD": // double
		if len(buffer) < 8 {
			return nil
		}
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return math.Float64frombits(binary.LittleEndian.Uint64(buffer))
		}
		return math.Float64frombits(binary.BigEndian.Uint64(e.value))
	case "SS": // short
		if len(buffer) < 2 {
			return nil
		}
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return int16(binary.LittleEndian.Uint16(buffer))
		}
		return int16(binary.BigEndian.Uint16(buffer))
	case "SL": // long
		if len(buffer) < 4 {
			return nil
		}
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return int32(binary.LittleEndian.Uint32(buffer))
		}
		return int32(binary.BigEndian.Uint32(buffer))
	case "US": // ushort
		if len(buffer) < 2 {
			return nil
		}
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return binary.LittleEndian.Uint16(buffer)
		}
		return binary.BigEndian.Uint16(buffer)
	case "UL": // ulong
		if len(buffer) < 4 {
			return nil
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
}

// Value returns an abstraction layer to the underlying bytestream according to VR
func (e Element) Value() interface{} {
	if e.ValueLength == 0xFFFFFFFF || len(e.Items) > 0 { // undefined length: will contain items
		return e.Items
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
	if e.SupportsMultiVM() {
		switch e.VR {
		case "AE", "AS", "CS", "DA", "DS", "DT", "IS", "LO", "PN", "SH", "TM", "UI": // LT, ST, UT do not support multiVM
			var outBuf []string
			for _, v := range splitCharacterStringVM(e.value) {
				outBuf = append(outBuf, decodeContents(v, &e).(string))
			}
			return outBuf
		case "FL":
			var outBuf []float32
			for _, v := range splitBinaryVM(e.value, 4) {
				outBuf = append(outBuf, decodeContents(v, &e).(float32))
			}
			return outBuf
		case "FD":
			var outBuf []float64
			for _, v := range splitBinaryVM(e.value, 8) {
				outBuf = append(outBuf, decodeContents(v, &e).(float64))
			}
			return outBuf
		case "SS":
			var outBuf []int16
			for _, v := range splitBinaryVM(e.value, 2) {
				outBuf = append(outBuf, decodeContents(v, &e).(int16))
			}
			return outBuf
		case "SL":
			var outBuf []int32
			for _, v := range splitBinaryVM(e.value, 4) {
				outBuf = append(outBuf, decodeContents(v, &e).(int32))
			}
			return outBuf
		case "US":
			var outBuf []uint16
			for _, v := range splitBinaryVM(e.value, 2) {
				outBuf = append(outBuf, decodeContents(v, &e).(uint16))
			}
			return outBuf
		case "UL":
			var outBuf []uint32
			for _, v := range splitBinaryVM(e.value, 4) {
				outBuf = append(outBuf, decodeContents(v, &e).(uint32))
			}
			return outBuf
		}
	}
	return decodeContents(e.value, &e)
}

/*
===============================================================================
    `ElementStream`: Element Parser
===============================================================================
*/

// GetElement retrives an `Element` from the reader, and an `error` if something went wrong.
func (es *ElementStream) GetElement() (Element, error) {
	element := Element{}
	element.sourceElementStream = es

	startBytePos := es.GetPosition()
	element.FileOffsetStart = startBytePos
	tagUint32, err := es.getTag()
	if err != nil {
		return element, CorruptElementError("GetElement(): %v", err)
	}
	tag, _ := LookupTag(tagUint32)
	element.DictEntry = tag
	if es.TransferSyntax.Encoding.ImplicitVR {
		// implicit VR -- all VR length definitions are 32 bits
		element.ValueLength, err = es.getUint32()
		if err != nil {
			return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
		}
	} else {
		VRbytes, err := es.getBytes(2)
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
			err := es.skipBytes(2)
			if err != nil {
				return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
			}
			element.ValueLength, err = es.getUint32()
			if err != nil {
				return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
			}
		} else {
			length, err := es.getUint16()
			if err != nil {
				return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
			}
			element.ValueLength = uint32(length)
		}
	}
	if element.ValueLength == 0xFFFFFFFF {
		items, err := es.getUndefinedLength(element.VR == "SQ")
		if err != nil {
			return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
		}
		element.Items = items
	} else {
		// issue #4: Parser allows for element value length to exceed file size
		if int64(element.ValueLength) > es.readerSize {
			return element, CorruptElementError("GetElement(): value length (%d) exceeds file size (%d)", element.ValueLength, es.readerSize)
		}
		// string padding: should remove trailing+leading 0x00 / 0x20 bytes (see: http://dicom.nema.org/dicom/2013/output/chtml/part05/sect_6.2.html)
		// NOTE: some vendors pad with 0x20, some 0x00 -- seems to contradict NEMA spec. Let's account for both then:
		if element.ValueLength > 0 {
			valuebuf, err := es.getBytes(uint(element.ValueLength))
			if err != nil {
				switch err.(type) {
				case InsufficientBytes:
					if GetConfig().StrictMode {
						return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
					}
					// not running in safe mode, we can truncate the buffer to remaining bytes
					Warnf("element %s: value length truncated from %d bytes to %d bytes due to reaching end of the file. use with caution.",
						element.Tag, element.ValueLength, es.GetRemainingBytes())
					element.ValueLength = uint32(es.GetRemainingBytes())
					valuebuf, err = es.getBytes(uint(element.ValueLength))
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

	element.ByteLengthTotal = (es.GetPosition() - startBytePos)
	return element, nil
}

// getUndefinedLength retrieves embedded `Item`s in an element of "undefined length" from the reader
func (es *ElementStream) getUndefinedLength(parseElements bool) ([]Item, error) {
	var items []Item
	for {
		tagUint32, err := es.getTag()
		if err != nil {
			return items, CorruptElementStreamError("getUndefinedLength(): %v", err)
		}
		if tagUint32 == 0xFFFEE0DD {
			err := es.skipBytes(4)
			if err != nil {
				return items, CorruptElementStreamError("getUndefinedLength(): %v", err)
			}
			break
		}
		if tagUint32 != 0xFFFEE000 {
			return items, CorruptElementStreamError("getUndefinedLength(): 0x%08X != 0xFFFEE000 (%d)", tagUint32, es.GetPosition())
		}

		// try to retrieve element length from the reader
		length, err := es.getUint32()
		if err != nil {
			return items, CorruptElementStreamError("getUndefinedLength(): %v", err)
		}

		var elements = make(map[uint32]Element)
		var unparsed = make([]byte, 0)
		if length == 0xFFFFFFFF { // undefined length item
			// find next FFFE, E00D = data for item ends
			var delimitationItemBytes [4]byte
			if es.TransferSyntax.Encoding.LittleEndian {
				delimitationItemBytes = [4]byte{0xFE, 0xFF, 0x0D, 0xE0}
			} else {
				delimitationItemBytes = [4]byte{0xFF, 0xFE, 0xE0, 0x0D}
			}

			for {
				// try to decode an element from the bytestream
				e, err := es.GetElement()
				if err != nil {
					switch err.(type) {
					case *CorruptElement:
						return nil, CorruptElementStreamError("getUndefinedLength(): %v", err)
					case *UnsupportedDicom:
						return nil, UnsupportedDicomError("getUndefinedLength(): %v", err)
					default:
						panic(err)
					}
				}
				elements[uint32(e.Tag)] = e

				// peek four bytes to see whether we have reached the delimitation item
				if check, err := es.reader.Peek(4); err != nil {
					return items, CorruptElementStreamError("getUndefinedLength(): %v", err)
				} else if bytes.Equal(check, delimitationItemBytes[:]) {
					// end
					break
				}
			}

			// now we must skip eight bytes (delimitation item + 0x00{4}) (see: NEMA Table 7.5-3)
			if err = es.skipBytes(8); err != nil {
				return items, CorruptElementStreamError("getUndefinedLength(): %v", err)
			}
		} else {
			// try to grab an element from the bytestream
			if length == 0 {
				continue
				/* Turns out the data set had bytes:
				   (40 00 08 00) (53 51)  00 00 (FF FF  FF FF) (FE FF  00 E0) (00 00  00 00) (FE FF  DD E0) 00 00
				   (4b: tag)     (2b:SQ)        (4b: un.len)   (4b:itm start) (4b: 0 len)    (4b: seq end)
				   Therefore, the item genuinely had length of zero.
				   This condition accounts for this possibility.
				*/
			}
			if parseElements {
				element, err := es.GetElement()
				if err != nil {
					switch err.(type) {
					case *CorruptElement:
						return nil, CorruptElementStreamError("getUndefinedLength(): %v", err)
					case *UnsupportedDicom:
						return nil, UnsupportedDicomError("getUndefinedLength(): %v", err)
					default:
						panic(err)
					}
				}
				elements[uint32(element.Tag)] = element
			} else {
				unparsed, err = es.getBytes(uint(length))
				if err != nil {
					return items, CorruptElementStreamError("getUndefinedLength(): %v", err)
				}
			}
		}
		item := Item{Elements: elements, Unparsed: unparsed}
		items = append(items, item)
	}
	return items, nil
}

// skipBytes fast-forwards the reader `num` bytes
func (es *ElementStream) skipBytes(num int) (err error) {
	if num == 0 {
		return nil
	}
	if numRemaining := es.GetRemainingBytes(); numRemaining < int64(num) {
		return CorruptElementStreamError("skipBytes(%d): would exceed buffer size (%d bytes)", num, numRemaining)
	}
	es.nread, err = es.reader.Discard(num)
	if err != nil {
		return CorruptElementStreamError("skipBytes(%d): %v", num, err)
	}
	es.readerPos += int64(es.nread)
	if es.nread < num {
		return CorruptElementStreamError("skipBytes(%d): nseek = %d", num, es.nread)
	}
	return nil
}

// GetPosition returns the current buffer position
func (es *ElementStream) GetPosition() int64 {
	return es.readerPos
}

// GetRemainingBytes returns the number of remaining unread bytes
func (es *ElementStream) GetRemainingBytes() int64 {
	return es.readerSize - es.readerPos
}

// getUint16 retrieves a uint16 (two bytes) from the reader
func (es *ElementStream) getUint16() (res uint16, err error) {
	if numRemaining := es.GetRemainingBytes(); numRemaining < 2 {
		return 0, CorruptElementStreamError("getUint16(): would exceed buffer size (%d bytes)", numRemaining)
	}
	es.nread, err = io.ReadFull(es.reader, es.ui16b[:])
	if err != nil {
		return 0, CorruptElementStreamError("getUint16(): %v", err)
	}
	es.readerPos += int64(es.nread)
	if es.nread != 2 {
		return 0, CorruptElementStreamError("getUint16(): nread = %d (!= 2)", es.nread)
	}
	if es.TransferSyntax.Encoding.LittleEndian {
		res = binary.LittleEndian.Uint16(es.ui16b[:])
	} else {
		res = binary.BigEndian.Uint16(es.ui16b[:])
	}
	return
}

// getUint32 retrieves a uint32 (four bytes) from the reader
func (es *ElementStream) getUint32() (res uint32, err error) {
	if numRemaining := es.GetRemainingBytes(); numRemaining < 4 {
		return 0, CorruptElementStreamError("getUint32(): would exceed buffer size (%d bytes)", numRemaining)
	}
	es.nread, err = io.ReadFull(es.reader, es.ui32b[:])
	if err != nil {
		return 0, CorruptElementStreamError("getUint32(): %v", err)
	}
	es.readerPos += int64(es.nread)
	if es.nread != 4 {
		return 0, CorruptElementStreamError("getUint32(): nread = %d (!= 4)", es.nread)
	}
	if es.TransferSyntax.Encoding.LittleEndian {
		res = binary.LittleEndian.Uint32(es.ui32b[:])
	} else {
		res = binary.BigEndian.Uint32(es.ui32b[:])
	}
	return
}

// tagFromBytes returns a tag uint32 from an array of bytes. length should be at least four.
func tagFromBytes(buf []byte, littleEndian bool) uint32 {
	if littleEndian {
		return (uint32(binary.LittleEndian.Uint16(buf[0:2])) << 16) | uint32(binary.LittleEndian.Uint16(buf[2:4]))
	}
	return (uint32(binary.BigEndian.Uint16(buf[0:2])) << 16) | uint32(binary.BigEndian.Uint16(buf[2:4]))
}

// getTag retrieves a tag uint32 from the reader
func (es *ElementStream) getTag() (tag uint32, err error) {
	if numRemaining := es.GetRemainingBytes(); numRemaining < 4 {
		return 0, CorruptElementStreamError("getUint32(): would exceed buffer size (%d bytes)", numRemaining)
	}
	es.nread, err = io.ReadFull(es.reader, es.ui32b[:])
	if err != nil {
		return 0, CorruptElementStreamError("getUint32(): %v", err)
	}
	es.readerPos += int64(es.nread)
	if es.nread != 4 {
		return 0, CorruptElementStreamError("getUint32(): nread = %d (!= 4)", es.nread)
	}
	tag = tagFromBytes(es.ui32b[:], es.TransferSyntax.Encoding.LittleEndian)
	return
}

// getBytes retrieves `num` bytes from the reader
func (es *ElementStream) getBytes(num uint) ([]byte, error) {
	if num == 0 {
		return []byte{}, nil
	}
	if num > uint(es.GetRemainingBytes()) {
		return nil, InsufficientBytes{fmt.Errorf("getBytes(%d): (offset 0x%X): would exceed buffer size (%d bytes)", num, es.GetPosition(), es.GetRemainingBytes())}
	}
	buf := make([]byte, num)
	nread, err := io.ReadFull(es.reader, buf)
	if err != nil {
		return buf, CorruptElementStreamError("getBytes(%d): %v", num, err)
	}
	es.readerPos += int64(nread)
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
func (es *ElementStream) SetTransferSyntax(transferSyntaxUID string) {
	es.TransferSyntax.SetFromUID(transferSyntaxUID)
	es.TransferSyntax.Encoding = GetEncodingForTransferSyntax(es.TransferSyntax)
}

/*
===============================================================================
    `Dicom`: DICOM Parser
===============================================================================
*/

// getPreamble retrieves the 128 byte preamble at the start of each dicom file
// if the preamble does not exist, or the magic string immediately following preamble
// is not "DICM", `found` will be false.
func (df *Dicom) getPreamble() (preamble []byte, found bool) {
	preamble, err := df.elementStream.reader.Peek(132)
	if err != nil {
		return preamble, false
	}
	if string(preamble[128:]) != "DICM" {
		return preamble[:128], false
	}
	err = df.elementStream.skipBytes(132)
	if err != nil {
		panic(err) // should never happen, since we have already called `Peek(132)`
	}
	return preamble[:128], true
}

// crawlMeta attempts to retrieve all "meta" elements from the reader.
// See ``7.1 DICOM File Meta Information`` for more information.
func (df *Dicom) crawlMeta() error {
	preamble, preambleFound := df.getPreamble()
	if preambleFound {
		copy(df.Preamble[:], preamble)
	} else {
		Debug("file is missing preamble (bytes 0-128)")
	}

	for {
		// check whether meta section has finished
		nextUpperBytes, err := df.elementStream.reader.Peek(2)
		if err != nil {
			break
		}
		nextUpper := binary.LittleEndian.Uint16(nextUpperBytes)
		if nextUpper != 0x0002 {
			Debugf("exiting meta (nextUpper = %04X, offset = 0x%X)", nextUpper, df.elementStream.GetPosition())
			df.TotalMetaBytes = df.elementStream.GetPosition()
			break
		}

		// get next element
		element, err := df.elementStream.GetElement()
		if err != nil {
			switch err.(type) {
			case *CorruptElement, *CorruptElementStream:
				return CorruptDicomError("crawlMeta(): %v", err)
			case *UnsupportedDicom:
				return UnsupportedDicomError("crawlMeta(): %v", err)
			default:
				panic(err)
			}
		}
		df.Elements[uint32(element.Tag)] = element
	}
	return nil
}

func guessTransferSyntaxFromBytes(buf []byte) (encoding Encoding, success bool) {
	firstTwoLE := binary.LittleEndian.Uint16(buf[0:2])
	encoding.LittleEndian = true
	if firstTwoLE > 2000 && firstTwoLE != 0x7FE0 {
		// likely big endian
		encoding.LittleEndian = false
	}
	vrfrombytes := string(buf[4:6])
	encoding.ImplicitVR = true
	for _, vr := range RecognisedVRs {
		if vr == vrfrombytes {
			// likely explicit VR
			encoding.ImplicitVR = false
		}
	}
	success = true
	return
}

// guessTransferSyntax is a heuristic for determining the in-use transfer syntax
// 1. First, try to get tag and VR from first 6 bytes as ExplicitVR, LittleEndian
// 2. If bytes zero to two > 2000, its most likely Big Endian
// 3. if bytes four to six match VR string, it's most likely explicit VR
func (df *Dicom) guessTransferSyntax() (encoding Encoding, success bool) {
	peeked, err := df.elementStream.reader.Peek(6)
	if err == nil {
		encoding, success = guessTransferSyntaxFromBytes(peeked)
	}
	return
}

// crawlElements attempts to retrieve all remaining elements from the reader.
// See ``7.1 Data Elements`` for more information.
func (df *Dicom) crawlElements() error {
	// change transfer syntax if necessary
	tsElement, found := df.GetElement(0x00020010)
	if found {
		if transfersyntaxuid, ok := tsElement.Value().(string); ok {
			supported := checkTransferSyntaxSupport(transfersyntaxuid)
			if !supported {
				return UnsupportedDicomError("transfer syntax %s is unsupported", transfersyntaxuid)
			}
			df.elementStream.SetTransferSyntax(transfersyntaxuid)
		} else {
			return CorruptDicomError("TransferSyntaxUID is corrupt")
		}
	} else {
		df.elementStream.SetTransferSyntax("1.2.840.10008.1.2")
		encoding, success := df.guessTransferSyntax()
		if success {
			df.elementStream.TransferSyntax.Encoding = &encoding
			Debugf("guessed transfer syntax encoding: %s", df.elementStream.TransferSyntax.Encoding)
		} else {
			return CorruptDicomError("missing transfer syntax tag in file, and could not guess transfer syntax")
		}
	}
	for {
		if df.elementStream.GetPosition() >= df.elementStream.readerSize {
			break
		}
		element, err := df.elementStream.GetElement()
		if err != nil {
			switch err.(type) {
			case *CorruptElement:
				return CorruptDicomError("crawlElements(): %v", err)
			case *UnsupportedDicom:
				return UnsupportedDicomError("crawlElements(): %v", err)
			default:
				panic(err)
			}

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
	dcm.reader = ReaderPool.Get(f)
	defer func() {
		ReaderPool.Put(dcm.reader)
	}()
	dcm.elementStream = NewElementStream(dcm.reader, fileSize)
	if err := dcm.crawlMeta(); err != nil {
		switch err.(type) {
		case *CorruptDicom:
			return dcm, CorruptDicomError(`the file "%s" is corrupt: %v`, filepath.Base(path), err)
		case *UnsupportedDicom:
			return dcm, UnsupportedDicomError(`the file "%s" is unsupported: %v`, filepath.Base(path), err)
		default:
			panic(err)
		}
	}
	if err := dcm.crawlElements(); err != nil {
		switch err.(type) {
		case *CorruptDicom:
			return dcm, CorruptDicomError(`the file "%s" is corrupt: %v`, filepath.Base(path), err)
		case *UnsupportedDicom:
			return dcm, UnsupportedDicomError(`the file "%s" is unsupported: %v`, filepath.Base(path), err)
		default:
			panic(err)
		}

	}

	return dcm, nil
}

// ParseFromBytes parses a dicom from a bytestream
func ParseFromBytes(source []byte) (Dicom, error) {
	dcm := Dicom{}

	dcm.reader = ReaderPool.Get(bytes.NewReader(source))
	defer func() {
		ReaderPool.Put(dcm.reader)
	}()
	dcm.elementStream = NewElementStream(dcm.reader, int64(len(source)))
	dcm.Elements = make(map[uint32]Element)

	if err := dcm.crawlMeta(); err != nil {
		switch err.(type) {
		case *CorruptDicom:
			return dcm, CorruptDicomError(`the input bytes are corrupt: %v`, err)
		case *UnsupportedDicom:
			return dcm, UnsupportedDicomError(`the input bytes are unsupported: %v`, err)
		default:
			panic(err)
		}
	}
	if err := dcm.crawlElements(); err != nil {
		switch err.(type) {
		case *CorruptDicom:
			return dcm, CorruptDicomError(`the input bytes are corrupt: %v`, err)
		case *UnsupportedDicom:
			return dcm, UnsupportedDicomError(`the input bytes are unsupported: %v`, err)
		default:
			panic(err)
		}
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
