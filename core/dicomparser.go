package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"os"

	"github.com/b71729/opendcm/dictionary"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

type TransferSyntax struct {
	UIDEntry *dictionary.UIDEntry
	Encoding *Encoding
}

// https://nathanleclaire.com/blog/2014/08/09/dont-get-bitten-by-pointer-vs-non-pointer-method-receivers-in-golang/
func (ts *TransferSyntax) SetFromUID(uidstr string) error {
	log.Printf("SetFromUID: %s", uidstr)
	uidptr, err := LookupUID(uidstr)
	if err != nil {
		return err
	}
	ts.UIDEntry = uidptr
	ts.Encoding = GetEncodingForTransferSyntax(*ts)
	log.Printf("Switched! to Reader Transfer Syntax UID: %v", uidptr)
	log.Printf("Encoding: %v", ts.Encoding)
	return nil
}

// Encoding represents the expected encoding of dicom attributes. See TransferSyntaxToEncodingMap.
type Encoding struct {
	ImplicitVR   bool
	LittleEndian bool
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

// DicomFileReader provides an abstraction layer around a `byees.Reader` to facilitate easier parsing.
type DicomFileReader struct {
	_reader        *bytes.Reader
	FilePath       string
	Position       int64
	TransferSyntax TransferSyntax
}

func NewDicomFileReader(path string) (DicomFileReader, error) {
	reader := DicomFileReader{Position: 0, TransferSyntax: TransferSyntax{}, FilePath: path}
	uid, _ := LookupUID("1.2.840.10008.1.2.1")
	reader.TransferSyntax.UIDEntry = uid
	reader.TransferSyntax.Encoding = GetEncodingForTransferSyntax(reader.TransferSyntax)
	err := reader.BufferFromFile(1024)
	if err != nil {
		return reader, err
	}
	return reader, nil
}

// ReadElement yields an `Element` from the active buffer, and an `error` if something went wrong.
func (dr DicomFileReader) ReadElement() (Element, error) {
	element := Element{}
	lower, err := dr.readUint16()
	if err != nil {
		return element, err
	}
	upper, err := dr.readUint16()
	if err != nil {
		return element, err
	}
	tagUint32 := (uint32(lower) << 16) | uint32(upper)
	tag, _ := LookupTag(tagUint32)
	element.DictEntry = tag
	if !dr.TransferSyntax.Encoding.ImplicitVR { // if explicit, read VR from buffer
		if element.VR == "UN" { // but only if we dont already have VR from dictionary (more reliable)
			VRbytes, err := dr.readBytes(2)
			if err != nil {
				return element, err
			}
			element.VR = string(VRbytes)
		} else { // else just skip two bytes as we are using dictionary value
			err := dr.skipBytes(2)
			if err != nil {
				return element, err
			}
		}
		if element.VR == "OB" || element.VR == "OW" || element.VR == "SQ" || element.VR == "UN" || element.VR == "UT" {
			// these VRs, in explicit VR mode, have two reserved bytes following VR definition
			err := dr.skipBytes(2)
			if err != nil {
				return element, err
			}
			element.ValueLength, err = dr.readUint32()
			if err != nil {
				return element, err
			}
		} else {
			length, err := dr.readUint16()
			if err != nil {
				return element, err
			}
			element.ValueLength = uint32(length)
		}
	} else {
		// implicit VR -- all VR length definitions are 32 bits
		element.ValueLength, err = dr.readUint32()
		if err != nil {
			return element, err
		}
	}
	valuebuf := make([]byte, element.ValueLength)
	// TODO: string padding -- should remove trailing 0x00 / 0x20 bytes (see: http://dicom.nema.org/dicom/2013/output/chtml/part05/sect_6.2.html)
	err = dr.read(valuebuf)
	if err != nil {
		return element, err
	}
	padchar := byte(0xFF)
	switch element.VR {
	case "UI", "OB":
		padchar = 0x00
	case "AE", "AS", "CS", "DA", "DS", "DT", "IS", "LO", "LT", "OD", "OF", "OW", "PN", "SH", "ST", "TM", "UT":
		padchar = 0x20
	}
	if padchar != 0xFF && valuebuf[len(valuebuf)-1] == padchar {
		valuebuf = valuebuf[:len(valuebuf)-1]
	}
	element.value = bytes.NewBuffer(valuebuf)

	return element, nil
}

func (dr DicomFileReader) skipBytes(num int64) error {
	nseek, err := dr._reader.Seek(num, os.SEEK_CUR)
	if nseek < num || err != nil {
		return io.EOF
	}
	return nil
}

func (dr DicomFileReader) getPosition() int64 {
	pos, _ := dr._reader.Seek(0, io.SeekCurrent)
	return pos
}

func (dr DicomFileReader) read(v interface{}) error {
	if dr.TransferSyntax.Encoding.LittleEndian {
		return binary.Read(dr._reader, binary.LittleEndian, v)
	}
	return binary.Read(dr._reader, binary.BigEndian, v)
}

func (dr DicomFileReader) readUint16() (uint16, error) {
	buf := make([]byte, 2)
	nread, err := dr._reader.Read(buf)
	if nread != 2 || err != nil {
		return 0, io.EOF
	}
	if dr.TransferSyntax.Encoding.LittleEndian {
		return binary.LittleEndian.Uint16(buf), nil
	}
	return binary.BigEndian.Uint16(buf), nil
}

func (dr DicomFileReader) readUint32() (uint32, error) {
	buf := make([]byte, 4)
	nread, err := dr._reader.Read(buf)
	if nread != 4 || err != nil {
		return 0, io.EOF
	}
	if dr.TransferSyntax.Encoding.LittleEndian {
		return binary.LittleEndian.Uint32(buf), nil
	}
	return binary.BigEndian.Uint32(buf), nil
}

func (fb DicomFileReader) readBytes(num int) ([]byte, error) {
	buf := make([]byte, num)
	nread, err := fb._reader.Read(buf)
	if nread != num || err != nil {
		return buf, io.EOF
	}
	return buf, nil
}

func (fb *DicomFileReader) BufferFromFile(nbytes int) error {
	f, err := os.Open(fb.FilePath)
	check(err)
	defer f.Close()
	stat, err := f.Stat()
	check(err)
	var bufferSize int64
	if nbytes == -1 {
		bufferSize = stat.Size()
	} else {
		bufferSize = int64(nbytes)
	}
	buffer := make([]byte, bufferSize)
	nread, err := f.Read(buffer)
	if int64(nread) < bufferSize || err != nil {
		return err
	}
	r := bytes.NewReader(buffer)
	fb._reader = r
	return nil
}

func (df DicomFile) CrawlMeta() error {
	df.Reader._reader.Seek(0, io.SeekStart)
	err := df.Reader.BufferFromFile(1024)
	if err != nil {
		return err
	}

	preamble, err := df.Reader.readBytes(128)
	if err != nil {
		return err
	}
	copy(df.Meta.Preamble[:], preamble)
	dicmTestString, err := df.Reader.readBytes(4)
	if err != nil {
		return err
	}
	if string(dicmTestString) != "DICM" {
		return errors.New("Not a dicom file")
	}

	metaLengthElement, err := df.Reader.ReadElement()
	check(err)
	totalBytesMeta := df.Reader.getPosition() + int64(metaLengthElement.Value().(uint32))
	for {
		element, err := df.Reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return err
			}
		}
		df.Elements[uint32(element.Tag)] = element

		if df.Reader.getPosition() >= totalBytesMeta {
			break
		}
	}

	return nil
}

func (df DicomFile) CrawlElements() error {
	err := df.Reader.BufferFromFile(-1)
	if err != nil {
		return err
	}
	// change transfer syntax if necessary
	transfersyntaxuid, ok := df.GetElement(0x0020010)
	if ok {
		s := transfersyntaxuid.Value().(string)
		df.Reader.TransferSyntax = TransferSyntax{}
		err := df.Reader.TransferSyntax.SetFromUID(s)
		if err != nil {
			log.Fatalln(err)
			return err
		}
	}

	// now parse rest of elements:
	fileStat, _ := os.Stat(df.filepath)
	fileSize := fileStat.Size()
	for {
		element, err := df.Reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return err
			}
		}
		df.Elements[uint32(element.Tag)] = element

		if df.Reader.getPosition() >= fileSize {
			break
		}
	}

	return nil
}

func ParseDicom(path string) (DicomFile, error) {
	dcm := DicomFile{}
	dcm.filepath = path
	dcm.Elements = make(map[uint32]Element)
	dr, err := NewDicomFileReader(path)
	if err != nil {
		return dcm, err
	}
	dcm.Reader = dr

	// parse header:
	if err := dcm.CrawlMeta(); err != nil {
		return dcm, err
	}
	if err = dcm.CrawlElements(); err != nil {
		return dcm, err
	}

	return dcm, nil
}
