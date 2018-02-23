package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/b71729/opendcm/dictionary"
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
	*dictionary.DictEntry
	ValueLength uint32
	value       *bytes.Buffer
}

func LookupTag(t uint32) (*dictionary.DictEntry, bool) {
	val, ok := dictionary.DicomDictionary[t]
	if !ok {
		tag := dictionary.Tag(t)
		name := fmt.Sprintf("Unknown%s", tag)
		return &dictionary.DictEntry{Tag: tag, Name: name, NameHuman: name, VR: "UN", Retired: false}, false
	}
	return val, ok
}

func LookupUID(uid string) (*dictionary.UIDEntry, error) {
	val, ok := dictionary.UIDDictionary[uid]
	if !ok {
		return &dictionary.UIDEntry{}, errors.New("could not find UID")
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
