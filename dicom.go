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
    DicomFile
===============================================================================
*/

// DicomFile represents a file containing one SOP Instance
// as per http://dicom.nema.org/dicom/2013/output/chtml/part10/chapter_7.html
type DicomFile struct {
	dataset  DataSet
	preamble [128]byte
	tmpBuffers
}

// GetPreamble returns the "preamble" component
func (df *DicomFile) GetPreamble() [128]byte {
	return df.preamble
}

// GetDataSet returns the parsed DataSet (elements)
func (df *DicomFile) GetDataSet() *DataSet {
	return &df.dataset
}

// NewDicomFile returns a fresh DicomFile suitable for parsing
// dicom data.
func NewDicomFile() DicomFile {
	df := DicomFile{}
	df.dataset = NewDataSet()
	return df
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
func (df *DicomFile) attemptReadPreamble(br *bin.Reader) (bool, error) {
	preamble := make([]byte, 132)
	if df.err = br.Peek(preamble); df.err != nil {
		return false, df.err
	}
	if bytes.Compare(preamble[128:132], dicmTestString) != 0 {
		return false, df.err
	}

	// dicm magic has a match. save preamble and discard bytes from stream
	copy(df.preamble[:], preamble[:128])
	br.Discard(132)
	return true, nil
}

// FromReader decodes a dicom file from `source`, returning an error
// if something went wrong during the process.
// This takes ownership of `source`; do not use it after passing through.
func (df *DicomFile) FromReader(source io.Reader) error {
	binaryReader := bin.NewReader(source, binary.LittleEndian)

	// attempt to parse preamble
	df._bool, df.err = df.attemptReadPreamble(&binaryReader)
	if df.err != nil {
		return df.err
	}
	if !df._bool {
		Debug("file is missing preamble/magic (bytes 0-132)")
	}

	elr := NewElementReader(binaryReader)
	// meta elements are always explicit vr, little endian
	elr.SetImplicitVR(false)
	elr.SetLittleEndian(true)

	// read elements
	inMeta := true
	for {
		if inMeta {
			// if in meta section, we should read the first two
			// bytes (first component of tag) to determine whether
			// we have reached boundary of meta section
			if df.err = elr.br.Peek(df._1kb[:2]); df.err != nil {
				if df.err == io.EOF {
					return nil
				}
				return df.err
			}
			// if the first component is not (0002), we have reached end
			// of meta section
			if binary.LittleEndian.Uint16(df._1kb[:2]) != 0x0002 {
				inMeta = false
				// determine binary encoding of non-meta section
				// we do this by peeking six bytes from the reader
				// and passing through to `determineEncoding`
				if df.err = elr.br.Peek(df._1kb[:6]); df.err != nil {
					if df.err == io.EOF {
						return nil
					}
					return df.err
				}
				elr.determineEncoding(df._1kb[:6])
			}
		}
		e := Element{}
		if df.err = elr.ReadElement(&e); df.err != nil {
			if df.err == io.EOF {
				return nil
			}
			return df.err
		}
		Debugf("Adding element: %08X (%s) @ %d", e.GetTag(), e.GetVR(), elr.br.GetPosition())
		df.GetDataSet().AddElement(e)
	}
}

// FromFile decodes a dicom file from the given file path
// See: FromReader for more information
func (df *DicomFile) FromFile(path string) error {
	var f *os.File
	if f, df.err = os.Open(path); df.err != nil {
		return df.err
	}
	defer f.Close()
	if df.err = df.FromReader(f); df.err != nil {
		return df.err
	}
	return nil
}
