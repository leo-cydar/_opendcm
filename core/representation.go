package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/b71729/opendcm/dictionary"
)

type DicomFile struct {
	FilePath       string
	ElementReader  ElementReader
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
	ValueLength  uint32
	value        *bytes.Buffer
	LittleEndian bool
	Items        []Item
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

// RepresentationFromBuffer returns an appropriate representation of the underlying bytestream according to VR
func RepresentationFromBuffer(buffer *bytes.Buffer, VR string, LittleEndian bool) interface{} {
	switch VR { // string
	case "UI", "SH", "UT", "ST", "PN", "LT", "IS", "DS", "CS", "AS", "AE", "LO", "TM", "DA", "DT":
		return string(buffer.Bytes())
	case "FL": // float
		if LittleEndian {
			return float32(binary.LittleEndian.Uint32(buffer.Bytes()))
		}
		return float32(binary.BigEndian.Uint32(buffer.Bytes()))
	case "FD": // double
		if LittleEndian {
			return float64(binary.LittleEndian.Uint64(buffer.Bytes()))
		}
		return float64(binary.BigEndian.Uint64(buffer.Bytes()))
	case "SS": // short
		if LittleEndian {
			return int32(binary.LittleEndian.Uint32(buffer.Bytes()))
		}
		return int32(binary.BigEndian.Uint32(buffer.Bytes()))
	case "SL": // long
		if LittleEndian {
			return int64(binary.LittleEndian.Uint64(buffer.Bytes()))
		}
		return int64(binary.BigEndian.Uint64(buffer.Bytes()))
	case "US": // ushort
		if LittleEndian {
			return binary.LittleEndian.Uint16(buffer.Bytes())
		}
		return binary.BigEndian.Uint16(buffer.Bytes())
	case "UL": // ulong
		if LittleEndian {
			return binary.LittleEndian.Uint32(buffer.Bytes())
		}
		return binary.BigEndian.Uint32(buffer.Bytes())

	default:
		return buffer.Bytes()
	}
}

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
	return RepresentationFromBuffer(e.value, e.VR, e.LittleEndian)
}
