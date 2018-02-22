package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

type DicomFileMeta struct {
	Preamble [128]byte
}

type DicomFile struct {
	filepath string
	Reader   DicomFileReader
	Meta     DicomFileMeta
	Elements map[uint32]Element
}

func (df DicomFile) GetElement(tag uint32) (Element, bool) {
	e, ok := df.Elements[tag]
	return e, ok
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

type UIDEntry struct {
	UID       string
	Type      string
	NameHuman string
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

func LookupUID(uid string) (*UIDEntry, error) {
	val, ok := UIDDictionary[uid]
	if !ok {
		return &UIDEntry{}, errors.New("could not find UID")
	}
	return val, nil
}

func (e Element) Value() interface{} {
	switch e.VR {
	case "UI", "SH", "UT", "ST", "PN", "OW", "LT", "IS", "DS", "CS", "AS", "AE", "LO":
		return string(e.value.Bytes())
	case "UL":
		return binary.LittleEndian.Uint32(e.value.Bytes())
	case "US":
		return binary.LittleEndian.Uint16(e.value.Bytes())
	default:
		return e.value.Bytes()
	}
}
