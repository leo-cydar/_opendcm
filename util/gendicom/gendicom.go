// 2>/dev/null;/usr/bin/env go run $0 $@; exit $?
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/b71729/opendcm/common"
	"github.com/b71729/opendcm/dicom"
)

var console = common.NewConsoleLogger(os.Stdout)

//TODO: move to common
func tagStringToTagUint32(tag string) (uint32, error) {
	tagString := strings.Replace(tag, ",", "", 1)
	tagInt, err := strconv.ParseUint(tagString, 16, 32)
	return uint32(tagInt), err
}

func check(err error) {
	if err != nil {
		console.Fatalf("error: %v", err)
	}
}

func generateElement(tagString string, value []byte, VR string) ([]byte, error) {
	return generateElementWithLength(tagString, value, VR, uint32(len(value)))
}

// NOTE: Explicit VR, Little Endian
func generateElementWithLength(tagString string, value []byte, VR string, length uint32) ([]byte, error) {
	ret := make([]byte, 4)
	tag, err := tagStringToTagUint32(tagString)
	if err != nil {
		return ret, nil
	}
	binary.LittleEndian.PutUint16(ret[0:], uint16(tag>>16))
	binary.LittleEndian.PutUint16(ret[2:], uint16(tag))
	ret = append(ret, []byte(VR)...)

	if length > 0 {
		// deal with padding
		switch VR {
		case "UI", "OB", "CS", "DS", "IS", "AE", "AS", "DA", "DT", "LO", "LT", "OD", "OF", "OW", "PN", "SH", "ST", "TM", "UT":
			if length%2 != 0 {
				value = append(value, 0x00)
				length++
			}
		}
	}

	switch VR {
	case "OB", "OW", "SQ", "UN", "UT":
		if length > 0xFFFFFFFF {
			return nil, errors.New("value length would overflow uint32")
		}
		// write length
		ret = append(ret, make([]byte, 2)...) // skip two bytes
		ret = append(ret, make([]byte, 4)...)
		binary.LittleEndian.PutUint32(ret[len(ret)-4:], length)
	default:
		if length > 0xFFFF {
			return nil, errors.New("value length would overflow uint16")
		}
		// write length
		ret = append(ret, make([]byte, 2)...)
		binary.LittleEndian.PutUint16(ret[len(ret)-2:], uint16(length))
	}
	if length > 0 {
		ret = append(ret, value...)
	}
	//console.Debugf("% 0x", ret)
	return ret, nil
}

// TODO: move to common
func elementFromBuffer(buf []byte) (dicom.Element, error) {
	r := bufio.NewReader(bytes.NewReader(buf))
	es := dicom.NewElementStream(r, int64(len(buf)))
	return es.GetElement()
}

func writeMeta() []byte {
	buffer := make([]byte, 128)
	buffer = append(buffer, []byte("DICM")...)

	// 0002,0001 File Meta Version
	elementBytes, err := generateElement("0002,0001", []byte{0x00, 0x01}, "OB")
	check(err)
	buffer = append(buffer, elementBytes...)

	// 0002,0002 Media Storage SOP Class UID
	// Use 1.2.840.10008.5.1.4.1.1.66 (Raw Data Storage), but may need to be adjusted.
	elementBytes, err = generateElement("0002,0002", []byte("1.2.840.10008.5.1.4.1.1.66"), "UI")
	check(err)
	buffer = append(buffer, elementBytes...)

	// 0002,0003 Media Storage SOP Instance UID
	randUID, err := common.NewRandInstanceUID()
	check(err)
	elementBytes, err = generateElement("0002,0003", []byte(randUID), "UI")
	check(err)
	buffer = append(buffer, elementBytes...)

	// 0002,0010 Transfer Syntax UID
	elementBytes, err = generateElement("0002,0010", []byte("1.2.840.10008.1.2.1"), "UI")
	check(err)
	buffer = append(buffer, elementBytes...)

	// 0002,0012 Implementation Class UID
	elementBytes, err = generateElement("0002,0012", []byte(common.GetImplementationUID(true)), "UI")
	check(err)
	buffer = append(buffer, elementBytes...)

	// (0002,0013)	Implementation Version Name	opendcm-0.1
	elementBytes, err = generateElement("0002,0013", []byte(fmt.Sprintf("opendcm-%s", common.OpenDCMVersion)), "SH")
	check(err)
	buffer = append(buffer, elementBytes...)

	// Now return to File Meta Length and populate
	val := make([]byte, 4)
	binary.LittleEndian.PutUint32(val, uint32(len(buffer)-132))
	elementBytes, err = generateElement("0002,0000", val, "UL")
	check(err)
	buffer = append(buffer[:132], append(elementBytes, buffer[132:]...)...)

	return buffer
}

func main() {
	if len(os.Args) != 2 {
		console.Fatalf("usage: %s out_file", filepath.Base(os.Args[0]))
	}
	outFileName := os.Args[1]
	if _, err := os.Stat(outFileName); err == nil {
		console.Fatalf("error: %s already exists", outFileName)
	}

	buffer := writeMeta()

	// write output
	f, err := os.Create(outFileName)
	check(err)
	nwrite, err := f.Write(buffer)
	check(err)
	if nwrite != len(buffer) {
		console.Fatalf("nwrite = %d (!= %d)", nwrite, len(buffer))
	}

	console.Infof("wrote meta information ok")

	elementBuffer := make([]byte, 0)

	// Create overflow element length (past buffer boundary)
	elementBytes, err := generateElementWithLength("0008,0005", []byte(""), "CS", 0xFF)
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	nwrite, err = f.Write(elementBuffer)
	check(err)
	if nwrite != len(elementBuffer) {
		console.Fatalf("nwrite = %d (!= %d)", nwrite, len(elementBuffer))
	}

	console.Infof("wrote elements ok")

	defer f.Close()
}
