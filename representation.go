package opendcm

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/b71729/opendcm/dictionary"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

// Dicom provides a link between components that make up a parsed DICOM file
type Dicom struct {
	FilePath       string
	reader         *bufio.Reader
	elementStream  ElementStream
	Preamble       [128]byte
	TotalMetaBytes int64
	Elements       map[uint32]Element
}

// GetElement returns an Element inside the Dicom according to `tag`.
// If the tag is not found, param `bool` will be false.
func (df Dicom) GetElement(tag uint32) (Element, bool) {
	e, ok := df.Elements[tag]
	return e, ok
}

// Element represents a data element (see: NEMA 7.1 Data Elements)
type Element struct {
	*dictionary.DictEntry
	ValueLength         uint32
	ByteLengthTotal     int64
	FileOffsetStart     int64
	value               *bytes.Buffer
	sourceElementStream *ElementStream
	Items               []Item
}

// CharacterSet provides a link between character encoding, description, and decode + encode functions.
type CharacterSet struct {
	Name        string
	Description string
	Encoding    encoding.Encoding
	decoder     *encoding.Decoder
	encoder     *encoding.Encoder
}

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

// Item represents a nested Item within a Sequence (see: NEMA 7.5 Nesting of Data Sets)
type Item struct {
	Elements        map[uint32]Element
	UnknownSections [][]byte
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

// ByTag implements a sort interface
type ByTag []Element

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
	// 	description[0] = fmt.Sprintf("!! %s", description[0])
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
		return float64(binary.BigEndian.Uint64(e.value.Bytes()))
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
	// TODO: Check whether ValueBytes() is necessary, or whether e.value.Bytes() would suffice
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
		return e.value.Bytes()
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
