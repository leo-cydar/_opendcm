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
	dcm := newDicom()
	assert.Equal(t, [128]byte{}, dcm.GetPreamble())

	// populate preamble and retry
	preamble := [128]byte{}
	preamble[0] = byte(0xEE)
	preamble[1] = byte(0xDD)
	dcm.preamble = preamble
	assert.Equal(t, preamble, dcm.GetPreamble())
}

func TestNewDicom(t *testing.T) {
	t.Parallel()
	assert.IsType(t, Dicom{}, newDicom())
}

func TestAttemptReadPreamble(t *testing.T) {
	t.Parallel()

	// should return true
	dcm := newDicom()
	f, err := os.Open(filepath.Join("testdata", "synthetic", "VRTest.dcm"))
	assert.NoError(t, err)
	r := bin.NewReader(f, binary.LittleEndian)
	b, err := dcm.attemptReadPreamble(&r)
	assert.NoError(t, err)
	assert.True(t, b)
	assert.Equal(t, [128]byte{}, dcm.preamble)

	// should return false
	dcm = newDicom()
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
	dcm := newDicom()
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
	dcm, err := FromReader(f)
	assert.NoError(t, err)
	assert.Equal(t, 8, dcm.Len())
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
	dcm, err = FromReader(r)
	assert.NoError(t, err)
	assert.Equal(t, 8, dcm.Len())
	f.Close()
}

func TestFromReaderError(t *testing.T) {
	t.Parallel()

	// dicom bytes that are not enough to peek the preamble component
	notEnoughBytes := make([]byte, 100)
	r := bytes.NewReader(notEnoughBytes)
	_, err := FromReader(r)
	assert.Error(t, err)

	// dicom bytes that make up a valid preamble component
	// but then abruptly ends; should not return error, but still is
	// unusable
	preambleNoElements := make([]byte, 132)
	preambleNoElements[128] = byte('D')
	preambleNoElements[129] = byte('I')
	preambleNoElements[130] = byte('C')
	preambleNoElements[131] = byte('M')
	r = bytes.NewReader(preambleNoElements)
	_, err = FromReader(r)
	assert.NoError(t, err)

	// append one byte, this should cause subsequent element parsing
	// to fail
	preambleNoElements = append(preambleNoElements, byte(0x00))
	r = bytes.NewReader(preambleNoElements)
	_, err = FromReader(r)
	assert.Error(t, err)

	// append another byte, should not be an explicit error
	preambleNoElements = append(preambleNoElements, byte(0x00))
	r = bytes.NewReader(preambleNoElements)
	_, err = FromReader(r)
	assert.NoError(t, err)

	// append another bytes, this should cause subsequent element
	// parsing to fail
	preambleNoElements = append(preambleNoElements, byte(0x00))
	r = bytes.NewReader(preambleNoElements)
	_, err = FromReader(r)
	assert.Error(t, err)

	// append another couple bytes, this also should cause subsequent element
	// parsing to fail
	preambleNoElements = append(preambleNoElements, make([]byte, 2)...)
	r = bytes.NewReader(preambleNoElements)
	_, err = FromReader(r)
	assert.Error(t, err)
}

func TestFromFile(t *testing.T) {
	t.Parallel()
	dcm, err := FromFile(filepath.Join("testdata", "synthetic", "VRTest.dcm"))
	assert.NoError(t, err)
	assert.Equal(t, 27, dcm.Len())
}

func TestFromFileError(t *testing.T) {
	t.Parallel()
	// try to parse dicom from
	// 1: file that does not exist
	// 2: file that exists, but is not a dicom
	for _, path := range []string{"__.__0000", "dicom_test.go"} {
		_, err := FromFile(path)
		assert.Error(t, err)
	}
}

func TestFromFileNoPermission(t *testing.T) {
	// try to parse a dicom from a file that the user has no read
	// permissions
	if runtime.GOOS == "windows" {
		t.Skipf("skip (windows)")
	}
	t.Parallel()
	f, err := ioutil.TempFile("", "")
	assert.NoError(t, err)
	assert.NoError(t, f.Chmod(0333))
	defer os.Remove(f.Name())
	_, err = FromFile(f.Name())
	assert.Error(t, err)
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
			filename:             "GB18030.dcm",
			expectedCharacterSet: "GB18030",
			expectedPatientName:  "编码值",
		},
	} {
		dcm, err := FromFile(filepath.Join("testdata", "synthetic", testCase.filename))
		assert.NoError(t, err)
		assert.Equal(t, testCase.expectedCharacterSet, dcm.GetCharacterSet().Name)
		name := ""
		var e = NewElement()
		assert.True(t, dcm.GetElement(0x00100010, &e))
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
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FromReader(r)
		r.Reset(buf)
	}
}
