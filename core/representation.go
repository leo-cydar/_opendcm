package core

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type DicomFileMeta struct {
	Preamble [128]byte
	Elements []Element
}

type DicomFile struct {
	filepath string
	Meta     DicomFileMeta
}

type Element struct {
	*DictEntry
	ValueLength uint32
	value       *bytes.Buffer
}

type Tag uint32

type DictEntry struct {
	Tag       Tag
	NameHuman string
	Name      string
	VR        string
	Retired   bool
}

func (t Tag) String() string {
	upper := uint32(t) >> 16
	lower := uint32(t) & 0xff
	return fmt.Sprintf("(%04X,%04X)", upper, lower)
}

func LookupTag(t uint32) (*DictEntry, bool) {
	val, ok := DicomDictionary[t]
	if !ok {
		tag := Tag(t)
		name := fmt.Sprintf("Unknown%s", tag)
		return &DictEntry{Tag: tag, Name: name, NameHuman: name, VR: "UN", Retired: false}, false
	}
	return val, ok
}

func (e Element) Value() interface{} {
	switch e.VR {
	case "UI", "SH", "UT", "ST", "PN", "OW", "LT", "IS", "DS", "CS", "AS", "AE":
		return string(e.value.Bytes())
	case "UL":
		return binary.LittleEndian.Uint32(e.value.Bytes())
	case "US":
		return binary.LittleEndian.Uint16(e.value.Bytes())
	default:
		return e.value.Bytes()
	}
}
