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
	dcm := NewDicom()
	assert.Equal(t, [128]byte{}, dcm.GetPreamble())

	// populate preamble and retry
	preamble := [128]byte{}
	preamble[0] = byte(0xEE)
	preamble[1] = byte(0xDD)
	dcm.preamble = preamble
	assert.Equal(t, preamble, dcm.GetPreamble())
}

func TestGetDataSet(t *testing.T) {
	t.Parallel()
	dcm := NewDicom()
	// empty dataset
	ds := dcm.GetDataSet()
	assert.Equal(t, 0, ds.Len())

	// couple of entries
	e := NewElement()
	e.dictEntry, _ = lookupTag(0x0072006C)
	dcm.dataset.AddElement(e)
	e.dictEntry, _ = lookupTag(0x00720064)
	dcm.dataset.AddElement(e)
	ds = dcm.GetDataSet()
	assert.Equal(t, 2, ds.Len())
}

func TestNewDicomFile(t *testing.T) {
	t.Parallel()
	assert.IsType(t, Dicom{}, NewDicom())
}

func TestAttemptReadPreamble(t *testing.T) {
	t.Parallel()

	// should return true
	dcm := NewDicom()
	f, err := os.Open(filepath.Join("testdata", "synthetic", "VRTest.dcm"))
	assert.NoError(t, err)
	r := bin.NewReader(f, binary.LittleEndian)
	b, err := dcm.attemptReadPreamble(&r)
	assert.NoError(t, err)
	assert.True(t, b)
	assert.Equal(t, [128]byte{}, dcm.preamble)

	// should return false
	dcm = NewDicom()
	f, err = os.Open(filepath.Join("testdata", "synthetic", "MissingPreambleMagic.dcm"))
	assert.NoError(t, err)
	r = bin.NewReader(f, binary.LittleEndian)
	b, err = dcm.attemptReadPreamble(&r)
	assert.NoError(t, err)
	assert.False(t, b)
}

func TestAttemptReadPreambleError(t *testing.T) {
	t.Parallel()

	// not enough bytes
	dcm := NewDicom()
	r := bin.NewReaderBytes([]byte{}, binary.LittleEndian)
	b, err := dcm.attemptReadPreamble(&r)
	assert.Error(t, err)
	assert.False(t, b)
}

func TestFromReader(t *testing.T) {
	t.Parallel()
	// from file reader
	f, err := os.Open(filepath.Join("testdata", "synthetic", "MissingPreambleMagic.dcm"))
	assert.NoError(t, err)
	dcm := NewDicom()
	assert.NoError(t, dcm.FromReader(f))
	assert.Equal(t, 8, dcm.GetDataSet().Len())
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
	dcm = NewDicom()
	assert.NoError(t, dcm.FromReader(r))
	assert.Equal(t, 8, dcm.GetDataSet().Len())
	f.Close()
}

func TestFromReaderError(t *testing.T) {
	t.Parallel()

	// dicom bytes that are not enough to peek the preamble component
	notEnoughBytes := make([]byte, 100)
	r := bytes.NewReader(notEnoughBytes)
	dcm := NewDicom()
	assert.Error(t, dcm.FromReader(r))

	// dicom bytes that make up a valid preamble component
	// but then abruptly ends; should not return error, but still is
	// unusable
	preambleNoElements := make([]byte, 132)
	preambleNoElements[128] = byte('D')
	preambleNoElements[129] = byte('I')
	preambleNoElements[130] = byte('C')
	preambleNoElements[131] = byte('M')
	r = bytes.NewReader(preambleNoElements)
	assert.NoError(t, dcm.FromReader(r))

	// append one byte, this should cause subsequent element parsing
	// to fail
	preambleNoElements = append(preambleNoElements, byte(0x00))
	r = bytes.NewReader(preambleNoElements)
	assert.Error(t, dcm.FromReader(r))

	// append another byte, should not be an explicit error
	preambleNoElements = append(preambleNoElements, byte(0x00))
	r = bytes.NewReader(preambleNoElements)
	assert.NoError(t, dcm.FromReader(r))

	// append another bytes, this should cause subsequent element
	// parsing to fail
	preambleNoElements = append(preambleNoElements, byte(0x00))
	r = bytes.NewReader(preambleNoElements)
	assert.Error(t, dcm.FromReader(r))

	// append another couple bytes, this also should cause subsequent element
	// parsing to fail
	preambleNoElements = append(preambleNoElements, make([]byte, 2)...)
	r = bytes.NewReader(preambleNoElements)
	assert.Error(t, dcm.FromReader(r))
}

func TestFromFile(t *testing.T) {
	t.Parallel()
	dcm := NewDicom()
	assert.NoError(t, dcm.FromFile(filepath.Join("testdata", "synthetic", "VRTest.dcm")))
	assert.Equal(t, 27, dcm.GetDataSet().Len())
}

func TestFromFileError(t *testing.T) {
	t.Parallel()
	dcm := NewDicom()
	// try to parse dicom from
	// 1: file that does not exist
	// 2: file that exists, but is not a dicom
	for _, path := range []string{"__.__0000", "dicom_test.go"} {
		assert.Error(t, dcm.FromFile(path))
	}
}

func TestFromFileNoPermission(t *testing.T) {
	// try to parse a dicom from a file that the user has no read
	// permissions
	if runtime.GOOS == "windows" {
		t.Skipf("skip (windows)")
	}
	t.Parallel()
	dcm := NewDicom()
	f, err := ioutil.TempFile("", "")
	assert.NoError(t, err)
	assert.NoError(t, f.Chmod(0333))
	defer os.Remove(f.Name())
	assert.Error(t, dcm.FromFile(f.Name()))
}

func TestCharsetDecode(t *testing.T) {
	// ensure that, given a range of charactersets, the output is as expected
	t.Parallel()
	for _, testCase := range []struct {
		filename             string
		expectedCharacterSet string
		expectedPatientName  string
	}{
		{
			filename:             "ShiftJIS.dcm",
			expectedCharacterSet: "ISO_IR 13",
			expectedPatientName:  "エンコードされたメッセージ",
		},
		{
			filename:             "ISO_IR100.dcm",
			expectedCharacterSet: "ISO_IR 100",
			expectedPatientName:  "Encoded Message",
		},
		{
			filename:             "ISO_IR101.dcm",
			expectedCharacterSet: "ISO_IR 101",
			expectedPatientName:  "kódovanej správy",
		},
		{
			filename:             "ISO_IR109.dcm",
			expectedCharacterSet: "ISO_IR 109",
			expectedPatientName:  "messaġġ kodifikat",
		},
		{
			filename:             "ISO_IR110.dcm",
			expectedCharacterSet: "ISO_IR 110",
			expectedPatientName:  "kodeeritud sõnum",
		},
		{
			filename:             "ISO_IR126.dcm",
			expectedCharacterSet: "ISO_IR 126",
			expectedPatientName:  "κωδικοποιημένο μήνυμα",
		},
		{
			filename:             "ISO_IR127.dcm",
			expectedCharacterSet: "ISO_IR 127",
			expectedPatientName:  "رسالة مشفرة",
		},
		{
			filename:             "ISO_IR138.dcm",
			expectedCharacterSet: "ISO_IR 138",
			expectedPatientName:  "הודעה מקודדת",
		},
		{
			filename:             "ISO_IR144.dcm",
			expectedCharacterSet: "ISO_IR 144",
			expectedPatientName:  "закодированное сообщение",
		},
		{
			filename:             "ISO_IR148.dcm",
			expectedCharacterSet: "ISO_IR 148",
			expectedPatientName:  "kodlanmış mesaj",
		},
		{
			filename:             "ISO_IR166.dcm",
			expectedCharacterSet: "ISO_IR 166",
			expectedPatientName:  "ข้อความที่เข้ารหัส",
		},
		{
			filename:             "ISO_IR192.dcm",
			expectedCharacterSet: "ISO_IR 192",
			expectedPatientName:  "Éncø∂é∂ √ålüÉ",
		},
		{
			filename:             "GB18030.dcm", // TODO
			expectedCharacterSet: "GB18030",
			expectedPatientName:  "编码值",
		},
	} {
		dcm := NewDicom()
		assert.NoError(t, dcm.FromFile(filepath.Join("testdata", "synthetic", testCase.filename)))
		assert.Equal(t, testCase.expectedCharacterSet, dcm.GetDataSet().GetCharacterSet().Name)
		name := ""
		var e = NewElement()
		assert.True(t, dcm.GetDataSet().GetElement(0x00100010, &e))
		assert.NoError(t, e.GetValue(&name))
		assert.Equal(t, testCase.expectedPatientName, name)
	}

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
	dcm := NewDicom()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dcm.FromReader(r)
		r.Reset(buf)
	}
}
