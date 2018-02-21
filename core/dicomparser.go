package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"log"
	"os"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func ReadElement(r *bytes.Reader) (Element, error) {
	element := Element{}
	buf := make([]byte, 4)
	r.Read(buf)
	lower := binary.LittleEndian.Uint16(buf[:2])
	upper := binary.LittleEndian.Uint16(buf[2:])
	tagUint32 := (uint32(lower) << 16) | uint32(upper)
	tag, _ := LookupTag(tagUint32)
	element.DictEntry = tag

	if element.VR == "UN" /* && explicit VR */ {
		element.VR = string(readBytes(r, 2))
	} else {
		// if explicit VR only, skip two bytes:
		// TODO: Check Transfer syntax
		r.Seek(2, os.SEEK_CUR)
	}
	if element.VR == "OB" || element.VR == "OW" || element.VR == "SQ" || element.VR == "UN" || element.VR == "UT" {
		r.Seek(2, os.SEEK_CUR)
		element.ValueLength = readUint32(r)
	} else {
		element.ValueLength = uint32(readUint16(r))
	}
	valuebuf := readBytes(r, int(element.ValueLength))
	element.value = bytes.NewBuffer(valuebuf)
	return element, nil
}

func readUint16(r *bytes.Reader) uint16 {
	buf := make([]byte, 2)
	r.Read(buf)
	return binary.LittleEndian.Uint16(buf)
}

func readUint32(r *bytes.Reader) uint32 {
	buf := make([]byte, 4)
	r.Read(buf)
	return binary.LittleEndian.Uint32(buf)
}

func readBytes(r *bytes.Reader, num int) []byte {
	buf := make([]byte, num)
	r.Read(buf)
	return buf
}

func ParseDicom(path string) (DicomFile, error) {
	dcm := DicomFile{}
	f, err := os.Open(path)
	check(err)
	defer f.Close()
	stat, err := f.Stat()
	check(err)
	if stat.Size() < int64(132) {
		return dcm, errors.New("Not a dicom file")
	}

	//reader := bufio.NewReader(f)
	bufferSize := int64(1024)
	if stat.Size() < 1024 {
		bufferSize = stat.Size()
	}
	buffer := make([]byte, bufferSize)
	f.Read(buffer)
	r := bytes.NewReader(buffer)
	copy(dcm.Meta.Preamble[:], readBytes(r, 128))
	dicmTestString := readBytes(r, 4)
	if string(dicmTestString) != "DICM" {
		return dcm, errors.New("Not a dicom file")
	}
	// 132
	metaLengthElement, err := ReadElement(r)
	totalBytesMeta := int64(132 + int(metaLengthElement.Value().(uint32)))
	for {
		position, _ := r.Seek(0, os.SEEK_CUR)
		if position > totalBytesMeta {
			break
		}
		element, err := ReadElement(r)
		check(err)
		log.Printf("[%s] %s = %v", element.VR, element.Name, element.Value())
	}

	return dcm, nil
}
