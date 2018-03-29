package opendcm

import (
	"bytes"
	"encoding/binary"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/b71729/bin"

	"github.com/stretchr/testify/assert"
)

/*
===============================================================================
    DicomFile
===============================================================================
*/

func TestGetPreamble(t *testing.T) {
	t.Parallel()
	df := NewDicomFile()
	assert.Equal(t, [128]byte{}, df.GetPreamble())

	// populate preamble and retry
	preamble := [128]byte{}
	preamble[0] = byte(0xEE)
	preamble[1] = byte(0xDD)
	df.preamble = preamble
	assert.Equal(t, preamble, df.GetPreamble())
}

func TestGetDataSet(t *testing.T) {
	t.Parallel()
	df := NewDicomFile()
	// empty dataset
	ds := df.GetDataSet()
	assert.Equal(t, 0, ds.Len())

	// couple of entries
	df.dataset.AddElement(Element{tag: 0x00050001})
	df.dataset.AddElement(Element{tag: 0x00020009})
	ds = df.GetDataSet()
	assert.Equal(t, 2, ds.Len())
}

func TestNewDicomFile(t *testing.T) {
	t.Parallel()
	assert.IsType(t, DicomFile{}, NewDicomFile())
}

func TestAttemptReadPreamble(t *testing.T) {
	t.Parallel()

	// should return true
	df := NewDicomFile()
	f, err := os.Open(filepath.Join("testdata", "synthetic", "VRTest.dcm"))
	assert.NoError(t, err)
	r := bin.NewReader(f, binary.LittleEndian)
	b, err := df.attemptReadPreamble(&r)
	assert.NoError(t, err)
	assert.True(t, b)
	assert.Equal(t, [128]byte{}, df.preamble)

	// should return false
	df = NewDicomFile()
	f, err = os.Open(filepath.Join("testdata", "synthetic", "MissingPreambleMagic.dcm"))
	assert.NoError(t, err)
	r = bin.NewReader(f, binary.LittleEndian)
	b, err = df.attemptReadPreamble(&r)
	assert.NoError(t, err)
	assert.False(t, b)
}

func TestAttemptReadPreambleError(t *testing.T) {
	t.Parallel()

	// not enough bytes
	df := NewDicomFile()
	r := bin.NewReaderBytes([]byte{}, binary.LittleEndian)
	b, err := df.attemptReadPreamble(&r)
	assert.Error(t, err)
	assert.False(t, b)
}
func TestFromReader(t *testing.T) {
	t.Parallel()
	// from file reader
	f, err := os.Open(filepath.Join("testdata", "synthetic", "MissingPreambleMagic.dcm"))
	assert.NoError(t, err)
	df := NewDicomFile()
	assert.NoError(t, df.FromReader(f))
	assert.Equal(t, 8, df.GetDataSet().Len())
	f.Close()

	// from byte reader
	f, err = os.Open(filepath.Join("testdata", "synthetic", "MissingPreambleMagic.dcm"))
	stat, err := f.Stat()
	assert.NoError(t, err)

	// read into byte slice
	buf := make([]byte, stat.Size())
	nread, err := f.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, len(buf), nread)
	r := bytes.NewReader(buf)
	df = NewDicomFile()
	assert.NoError(t, df.FromReader(r))
	assert.Equal(t, 8, df.GetDataSet().Len())
	f.Close()
}

func TestFromReaderError(t *testing.T) {
	t.Parallel()

	// dicom bytes that are not enough to peek the preamble component
	notEnoughBytes := make([]byte, 100)
	r := bytes.NewReader(notEnoughBytes)
	df := NewDicomFile()
	assert.Error(t, df.FromReader(r))

	// dicom bytes that make up a valid preamble component
	// but then abruptly ends; should not return error, but still is
	// unusable
	preambleNoElements := make([]byte, 132)
	preambleNoElements[128] = byte('D')
	preambleNoElements[129] = byte('I')
	preambleNoElements[130] = byte('C')
	preambleNoElements[131] = byte('M')
	r = bytes.NewReader(preambleNoElements)
	assert.NoError(t, df.FromReader(r))

	// append one byte, this should cause subsequent element parsing
	// to fail
	preambleNoElements = append(preambleNoElements, byte(0x00))
	r = bytes.NewReader(preambleNoElements)
	assert.Error(t, df.FromReader(r))

	// append another byte, should not be an explicit error
	preambleNoElements = append(preambleNoElements, byte(0x00))
	r = bytes.NewReader(preambleNoElements)
	assert.NoError(t, df.FromReader(r))

	// append another bytes, this should cause subsequent element
	// parsing to fail
	preambleNoElements = append(preambleNoElements, byte(0x00))
	r = bytes.NewReader(preambleNoElements)
	assert.Error(t, df.FromReader(r))

	// append another couple bytes, this also should cause subsequent element
	// parsing to fail
	preambleNoElements = append(preambleNoElements, make([]byte, 2)...)
	r = bytes.NewReader(preambleNoElements)
	assert.Error(t, df.FromReader(r))
}

func TestFromFile(t *testing.T) {
	t.Parallel()
	df := NewDicomFile()
	assert.NoError(t, df.FromFile(filepath.Join("testdata", "synthetic", "VRTest.dcm")))
}

func TestFromFileError(t *testing.T) {
	t.Parallel()
	df := NewDicomFile()
	// try to parse dicom from
	// 1: file that does not exist
	// 2: file that exists, but is not a dicom
	for _, path := range []string{"__.__0000", "dicom_test.go"} {
		assert.Error(t, df.FromFile(path))
	}
}

func TestFromFileNoPermission(t *testing.T) {
	// try to parse a dicom from a file that the user has no read
	// permissions
	if runtime.GOOS == "windows" {
		t.Skipf("skip (windows)")
	}
	t.Parallel()
	df := NewDicomFile()
	f, err := ioutil.TempFile("", "")
	assert.NoError(t, err)
	assert.NoError(t, f.Chmod(0333))
	defer os.Remove(f.Name())
	assert.Error(t, df.FromFile(f.Name()))
}

func BenchmarkFromReader(b *testing.B) {
	// from byte reader
	f, err := os.Open(filepath.Join("testdata", "synthetic", "VRTest.dcm"))
	if err != nil {
		b.Fatal(err)
	}
	stat, err := f.Stat()
	if err != nil {
		b.Fatal(err)
	}

	// read into byte slice
	buf := make([]byte, stat.Size())
	nread, err := f.Read(buf)
	if err != nil {
		b.Fatal(err)
	}
	if nread != len(buf) {
		b.Fatal(nread)
	}
	r := bytes.NewReader(buf)
	df := NewDicomFile()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		df.FromReader(r)
		r.Reset(buf)
	}
}
