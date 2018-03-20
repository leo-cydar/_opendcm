package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	. "github.com/b71729/opendcm"
)

/*
===============================================================================
    Util: Create DICOM File
===============================================================================
*/

var baseFile = filepath.Base(os.Args[0])

func check(err error) {
	if err != nil {
		FatalfDepth(3, "error: %v", err)
	}
}

func usage() {
	fmt.Printf("OpenDCM version %s\n", OpenDCMVersion)
	fmt.Printf("usage: %s out_file\n", baseFile)
	os.Exit(1)
}

func main() {
	GetConfig()
	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		usage()
	}
	if len(os.Args) != 2 {
		usage()
	}
	outFileName := os.Args[1]
	if _, err := os.Stat(outFileName); err == nil {
		Fatalf(`file "%s" already exists`, outFileName)
	}

	buffer := writeMeta()

	// write output
	f, err := os.Create(outFileName)
	check(err)
	nwrite, err := f.Write(buffer)
	check(err)
	if nwrite != len(buffer) {
		Fatalf("could not write all meta elements to disk. nwrite=%d bytes, size=%d bytes", nwrite, len(buffer))
	}

	Info("wrote meta information to disk")

	elementBuffer := make([]byte, 0)

	/// VRs with defined length
	// AE
	elementBytes, err := generateElement("0072,005E", []byte("AENAME"), "AE")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// AS
	elementBytes, err = generateElement("0072,005F", []byte("012Y"), "AS")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// AT
	elementBytes, err = generateElement("0072,0060", []byte{0x42, 0x24, 0x01, 0x90}, "AT")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// CS
	elementBytes, err = generateElement("0072,0062", []byte("CODESTRING_1"), "CS")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// DA
	elementBytes, err = generateElement("0072,0061", []byte("20180317"), "DA")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// DS
	elementBytes, err = generateElement("0072,0072", []byte("360.8"), "DS")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// DT
	elementBytes, err = generateElement("0072,0063", []byte("200508101215"), "DT")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// FL
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, math.Float32bits(127.50812))
	elementBytes, err = generateElement("0072,0076", buf, "FL")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// FD
	buf = make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, math.Float64bits(123456.123456789))
	elementBytes, err = generateElement("0072,0074", buf, "FD")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// IS
	elementBytes, err = generateElement("0072,0064", []byte("0123456789"), "IS")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// LO
	elementBytes, err = generateElement("0072,0066", []byte(`Long String`), "LO")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// LT
	elementBytes, err = generateElement("0072,0068", []byte(`Long\Text\No\Split`), "LT")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// OB
	elementBytes, err = generateElement("0072,0065", []byte{0x01, 0x02, 0x03, 0x04}, "OB")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// OB of undefined length
	buf = genItemBytesRaw([]byte{0x01, 0x02, 0x03, 0x04}, 4)
	elementBytes, err = generateElementWithLength("7FE0,0010", buf, "OB", 0xFFFFFFFF)
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// OD
	buf = make([]byte, 16)
	binary.LittleEndian.PutUint64(buf[0:], 888888887)
	binary.LittleEndian.PutUint64(buf[8:], 777777778)
	elementBytes, err = generateElement("0072,0073", buf, "OD")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// OF
	buf = make([]byte, 8)
	binary.LittleEndian.PutUint32(buf[0:], math.Float32bits(123.4))
	binary.LittleEndian.PutUint32(buf[4:], math.Float32bits(567.8))
	elementBytes, err = generateElement("0072,0067", buf, "OF")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// OW
	buf = make([]byte, 16)
	binary.LittleEndian.PutUint32(buf[0:], 4321)
	binary.LittleEndian.PutUint32(buf[4:], 8765)
	binary.LittleEndian.PutUint32(buf[8:], 2109)
	binary.LittleEndian.PutUint32(buf[12:], 6543)
	elementBytes, err = generateElement("0072,0069", buf, "OW")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// PN
	elementBytes, err = generateElement("0072,006A", []byte(`Anderson^Leo`), "PN")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// SH
	elementBytes, err = generateElement("0072,006C", []byte(`Short String`), "SH")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// SL
	buf = make([]byte, 4)
	v := int32(-1234)
	binary.LittleEndian.PutUint32(buf[0:], uint32(v))
	elementBytes, err = generateElement("0072,007C", buf, "SL")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// SQ
	buf = make([]byte, 0)

	// SQ Encoding 5.12.1: undefined-len SQ with defined-len items
	asBytes := genItemBytes("0072,005F", []byte("012Y"), "AS", 4)
	stBytes := genItemBytes("0072,006E", []byte(`Unlimited\Text`), "UT", 14)
	buf = append(buf, asBytes...)
	buf = append(buf, stBytes...)
	elementBytes, err = generateElementWithLength("0072,0080", buf, "SQ", 0xFFFFFFFF)
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// SQ Encoding 5.12.3: undefined-len SQ with undefined-len items
	nestedAS := genItemBytes("0072,005F", []byte("012Y"), "AS", 4)
	sequenceItem := genItemBytes("0072,0080", nestedAS, "SQ", 0xFFFFFFFF)
	for i := 0; i < 4; i++ {
		sequenceItem = genItemBytes("0072,0080", sequenceItem, "SQ", 0xFFFFFFFF)
	}

	elementBytes, err = generateElementWithLength("0008,9121", sequenceItem, "SQ", 0xFFFFFFFF)
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// SS
	buf = make([]byte, 2)
	v2 := int16(-1234)
	binary.LittleEndian.PutUint16(buf[0:], uint16(v2))
	elementBytes, err = generateElement("0072,007E", buf, "SS")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// ST
	elementBytes, err = generateElement("0072,006E", []byte(`Short\Text\No\Split`), "ST")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// TM
	elementBytes, err = generateElement("0072,006B", []byte(`121530.35`), "TM")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// UI
	elementBytes, err = generateElement("0072,007F", []byte(`127.0.0.1`), "UI")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// UL
	buf = make([]byte, 4)
	v3 := uint32(123456789)
	binary.LittleEndian.PutUint32(buf[0:], v3)
	elementBytes, err = generateElement("0072,0078", buf, "UL")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// UN
	elementBytes, err = generateElement("0072,006D", []byte("UnknownData"), "UN")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// US
	buf = make([]byte, 2)
	v4 := uint16(12345)
	binary.LittleEndian.PutUint16(buf[0:], v4)
	elementBytes, err = generateElement("0072,007A", buf, "US")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	// UT
	elementBytes, err = generateElement("0072,0070", []byte(`Unlimited\Text\No\Split`), "UT")
	check(err)
	elementBuffer = append(elementBuffer, elementBytes...)

	nwrite, err = f.Write(elementBuffer)
	check(err)
	if nwrite != len(elementBuffer) {
		Fatalf("could not write all elements to disk. nwrite=%d bytes, size=%d bytes", nwrite, len(elementBuffer))
	}

	Info("wrote elements to disk")

	defer f.Close()
}

// TODO: move to common
func tagStringToTagUint32(tag string) (uint32, error) {
	tagString := strings.Replace(tag, ",", "", 1)
	tagInt, err := strconv.ParseUint(tagString, 16, 32)
	return uint32(tagInt), err
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

	if length > 0 && length < 0xFFFFFFFF {
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
	if length == 0xFFFFFFFF {
		ret = append(ret, []byte{
			0xFE, 0xFF, 0xDD, 0xE0, // 4b: sequence end tag
			0x00, 0x00, 0x00, 0x00, // 4b: filler
		}...)
	}
	return ret, nil
}

// TODO: move to common
func elementFromBuffer(buf []byte) (Element, error) {
	r := bufio.NewReader(bytes.NewReader(buf))
	es := NewElementStream(r, int64(len(buf)))
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
	randUID, err := NewRandInstanceUID()
	check(err)
	elementBytes, err = generateElement("0002,0003", []byte(randUID), "UI")
	check(err)
	buffer = append(buffer, elementBytes...)

	// 0002,0010 Transfer Syntax UID
	elementBytes, err = generateElement("0002,0010", []byte("1.2.840.10008.1.2.1"), "UI")
	check(err)
	buffer = append(buffer, elementBytes...)

	// 0002,0012 Implementation Class UID
	elementBytes, err = generateElement("0002,0012", []byte(GetImplementationUID(true)), "UI")
	check(err)
	buffer = append(buffer, elementBytes...)

	// (0002,0013)    Implementation Version Name    opendcm-0.1
	elementBytes, err = generateElement("0002,0013", []byte(fmt.Sprintf("opendcm-%s", OpenDCMVersion)), "SH")
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

func genItemBytesRaw(value []byte, length uint32) []byte {
	outBytes := []byte{0xFE, 0xFF, 0x00, 0xE0}
	outBytes = append(outBytes, make([]byte, 4)...)
	binary.LittleEndian.PutUint32(outBytes[4:], length)
	outBytes = append(outBytes, value...)
	if length == 0xFFFFFFFF {
		outBytes = append(outBytes, []byte{
			0xFE, 0xFF, 0x0D, 0xE0, // 4b: item #1 end tag
			0x00, 0x00, 0x00, 0x00, // 4b: filler
		}...)
	}
	return outBytes
}

func genItemBytes(tagString string, value []byte, VR string, length uint32) []byte {
	el, err := generateElementWithLength(tagString, value, VR, length)
	if err != nil {
		panic(err)
	}
	outBytes := genItemBytesRaw(el, length)
	return outBytes

}
