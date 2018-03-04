// Package file implements functionality to parse dicom files
package file

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// UnsupportedDicom is an error representing that the `Dicom` is unsupported
type UnsupportedDicom struct {
	error
}

// NotADicom is an error representing that the input is not recognised as a valid dicom
type NotADicom struct {
	error
}

// CorruptDicom is an error representing that a `Dicom` is corrupt
type CorruptDicom struct {
	error
}

// CorruptElement is an error representing that an `Element` is corrupt
type CorruptElement struct {
	error
}

// ElementStreamError is an error representing that the `ElementStream` encountered a general problem
type ElementStreamError struct {
	error
}

// VRSpecification represents a specification for VR, according to NEMA specs.
type VRSpecification struct {
	VR                 string
	MaximumLengthBytes uint32
	FixedLength        bool
	CharsetRe          *regexp.Regexp
}

func checkTransferSyntaxSupport(tsuid string) bool {
	switch tsuid {
	case "1.2.840.10008.1.2", // Implicit VR Little Endian: Default Transfer Syntax for DICOM
		"1.2.840.10008.1.2.1",    // Explicit VR Little Endian,
		"1.2.840.10008.1.2.2",    // Explicit VR Big Endian (Retired)
		"1.2.840.10008.1.2.4.91", // JPEG 2000 Image Compression
		"1.2.840.10008.1.2.4.70": // Default Transfer Syntax for Lossless JPEG Image Compression
		return true
	default:
		return false
	}
}

// ElementStream provides an abstraction layer around a `*bytes.Reader` to facilitate easier parsing.
type ElementStream struct {
	reader         *bytes.Reader
	fileOffset     int64
	TransferSyntax TransferSyntax
	CharacterSet   *CharacterSet
}

// GetElement yields an `Element` from the active stream, and an `error` if something went wrong.
func (elementStream *ElementStream) GetElement() (Element, error) {
	element := Element{}
	element.FileOffsetStart = elementStream.getPosition() + elementStream.fileOffset
	element.sourceElementStream = elementStream
	lower, err := elementStream.getUint16()
	if err != nil {
		return element, &CorruptElement{fmt.Errorf("GetElement(): %v", err)}
	}
	upper, err := elementStream.getUint16()
	if err != nil {
		return element, &CorruptElement{fmt.Errorf("GetElement(): %v", err)}
	}
	tagUint32 := (uint32(lower) << 16) | uint32(upper)
	tag, _ := LookupTag(tagUint32)
	element.DictEntry = tag
	if !elementStream.TransferSyntax.Encoding.ImplicitVR { // if explicit, read VR from buffer
		if element.VR == "UN" { // but only if we dont already have VR from dictionary (more reliable)
			VRbytes, err := elementStream.getBytes(2)
			if err != nil {
				return element, &CorruptElement{fmt.Errorf("GetElement(): [%s] %v", tag.Tag, err)}
			}
			element.VR = string(VRbytes)
		} else { // else just skip two bytes as we are using dictionary value
			err := elementStream.skipBytes(2)
			if err != nil {
				return element, &CorruptElement{fmt.Errorf("GetElement(): [%s] %v", tag.Tag, err)}
			}
		}
		if element.VR == "OB" || element.VR == "OW" || element.VR == "SQ" || element.VR == "UN" || element.VR == "UT" {
			// these VRs, in explicit VR mode, have two reserved bytes following VR definition
			err := elementStream.skipBytes(2)
			if err != nil {
				return element, &CorruptElement{fmt.Errorf("GetElement(): [%s] %v", tag.Tag, err)}
			}
			element.ValueLength, err = elementStream.getUint32()
			if err != nil {
				return element, &CorruptElement{fmt.Errorf("GetElement(): [%s] %v", tag.Tag, err)}
			}
		} else {
			length, err := elementStream.getUint16()
			if err != nil {
				return element, &CorruptElement{fmt.Errorf("GetElement(): [%s] %v", tag.Tag, err)}
			}
			element.ValueLength = uint32(length)
		}
	} else {
		// implicit VR -- all VR length definitions are 32 bits
		element.ValueLength, err = elementStream.getUint32()
		if err != nil {
			return element, &CorruptElement{fmt.Errorf("GetElement(): [%s] %v", tag.Tag, err)}
		}
	}
	// issue #4: Parser allows for element value length to exceed file size
	if int(element.ValueLength) > elementStream.reader.Len() {
		return element, &CorruptElement{fmt.Errorf("GetElement(): value length (%d) exceeds buffer size (%d)", element.ValueLength, elementStream.reader.Len())}
	}
	if element.ValueLength == 0xFFFFFFFF {
		var parseElements = (element.VR == "SQ")
		items, err := elementStream.getSequence(parseElements)
		if err != nil {
			return element, &CorruptElement{fmt.Errorf("GetElement(): [%s] %v", tag.Tag, err)}
		}
		element.Items = items
	} else {
		valuebuf := make([]byte, element.ValueLength)
		// string padding: should remove trailing+leading 0x00 / 0x20 bytes (see: http://dicom.nema.org/dicom/2013/output/chtml/part05/sect_6.2.html)
		// NOTE: some vendors pad with 0x20, some 0x00 -- seems to contradict NEMA spec. Let's account for both then:
		if element.ValueLength > 0 {
			err = elementStream.read(valuebuf)
			if err != nil {
				return element, &CorruptElement{fmt.Errorf("GetElement(): [%s] %v", tag.Tag, err)}
			}
			padchars := []byte{0x00, 0x20}
			if element.ValueLength > 1 { // cannot strip padding characters if it would leave the bytestream with length of 0
				switch element.VR {
				case "UI", "OB", "CS", "DS", "IS", "AE", "AS", "DA", "DT", "LO", "LT", "OD", "OF", "OW", "PN", "SH", "ST", "TM", "UT":
					for _, chr := range padchars {
						if valuebuf[len(valuebuf)-1] == chr {
							valuebuf = valuebuf[:len(valuebuf)-1]
							element.ValueLength--
						} else if valuebuf[0] == chr { // NOTE: assumes padding will only take place on one side. Should be fine.
							valuebuf = valuebuf[1:]
							element.ValueLength--
						}
					}
				}
			}
		}
		element.value = bytes.NewBuffer(valuebuf)
	}

	element.ByteLengthTotal = (elementStream.getPosition() + elementStream.fileOffset) - element.FileOffsetStart
	return element, nil
}

func (elementStream *ElementStream) getUntil(delimiter []byte) ([]byte, error) {
	if len(delimiter) > 8 {
		panic("does not support delimiters with length greater than 8 bytes")
	}
	var buf []byte
	for {
		currentBuffer, err := elementStream.getBytes(136) // 128 bytes plus maximum 8 bytes for delimiter boundary
		if err != nil {
			return buf, &ElementStreamError{fmt.Errorf("getUntil(%v): %v", delimiter, err)}
		}
		delimiterPos := bytes.Index(currentBuffer, delimiter)
		if delimiterPos >= 0 { // found
			buf = append(buf[:], currentBuffer[:delimiterPos]...)
			_, err = elementStream.reader.Seek(-int64(136-delimiterPos), io.SeekCurrent)
			return buf, nil
		}
		buf = append(buf[:], currentBuffer[:128]...)
		_, err = elementStream.reader.Seek(-8, io.SeekCurrent)
		if err != nil {
			return buf, &ElementStreamError{fmt.Errorf("getUntil(%v): %v", delimiter, err)}
		}
	}
}

// getSequence parses a sequence of "unlimited length" from the bytestream
func (elementStream *ElementStream) getSequence(parseElements bool) ([]Item, error) {
	var items []Item
	for {
		lower, err := elementStream.getUint16()
		if err != nil {
			return items, &ElementStreamError{fmt.Errorf("getSequence(%v): %v", parseElements, err)}
		}
		upper, err := elementStream.getUint16()
		if err != nil {
			return items, &ElementStreamError{fmt.Errorf("getSequence(%v): %v", parseElements, err)}
		}
		tagUint32 := (uint32(lower) << 16) | uint32(upper)
		if tagUint32 == 0xFFFEE0DD {
			err := elementStream.skipBytes(4)
			if err != nil {
				return items, &ElementStreamError{fmt.Errorf("getSequence(%v): %v", parseElements, err)}
			}
			break
		}
		if tagUint32 != 0xFFFEE000 {
			return items, &ElementStreamError{fmt.Errorf("getSequence(%v): 0x%08X != 0xFFFEE000", parseElements, tagUint32)}
		}
		length, err := elementStream.getUint32()
		if err != nil {
			return items, &ElementStreamError{fmt.Errorf("getSequence(%v): %v", parseElements, err)}
		}

		var elements = make(map[uint32]Element)
		var unknownBuffers [][]byte
		if length == 0xFFFFFFFF { // unlimited length item
			// find next FFFE, E00D = data for item ends
			var delimitationItemBytes []byte
			if elementStream.TransferSyntax.Encoding.LittleEndian {
				delimitationItemBytes = []byte{0xFE, 0xFF, 0x0D, 0xE0}
			} else {
				delimitationItemBytes = []byte{0xFF, 0xFE, 0xE0, 0x0D}
			}

			for {
				// try to grab an element according to current TransferSyntax
				e, err := elementStream.GetElement()
				if err != nil {
					return items, &CorruptElement{fmt.Errorf("getSequence(%v): %v", parseElements, err)}
				}
				elements[uint32(e.Tag)] = e
				check, err := elementStream.getBytes(4)
				if err != nil {
					return items, &ElementStreamError{fmt.Errorf("getSequence(%v): %v", parseElements, err)}
				}
				if bytes.Compare(check, delimitationItemBytes) == 0 {
					// end
					break
				}
				elementStream.reader.Seek(-4, io.SeekCurrent)
			}

			// now we must skip four bytes (0x00{4}) (see: NEMA Table 7.5-3)
			err = elementStream.skipBytes(4)
			if err != nil {
				return items, &ElementStreamError{fmt.Errorf("getSequence(%v): %v", parseElements, err)}
			}
		} else {
			// try to grab an element according to current TransferSyntax
			if !parseElements {
				valuebuffer, err := elementStream.getBytes(uint(length))
				if err != nil {
					return items, &ElementStreamError{fmt.Errorf("getSequence(%v): %v", parseElements, err)}
				}
				unknownBuffers = append(unknownBuffers, valuebuffer)
			} else {
				if length == 0 {
					continue
					/* Turns out the data set had bytes:
					(40 00 08 00) (53 51)  00 00 (FF FF  FF FF) (FE FF  00 E0) (00 00  00 00) (FE FF  DD E0) 00 00
					(4b: tag)     (2b:SQ)        (4b: un.len)   (4b:itm start) (4b: 0 len)    (4b: seq end)
					Therefore, the item genuinely had length of zero.
					This condition accounts for this possibility.
					*/
				}
				element, err := elementStream.GetElement()
				if err != nil {
					return items, &CorruptElement{fmt.Errorf("getSequence(%v): %v", parseElements, err)}
				}
				elements[uint32(element.Tag)] = element
			}
		}
		item := Item{Elements: elements, UnknownSections: unknownBuffers}
		items = append(items, item)
	}

	return items, nil
}

func (elementStream *ElementStream) skipBytes(num int64) error {
	nseek, err := elementStream.reader.Seek(num, os.SEEK_CUR)
	if nseek < num {
		return &ElementStreamError{fmt.Errorf("skipBytes(%d): nseek = %d", num, nseek)}
	}
	if err != nil {
		return &ElementStreamError{fmt.Errorf("skipBytes(%d): %v", num, err)}
	}
	return nil
}

func (elementStream *ElementStream) getPosition() int64 {
	return elementStream.reader.Size() - int64(elementStream.reader.Len())
}

func (elementStream *ElementStream) read(v interface{}) error {
	var err error
	if elementStream.TransferSyntax.Encoding.LittleEndian {
		err = binary.Read(elementStream.reader, binary.LittleEndian, v)
	} else {
		err = binary.Read(elementStream.reader, binary.BigEndian, v)
	}
	if err != nil {
		return &ElementStreamError{fmt.Errorf("read(...): %v", err)}
	}
	return nil
}

func (elementStream *ElementStream) getUint16() (uint16, error) {
	buf := make([]byte, 2)
	nread, err := elementStream.reader.Read(buf)
	if nread != 2 {
		return 0, &ElementStreamError{fmt.Errorf("getUint16(): nread = %d (!= 2)", nread)}
	}
	if err != nil {
		return 0, &ElementStreamError{fmt.Errorf("getUint16(): %v", err)}
	}
	if elementStream.TransferSyntax.Encoding.LittleEndian {
		return binary.LittleEndian.Uint16(buf), nil
	}
	return binary.BigEndian.Uint16(buf), nil
}

func (elementStream *ElementStream) getUint32() (uint32, error) {
	buf := make([]byte, 4)
	nread, err := elementStream.reader.Read(buf)
	if nread != 4 {
		return 0, &ElementStreamError{fmt.Errorf("getUint32(): nread = %d (!= 4)", nread)}
	}
	if err != nil {
		return 0, &ElementStreamError{fmt.Errorf("getUint32(): %v", err)}
	}
	if elementStream.TransferSyntax.Encoding.LittleEndian {
		return binary.LittleEndian.Uint32(buf), nil
	}
	return binary.BigEndian.Uint32(buf), nil
}

func (elementStream *ElementStream) getBytes(num uint) ([]byte, error) {
	if int(num) > elementStream.reader.Len() {
		return nil, &ElementStreamError{fmt.Errorf("getBytes(%d): would exceed buffer size (%d bytes)", num, elementStream.reader.Len())}
	}
	buf := make([]byte, num)
	nread, err := elementStream.reader.Read(buf)
	if uint(nread) != num {
		return buf, &ElementStreamError{fmt.Errorf("getBytes(%d): nread = %d (!= %d)", num, nread, num)}
	}
	if err != nil {
		return buf, &ElementStreamError{fmt.Errorf("getBytes(%d): %v", num, err)}
	}
	return buf, nil
}

// NewElementStream sets up a new `ElementStream`
func NewElementStream(source []byte, transferSyntaxUID string, fileOffset int64) (ElementStream, error) {
	stream := ElementStream{TransferSyntax: TransferSyntax{}}
	stream.TransferSyntax.SetFromUID(transferSyntaxUID)
	stream.TransferSyntax.Encoding = GetEncodingForTransferSyntax(stream.TransferSyntax)
	stream.CharacterSet = CharacterSetMap["Default"]
	stream.reader = bytes.NewReader(source)
	stream.fileOffset = fileOffset
	return stream, nil
}

// LoadBytes loads `nbytes` from its FilePath, starting at offset `nstart`, possibly accepting a partial read.
func (df *Dicom) loadBytes(nstart int64, nbytes int, acceptPartialRead bool) ([]byte, error) {
	var buffer []byte
	var bufferSize int64

	if df.ByteSource != nil {
		if len(df.ByteSource) < 132 {
			return buffer, &NotADicom{}
		}
		if nstart == -1 {
			nstart = 0
		}
		if nstart == 0 && nbytes == -1 {
			return df.ByteSource, nil // simple, just return all bytes
		}
		if nbytes == -1 { // requesting all remaining bytes
			if int(nstart) > len(df.ByteSource) {
				return nil, &CorruptDicom{fmt.Errorf("loadBytes(%d, %d, %v): %d > input stream length", nstart, nbytes, acceptPartialRead, nstart)}
			}
			return df.ByteSource[nstart:], nil
		}

		if !acceptPartialRead && int(nstart)+nbytes > len(df.ByteSource) {
			return nil, &CorruptDicom{fmt.Errorf("loadBytes(%d, %d, %v): input stream length < %d", nstart, nbytes, acceptPartialRead, int(nstart)+nbytes)}
		}
		if !acceptPartialRead && int(nstart) > len(df.ByteSource) {
			return nil, &CorruptDicom{fmt.Errorf("loadBytes(%d, %d, %v): %d > input stream length", nstart, nbytes, acceptPartialRead, nstart)} // TODO: Can this be removed?
		}
		if int(nstart)+nbytes > len(df.ByteSource) {
			return df.ByteSource[nstart:], nil
		}
		return df.ByteSource[nstart:nbytes], nil
	}

	f, err := os.Open(df.FilePath)
	if err != nil {
		return buffer, &CorruptDicom{fmt.Errorf("loadBytes(%d, %d, %v): %v", nstart, nbytes, acceptPartialRead, err)}
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return buffer, &CorruptDicom{fmt.Errorf("loadBytes(%d, %d, %v): %v", nstart, nbytes, acceptPartialRead, err)}
	}
	if stat.Size() < 132 {
		return buffer, &NotADicom{}
	}

	if nbytes == -1 {
		bufferSize = stat.Size()
	} else {
		bufferSize = int64(nbytes)
	}
	if nstart > -1 {
		bufferSize -= nstart
	}
	buffer = make([]byte, bufferSize)

	if nstart > -1 {
		nseek, err := f.Seek(nstart, io.SeekStart)
		if nseek < nstart {
			return buffer, &CorruptDicom{fmt.Errorf("loadBytes(%d, %d, %v): nseek = %d, wanted %d", nstart, nbytes, acceptPartialRead, nseek, nstart)}
		}
		if err != nil {
			return buffer, &CorruptDicom{fmt.Errorf("loadBytes(%d, %d, %v): %v", nstart, nbytes, acceptPartialRead, err)}
		}
	}
	nread, err := f.Read(buffer)
	if err != nil {
		return buffer, &CorruptDicom{fmt.Errorf("loadBytes(%d, %d, %v): %v", nstart, nbytes, acceptPartialRead, err)}
	}
	if !acceptPartialRead && (int64(nread) < bufferSize) {
		return buffer, &CorruptDicom{fmt.Errorf("loadBytes(%d, %d, %v): nread = %d, wanted %d", nstart, nbytes, acceptPartialRead, nread, bufferSize)}
	}

	return buffer, nil
}

func (df *Dicom) crawlMeta() error {
	bytes, err := df.loadBytes(-1, 1024, true)
	if err != nil {
		switch err.(type) {
		case *NotADicom:
			return err
		default:
			return &CorruptDicom{fmt.Errorf("crawlMeta(): %v", err)}
		}
	}
	df.elementStream, err = NewElementStream(bytes, "1.2.840.10008.1.2.1", 0)
	if err != nil {
		return &CorruptDicom{fmt.Errorf("crawlMeta(): %v", err)}
	}

	preamble, err := df.elementStream.getBytes(128)
	if err != nil {
		return &CorruptDicom{fmt.Errorf("crawlMeta(): %v", err)}
	}
	copy(df.Preamble[:], preamble)
	dicmTestString, err := df.elementStream.getBytes(4)
	if err != nil {
		return &CorruptDicom{fmt.Errorf("crawlMeta(): %v", err)}
	}
	if string(dicmTestString) != "DICM" {
		return &NotADicom{}
	}

	metaLengthElement, err := df.elementStream.GetElement()
	if err != nil {
		return &CorruptDicom{fmt.Errorf("crawlMeta: %v", err)}
	}
	df.Elements[uint32(metaLengthElement.Tag)] = metaLengthElement
	val := metaLengthElement.Value()
	switch val.(type) {
	case uint32:
		df.TotalMetaBytes = df.elementStream.getPosition() + int64(val.(uint32))
	default:
		return &CorruptDicom{fmt.Errorf("Meta length element is corrupt")}
	}
	for {
		element, err := df.elementStream.GetElement()
		if err != nil {
			return &CorruptDicom{fmt.Errorf("crawlMeta: %v", err)}
		}
		df.Elements[uint32(element.Tag)] = element

		if df.elementStream.getPosition() >= df.TotalMetaBytes {
			break
		}
	}

	return nil
}

func (df *Dicom) crawlElements() error {
	bytes, err := df.loadBytes(df.TotalMetaBytes, -1, false)
	if err != nil {
		return &CorruptDicom{fmt.Errorf("crawlElements(): %v", err)}
	}
	var transfersyntaxuid = "1.2.840.10008.1.2.1"
	// change transfer syntax if necessary
	tsElement, ok := df.GetElement(0x0020010)

	if ok {
		val := tsElement.Value()
		switch val.(type) {
		case string:
			transfersyntaxuid = val.(string)
			supported := checkTransferSyntaxSupport(transfersyntaxuid)
			if !supported {
				return &UnsupportedDicom{fmt.Errorf("unsupported transfer syntax: %s", transfersyntaxuid)}
			}
		default:
			return &CorruptDicom{fmt.Errorf("TransferSyntaxUID is corrupt")}
		}
	}

	df.elementStream, err = NewElementStream(bytes, transfersyntaxuid, df.TotalMetaBytes)
	if err != nil {
		return &CorruptDicom{fmt.Errorf("crawlElements(): %v", err)}
	}

	// now parse rest of elements:
	var fileSize int64
	if df.ByteSource != nil {
		fileSize = int64(len(df.ByteSource))
	} else {
		fileStat, err := os.Stat(df.FilePath)
		if err != nil {
			return &CorruptDicom{fmt.Errorf("crawlElements(): %v", err)}
		}
		fileSize = fileStat.Size()
	}
	for {
		element, err := df.elementStream.GetElement()
		if err != nil {
			return &CorruptDicom{fmt.Errorf("crawlElements(): %v", err)}
		}
		df.Elements[uint32(element.Tag)] = element

		switch element.Tag {
		case 0x00080005:
			df.elementStream.CharacterSet = CharacterSetMap[element.Value().([]string)[0]]
		}

		if df.elementStream.getPosition()+df.TotalMetaBytes >= fileSize {
			break
		}
	}
	return nil
}

// ParseDicom takes a relative/absolute path to a dicom file and returns a parsed `Dicom` [+ error]
func ParseDicom(path string) (Dicom, error) {
	dcm := Dicom{}
	dcm.FilePath = path
	dcm.Elements = make(map[uint32]Element)

	if err := dcm.crawlMeta(); err != nil {
		switch err.(type) {
		case *NotADicom:
			return dcm, &NotADicom{fmt.Errorf(`The file "%s" is not a valid dicom`, filepath.Base(path))}
		default:
			return dcm, &CorruptDicom{fmt.Errorf(`The file "%s" is corrupt: %v`, filepath.Base(path), err)}
		}

	}
	if err := dcm.crawlElements(); err != nil {
		return dcm, &CorruptDicom{fmt.Errorf(`The dicom "%s" is corrupt: %v`, filepath.Base(path), err)}
	}

	return dcm, nil
}

// ParseFromBytes parses a dicom from a bytestream
func ParseFromBytes(source []byte) (Dicom, error) {
	dcm := Dicom{}
	dcm.Elements = make(map[uint32]Element)
	dcm.ByteSource = source

	if err := dcm.crawlMeta(); err != nil {
		switch err.(type) {
		case *NotADicom:
			return dcm, &NotADicom{fmt.Errorf(`The bytes do not form a valid dicom`)}
		default:
			return dcm, &CorruptDicom{fmt.Errorf(`The bytes are corrupt: %v`, err)}
		}

	}
	if err := dcm.crawlElements(); err != nil {
		return dcm, &CorruptDicom{fmt.Errorf(`The bytes are corrupt: %v`, err)}
	}

	return dcm, nil
}

// ParseDicomChannel wraps `ParseDicom` in a channel for parsing in a goroutine
func ParseDicomChannel(path string, dicomchannel chan Dicom, errorchannel chan error, guard chan struct{}) {
	if guard != nil {
		<-guard
	}
	dcm, err := ParseDicom(path)

	if err != nil {
		errorchannel <- err
		return
	}
	dicomchannel <- dcm
}
