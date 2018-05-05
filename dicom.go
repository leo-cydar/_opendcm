package opendcm

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"

	"github.com/b71729/bin"
)

/*
===============================================================================
    Dicom
===============================================================================
*/

// Dicom represents a file containing one SOP Instance
// as per http://dicom.nema.org/dicom/2013/output/chtml/part10/chapter_7.html
type Dicom struct {
	preamble [128]byte
	DataSet
	tmpBuffers
}

// GetPreamble returns the "preamble" component
func (dcm *Dicom) GetPreamble() [128]byte {
	return dcm.preamble
}

// NewDicom returns a fresh Dicom suitable for parsing
// dicom data.
func newDicom() Dicom {
	dcm := Dicom{}
	dcm.DataSet = make(DataSet, 0)
	return dcm
}

// tmpBuffers provides an assortment of temporary variables used internally
// to reduce allocation overhead.
//
// These variables are **not** safe for concurrent use; can consider the use
// of Mutex if the need arises.
type tmpBuffers struct {
	_1kb  [1024]byte
	err   error
	i     int
	_bool bool
	i64   int64
	ui16  uint16
	ui32  uint32
}

// dicmTestString contains the dicom magic value
var dicmTestString = []byte("DICM")

// RecognisedVRs lists all recognised VRs.
// See ``6.2 Value Representation (VR)`` for more information
var RecognisedVRs = []string{
	"AE", "AS", "AT", "CS", "DA", "DS", "DT", "FL", "FD", "IS", "LO", "LT", "OB", "OD",
	"OF", "OW", "PN", "SH", "SL", "SQ", "SS", "ST", "TM", "UI", "UL", "UN", "US", "UT",
}

// attemptReadPreamble attempts to decode the "preamble"
func (dcm *Dicom) attemptReadPreamble(br *bin.Reader) (bool, error) {
	preamble := make([]byte, 132)
	if dcm.err = br.Peek(preamble); dcm.err != nil {
		return false, dcm.err
	}
	if bytes.Compare(preamble[128:132], dicmTestString) != 0 {
		return false, dcm.err
	}

	// dicm magic has a match. save preamble and discard bytes from stream
	copy(dcm.preamble[:], preamble[:128])
	br.Discard(132)
	return true, nil
}

// FromReader decodes a dicom file from `source`, returning an error
// if something went wrong during the process.
// This takes ownership of `source`; do not use it after passing through.
func FromReader(source io.Reader) (Dicom, error) {
	dcm := newDicom()
	binaryReader := bin.NewReader(source, binary.LittleEndian)

	// attempt to parse preamble
	dcm._bool, dcm.err = dcm.attemptReadPreamble(&binaryReader)
	if dcm.err != nil {
		return dcm, dcm.err
	}
	if !dcm._bool {
		Debug("file is missing preamble/magic (bytes 0-132)")
	}

	elr := NewElementReader(binaryReader)
	// meta elements are always explicit vr, little endian
	elr.SetImplicitVR(false)
	elr.SetLittleEndian(true)

	// read elements
	inMeta := true
	// initialise array of elements
	elements := make([]Element, 0)
	e := NewElement()
	for {
		if inMeta {
			// if in meta section, we should read the first two
			// bytes (first component of tag) to determine whether
			// we have reached boundary of meta section
			if dcm.err = elr.br.Peek(dcm._1kb[:2]); dcm.err != nil {
				if dcm.err == io.EOF {
					break
				}
				return dcm, dcm.err
			}
			// if the first component is not (0002), we have reached end
			// of meta section
			if binary.LittleEndian.Uint16(dcm._1kb[:2]) != 0x0002 {
				inMeta = false
				// determine binary encoding of non-meta section
				// we do this by peeking six bytes from the reader
				// and passing through to `determineEncoding`
				if dcm.err = elr.br.Peek(dcm._1kb[:6]); dcm.err != nil {
					if dcm.err == io.EOF {
						break
					}
					return dcm, dcm.err
				}
				elr.determineEncoding(dcm._1kb[:6])
			}
		}
		if dcm.err = elr.ReadElement(&e); dcm.err != nil {
			if dcm.err == io.EOF {
				break
			}
			return dcm, dcm.err
		}
		//Debugf("Adding element: %s [%s] @ %d", e.dictEntry, e.GetVR(), elr.br.GetPosition())
		switch e.GetTag() {
		case 0x00080005:
			dcm.addElement(e)
		default:
			elements = append(elements, e)
		}
	}

	// we must re-encode the parsed elements from their native characterset into UTF-8:
	// lookup character set according to the pre-defined table
	cs := dcm.GetCharacterSet()
	Debugf("CS: %v", cs.Name)
	decoder := cs.Encoding.NewDecoder()
	// for each element in dataset:
	for _, e = range elements {
		// 	is it of ("SH", "LO", "ST", "PN", "LT", "UT")?
		switch e.GetVR() {
		case "SH", "LO", "ST", "PN", "LT", "UT":
			// if so, decode data in-place
			e.data, _ = decoder.Bytes(e.data) // this will not result in an error as replacement runes are enforced
		}
		dcm.addElement(e)
	}
	return dcm, nil
}

// FromFile decodes a dicom file from the given file path
// See: FromReader for more information
func FromFile(path string) (Dicom, error) {
	var f *os.File
	dcm := newDicom()
	if f, dcm.err = os.Open(path); dcm.err != nil {
		return dcm, dcm.err
	}
	defer f.Close()
	return FromReader(f)
}
