package core

import (
	"fmt"
	"log"

	"github.com/b71729/opendcm/dictionary"
)

type TransferSyntax struct {
	UIDEntry *dictionary.UIDEntry
	Encoding *Encoding
}

// https://nathanleclaire.com/blog/2014/08/09/dont-get-bitten-by-pointer-vs-non-pointer-method-receivers-in-golang/
func (ts *TransferSyntax) SetFromUID(uidstr string) error {
	uidptr, err := LookupUID(uidstr)
	if err != nil {
		return err
	}
	ts.UIDEntry = uidptr
	ts.Encoding = GetEncodingForTransferSyntax(*ts)
	log.Printf("TransferSyntax: %s (%s)", ts.UIDEntry.NameHuman, ts.Encoding)
	return nil
}

// Encoding represents the expected encoding of dicom attributes. See TransferSyntaxToEncodingMap.
type Encoding struct {
	ImplicitVR   bool
	LittleEndian bool
}

func (e Encoding) String() string {
	var s1 = "ImplicitVR"
	var s2 = "LittleEndian"
	if !e.ImplicitVR {
		s1 = "ExplicitVR"
	}
	if !e.LittleEndian {
		s2 = "BigEndian"
	}
	return fmt.Sprintf("%s + %s", s1, s2)
}

// TransferSyntaxToEncodingMap provides a mapping between transfer syntax UID and encoding
// I couldn't find this mapping in the NEMA documents.
var TransferSyntaxToEncodingMap = map[string]*Encoding{
	"1.2.840.10008.1.2":      &Encoding{ImplicitVR: true, LittleEndian: true},
	"1.2.840.10008.1.2.1":    &Encoding{ImplicitVR: false, LittleEndian: true},
	"1.2.840.10008.1.2.1.99": &Encoding{ImplicitVR: false, LittleEndian: true},
	"1.2.840.10008.1.2.2":    &Encoding{ImplicitVR: false, LittleEndian: false},
}

// GetEncodingForTransferSyntax returns the encoding for a given TransferSyntax, or defaults.
func GetEncodingForTransferSyntax(ts TransferSyntax) *Encoding {
	if ts.UIDEntry != nil {
		encoding, ok := TransferSyntaxToEncodingMap[ts.UIDEntry.UID]
		if ok {
			return encoding
		}
	}
	return TransferSyntaxToEncodingMap["1.2.840.10008.1.2.1"] // fallback (default)
}
