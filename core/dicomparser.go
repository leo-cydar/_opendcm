package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
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
	buf, err := readBytes(r, 4)
	if err != nil {
		return element, err
	}
	lower := binary.LittleEndian.Uint16(buf[:2])
	upper := binary.LittleEndian.Uint16(buf[2:])
	tagUint32 := (uint32(lower) << 16) | uint32(upper)
	tag, _ := LookupTag(tagUint32)
	element.DictEntry = tag

	if element.VR == "UN" /* && explicit VR */ {
		VRbytes, err := readBytes(r, 2)
		if err != nil {
			return element, err
		}
		element.VR = string(VRbytes)
	} else {
		// if explicit VR only, skip two bytes:
		// TODO: Check Transfer syntax
		err := skipBytes(r, 2)
		if err != nil {
			return element, err
		}
	}
	if element.VR == "OB" || element.VR == "OW" || element.VR == "SQ" || element.VR == "UN" || element.VR == "UT" {
		err := skipBytes(r, 2)
		if err != nil {
			return element, err
		}
		element.ValueLength, err = readUint32(r)
		if err != nil {
			return element, err
		}
	} else {
		length, err := readUint16(r)
		if err != nil {
			return element, err
		}
		element.ValueLength = uint32(length)
	}
	valuebuf, err := readBytes(r, int(element.ValueLength))
	if err != nil {
		return element, err
	}
	element.value = bytes.NewBuffer(valuebuf)
	return element, nil
}

func skipBytes(r *bytes.Reader, num int64) error {
	nseek, err := r.Seek(num, os.SEEK_CUR)
	if nseek < num || err != nil {
		return io.EOF
	}
	return nil
}

func getPosition(r *bytes.Reader) int64 {
	pos, _ := r.Seek(0, io.SeekCurrent)
	return pos
}

func readUint16(r *bytes.Reader) (uint16, error) {
	buf := make([]byte, 2)
	nread, err := r.Read(buf)
	if nread != 2 || err != nil {
		return 0, io.EOF
	}
	return binary.LittleEndian.Uint16(buf), nil
}

func readUint32(r *bytes.Reader) (uint32, error) {
	buf := make([]byte, 4)
	nread, err := r.Read(buf)
	if nread != 4 || err != nil {
		return 0, io.EOF
	}
	return binary.LittleEndian.Uint32(buf), nil
}

func readBytes(r *bytes.Reader, num int) ([]byte, error) {
	buf := make([]byte, num)
	nread, err := r.Read(buf)
	if nread != num || err != nil {
		return buf, io.EOF
	}
	return buf, nil
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

	bufferSize := int64(1024)
	fileSize := stat.Size()
	if fileSize < 1024 {
		bufferSize = fileSize
	}
	buffer := make([]byte, bufferSize)
	f.Read(buffer)
	r := bytes.NewReader(buffer)
	preamble, err := readBytes(r, 128)
	if err != nil {
		return dcm, err
	}
	copy(dcm.Meta.Preamble[:], preamble)
	dicmTestString, err := readBytes(r, 4)
	if err != nil {
		return dcm, err
	}
	if string(dicmTestString) != "DICM" {
		return dcm, errors.New("Not a dicom file")
	}

	// parse header:

	metaLengthElement, err := ReadElement(r)
	check(err)
	log.Printf("[%s] %s = %v", metaLengthElement.VR, metaLengthElement.Name, metaLengthElement.Value())
	totalBytesMeta := getPosition(r) + int64(metaLengthElement.Value().(uint32))
	for {
		element, err := ReadElement(r)
		if err != nil {
			return dcm, err
		}
		dcm.Meta.Elements = append(dcm.Meta.Elements, element)
		if getPosition(r) >= totalBytesMeta {
			break
		}
	}

	return dcm, nil
}
