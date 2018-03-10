// Package dicom implements functionality to parse dicom files
package dicom

import (
	"fmt"

	"github.com/b71729/opendcm/dictionary"
)

// TransferSyntax provides a link between dictionary `UIDEntry` and encoding (byteorder, implicit/explicit VR)
type TransferSyntax struct {
	UIDEntry *dictionary.UIDEntry
	Encoding *Encoding
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

// Encoding represents the expected encoding of dicom attributes. See TransferSyntaxToEncodingMap.
type Encoding struct {
	ImplicitVR   bool
	LittleEndian bool
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

// TransferSyntaxToEncodingMap provides a mapping between transfer syntax UID and encoding
// I couldn't find this mapping in the NEMA documents.
var TransferSyntaxToEncodingMap = map[string]*Encoding{
	"1.2.840.10008.1.2":      {ImplicitVR: true, LittleEndian: true},
	"1.2.840.10008.1.2.1":    {ImplicitVR: false, LittleEndian: true},
	"1.2.840.10008.1.2.1.99": {ImplicitVR: false, LittleEndian: true},
	"1.2.840.10008.1.2.2":    {ImplicitVR: false, LittleEndian: false},
}

// GetEncodingForTransferSyntax returns the encoding for a given TransferSyntax, or defaults.
func GetEncodingForTransferSyntax(ts TransferSyntax) *Encoding {
	if ts.UIDEntry != nil {
		encoding, found := TransferSyntaxToEncodingMap[ts.UIDEntry.UID]
		if found {
			return encoding
		}
	}
	return TransferSyntaxToEncodingMap["1.2.840.10008.1.2.1"] // fallback (default)
}
