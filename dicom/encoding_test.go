package dicom

import (
	"testing"

	"github.com/b71729/opendcm/dictionary"
)

func TestUrecognisedSetFromUID(t *testing.T) {
	t.Parallel()
	ts := TransferSyntax{}
	err := ts.SetFromUID("1.1.1.1.1.1.1.1")
	if err == nil {
		t.Fatal("SetFromUID with unrecognised UID should return error")
	}
}

func TestRecognisedSetFromUID(t *testing.T) {
	t.Parallel()
	ts := TransferSyntax{}
	err := ts.SetFromUID("1.2.840.10008.1.2.2")
	if err != nil {
		t.Fatalf("SetFromUID returned error: %v", err)
	}
	if ts.Encoding.ImplicitVR {
		t.Fatalf("1.2.840.10008.1.2.2 should be Explicit VR")
	}
	if ts.Encoding.LittleEndian {
		t.Fatalf("1.2.840.10008.1.2.2 should be Big Endian")
	}
}

func TestEncodingStringRepresentation(t *testing.T) {
	t.Parallel()
	encoding := TransferSyntaxToEncodingMap["1.2.840.10008.1.2"]
	str := encoding.String()
	expected := "ImplicitVR + LittleEndian"
	if str != expected {
		t.Fatalf(`got "%s" (!= "%s")`, str, expected)
	}

	encoding = TransferSyntaxToEncodingMap["1.2.840.10008.1.2.2"]
	str = encoding.String()
	expected = "ExplicitVR + BigEndian"
	if str != expected {
		t.Fatalf(`got "%s" (!= "%s")`, str, expected)
	}
}

func TestUnrecognisedGetEncodingForTransferSyntax(t *testing.T) {
	t.Parallel()
	ts := TransferSyntax{UIDEntry: &dictionary.UIDEntry{UID: "1.1.1.1.1.1"}}
	encoding := GetEncodingForTransferSyntax(ts)
	if encoding != TransferSyntaxToEncodingMap["1.2.840.10008.1.2.1"] {
		t.Fatalf("encoding did not match expected encoding for unrecognised TS")
	}
}