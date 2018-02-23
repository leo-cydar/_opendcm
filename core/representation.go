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
	filepath       string
	Reader         DicomFileReader
	Meta           DicomFileMeta
	TotalMetaBytes int64
	Elements       map[uint32]Element
}

func (df DicomFile) GetElement(tag uint32) (Element, bool) {
	e, ok := df.Elements[tag]
	return e, ok
}

type Element struct {
	*dictionary.DictEntry
	ValueLength uint32
	value       *bytes.Buffer
	Items       []Item
}

type Item struct {
	Length uint32
	Value  *bytes.Buffer
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

// ByAge implements sort.Interface for []Person based on
// the Age field.
type ByTag []Element

func (a ByTag) Len() int           { return len(a) }
func (a ByTag) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByTag) Less(i, j int) bool { return a[i].Tag < a[j].Tag }

func (e Element) Value() interface{} {
	if e.value == nil {
		if len(e.Items) > 0 {
			return e.Items
		}
		panic("called Element.Value() but neither value set, nor Items")
	}
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
