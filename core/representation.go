package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"

	"golang.org/x/text/encoding/korean"

	"golang.org/x/text/encoding/simplifiedchinese"

	"golang.org/x/text/encoding/unicode"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"

	"golang.org/x/text/encoding/charmap"

	"github.com/b71729/opendcm/dictionary"
)

type DicomFile struct {
	FilePath       string
	elementStream  ElementStream
	Preamble       [128]byte
	TotalMetaBytes int64
	Elements       map[uint32]Element
}

type DicomFileChannel struct {
	DicomFile DicomFile
	Error     error
}

// GetElement returns an Element inside the DicomFile according to `tag`.
// If the tag is not found, param `bool` will be false.
func (df DicomFile) GetElement(tag uint32) (Element, bool) {
	e, ok := df.Elements[tag]
	return e, ok
}

// Element represents a data element (see: NEMA 7.1 Data Elements)
type Element struct {
	*dictionary.DictEntry
	ValueLength         uint32
	value               *bytes.Buffer
	sourceElementStream *ElementStream
	Items               []Item
}

type CharacterSet struct {
	Name        string
	Description string
	Encoding    encoding.Encoding
	decoder     *encoding.Decoder
	encoder     *encoding.Encoder
	// TODO: EncodeFunc
}

var CharacterSetMap = map[string]*CharacterSet{
	"Default":    &CharacterSet{Name: "Default", Description: "Default Character Repertoire", Encoding: unicode.UTF8},
	"ISO_IR 13":  &CharacterSet{Name: "ISO_IR 13", Description: "Japanese", Encoding: japanese.ShiftJIS},
	"ISO_IR 100": &CharacterSet{Name: "ISO_IR 100", Description: "Latin alphabet No. 1", Encoding: charmap.ISO8859_1},
	"ISO_IR 101": &CharacterSet{Name: "ISO_IR 101", Description: "Latin alphabet No. 2", Encoding: charmap.ISO8859_2},
	"ISO_IR 109": &CharacterSet{Name: "ISO_IR 109", Description: "Latin alphabet No. 3", Encoding: charmap.ISO8859_3},
	"ISO_IR 110": &CharacterSet{Name: "ISO_IR 110", Description: "Latin alphabet No. 4", Encoding: charmap.ISO8859_4},
	"ISO_IR 126": &CharacterSet{Name: "ISO_IR 144", Description: "Greek", Encoding: charmap.ISO8859_7},
	"ISO_IR 127": &CharacterSet{Name: "ISO_IR 144", Description: "Arabic", Encoding: charmap.ISO8859_6},
	"ISO_IR 138": &CharacterSet{Name: "ISO_IR 138", Description: "Hebrew", Encoding: charmap.ISO8859_8},
	"ISO_IR 144": &CharacterSet{Name: "ISO_IR 144", Description: "Cyrillic", Encoding: charmap.ISO8859_5},
	"ISO_IR 148": &CharacterSet{Name: "ISO_IR 148", Description: "Latin alphabet No. 5", Encoding: charmap.ISO8859_9},
	"ISO_IR 166": &CharacterSet{Name: "ISO_IR 166", Description: "Thai", Encoding: charmap.Windows874},
	"ISO_IR 192": &CharacterSet{Name: "ISO_IR 192", Description: "Unicode (UTF-8)", Encoding: unicode.UTF8},

	"ISO 2022 IR 6":   &CharacterSet{Name: "ISO 2022 IR 6", Description: "ASCII", Encoding: unicode.UTF8},
	"ISO 2022 IR 13":  &CharacterSet{Name: "ISO 2022 IR 13", Description: "Japanese (Shift JIS)", Encoding: japanese.ShiftJIS},
	"ISO 2022 IR 87":  &CharacterSet{Name: "ISO 2022 IR 87", Description: "Japanese (Kanji)", Encoding: japanese.ISO2022JP},
	"ISO 2022 IR 100": &CharacterSet{Name: "ISO 2022 IR 100", Description: "Latin alphabet No. 1", Encoding: charmap.ISO8859_1},
	"ISO 2022 IR 101": &CharacterSet{Name: "ISO 2022 IR 101", Description: "Latin alphabet No. 2", Encoding: charmap.ISO8859_2},
	"ISO 2022 IR 109": &CharacterSet{Name: "ISO 2022 IR 109", Description: "Latin alphabet No. 3", Encoding: charmap.ISO8859_3},
	"ISO 2022 IR 110": &CharacterSet{Name: "ISO 2022 IR 110", Description: "Latin alphabet No. 4", Encoding: charmap.ISO8859_4},
	"ISO 2022 IR 127": &CharacterSet{Name: "ISO 2022 IR 127", Description: "Arabic", Encoding: charmap.ISO8859_6},
	"ISO 2022 IR 138": &CharacterSet{Name: "ISO 2022 IR 138", Description: "Hebrew", Encoding: charmap.ISO8859_8},
	"ISO 2022 IR 144": &CharacterSet{Name: "ISO 2022 IR 144", Description: "Cyrillic", Encoding: charmap.ISO8859_5},
	"ISO 2022 IR 148": &CharacterSet{Name: "ISO 2022 IR 148", Description: "Latin alphabet No. 5", Encoding: charmap.ISO8859_9},
	"ISO 2022 IR 149": &CharacterSet{Name: "ISO 2022 IR 149", Description: "Korean", Encoding: korean.EUCKR}, // TODO
	"ISO 2022 IR 159": &CharacterSet{Name: "ISO 2022 IR 159", Description: "Japanese (Supplementary Kanji)", Encoding: japanese.ISO2022JP},
	"ISO 2022 IR 166": &CharacterSet{Name: "ISO 2022 IR 166", Description: "Thai", Encoding: charmap.Windows874},
	"GB18030":         &CharacterSet{Name: "GB18030", Description: "Chinese (Simplified)", Encoding: simplifiedchinese.GB18030},
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

// GetElement returns an Element inside the DicomFile according to `tag`.
// If the tag is not found, param `bool` will be false.
func (i Item) GetElement(tag uint32) (Element, bool) {
	e, ok := i.Elements[tag]
	return e, ok
}

// LookupTag searches for the corresponding `dictionary.DicomDictionary` entry for the given tag uint32
func LookupTag(t uint32) (*dictionary.DictEntry, bool) {
	val, ok := dictionary.DicomDictionary[t]
	if !ok {
		tag := dictionary.Tag(t)
		name := fmt.Sprintf("Unknown%s", tag)
		return &dictionary.DictEntry{Tag: tag, Name: name, NameHuman: name, VR: "UN", Retired: false}, false
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
				description = append(description, fmt.Sprintf("     - %s [%s] %v", e.Tag, e.VR, e.Value()))
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

	if !e.CheckConformance() {
		description[0] = fmt.Sprintf("!! %s", description[0])
	}
	return description
}

// Value returns an abstraction layer to the underlying bytestream according to VR
func (e Element) Value() interface{} {
	if e.value == nil || e.ValueLength == 0 { // check both to be sure
		if len(e.Items) > 0 {
			return e.Items
		}
		return nil // neither value nor items set -- contents are empty
	}
	switch e.VR { // string
	case "SH", "LO", "ST", "PN", "LT", "UT":
		decoded, err := decodeBytes(e.value.Bytes(), e.sourceElementStream.CharacterSet)
		if err != nil {
			return nil
		}
		return decoded
	case "IS", "DS", "TM", "DA", "DT", "UI", "CS", "AS", "AE":
		return string(e.value.Bytes())
	case "FL": // float
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return float32(binary.LittleEndian.Uint32(e.value.Bytes()))
		}
		return float32(binary.BigEndian.Uint32(e.value.Bytes()))
	case "FD": // double
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return float64(binary.LittleEndian.Uint64(e.value.Bytes()))
		}
		return float64(binary.BigEndian.Uint64(e.value.Bytes()))
	case "SS": // short
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return int16(binary.LittleEndian.Uint16(e.value.Bytes()))
		}
		return int16(binary.BigEndian.Uint16(e.value.Bytes()))
	case "SL": // long
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return int32(binary.LittleEndian.Uint32(e.value.Bytes()))
		}
		return int32(binary.BigEndian.Uint32(e.value.Bytes()))
	case "US": // ushort
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return binary.LittleEndian.Uint16(e.value.Bytes())
		}
		return binary.BigEndian.Uint16(e.value.Bytes())
	case "UL": // ulong
		if e.sourceElementStream.TransferSyntax.Encoding.LittleEndian {
			return binary.LittleEndian.Uint32(e.value.Bytes())
		}
		return binary.BigEndian.Uint32(e.value.Bytes())
	// TODO: OW and OF require byteswapping too, but am unable to find sample datasets to validate method.
	// Other libraries seem to not byteswap, and instead defer to consumer.
	default:
		return e.value.Bytes()
	}
}

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
			log.Printf("Found element: %s", v.Tag)
			buffer = append(buffer, v.ValueBytes()...)
		}
	}

	return buffer
}
