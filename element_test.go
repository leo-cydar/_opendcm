package opendcm

import (
	"errors"
)

// Utils

// validUL1 contains one item of defined length
// ImplicitVR, LittleEndian
var validUL1 = []byte{
	0xFE, 0xFF, 0x00, 0xE0, // StartItem Tag
	0x0C, 0x00, 0x00, 0x00, // Item total length: 12 bytesf
	0x01, 0x7F, 0x34, 0x12, // (7F01,1234) Tag
	0x04, 0x00, 0x00, 0x00, // Length: 4 bytes
	0x4C, 0x65, 0x6F, 0x00, // Data: "Leo"+NULL
	0xFE, 0xFF, 0xDD, 0xE0, // SequenceDelimItem
	0x00, 0x00, 0x00, 0x00, // Filler: 4 bytes
}

// validUL2 contains one item of undefined length
// ImplicitVR, LittleEndian
var validUL2 = []byte{
	0xFE, 0xFF, 0x00, 0xE0, // StartItem Tag
	0xFF, 0xFF, 0xFF, 0xFF, // Item total length: undefined
	0x01, 0x7F, 0x34, 0x12, // (7F01,1234) Tag
	0x04, 0x00, 0x00, 0x00, // Length: 4 bytes
	0x4C, 0x65, 0x6F, 0x00, // Data: "Leo"+NULL
	0xFE, 0xFF, 0x0D, 0xE0, // ItemEnd Tag
	0x00, 0x00, 0x00, 0x00, // ItemEnd Length: 0
	0xFE, 0xFF, 0xDD, 0xE0, // SequenceDelimItem
	0x00, 0x00, 0x00, 0x00, // Filler: 4 bytes
}

type devNull int

// devNull implements `io.Reader` and `io.Writer` to remove reader-specific impact on benchmarks
var blackHole = devNull(0)

func (devNull) Read(p []byte) (int, error) {
	return len(p), nil
}

func (devNull) Write(p []byte) (int, error) {
	return len(p), nil
}

type failAfterN struct {
	pos       int
	failAfter int
}

func (w *failAfterN) Write(p []byte) (int, error) {
	if w.failAfter <= w.pos {
		return 0, errors.New("error")
	}
	w.pos += len(p)
	return 0, nil
}

func (w *failAfterN) Read(p []byte) (int, error) {
	if w.failAfter <= w.pos {
		return 0, errors.New("error")
	}
	w.pos += len(p)
	return len(p), nil
}

/*
===============================================================================
    DataSet
===============================================================================
*/
