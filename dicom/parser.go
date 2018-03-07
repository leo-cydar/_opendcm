// Package dicom implements functionality to parse dicom files
package dicom

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// DicomReadBufferSize is the number of bytes to be buffered from disk when parsing dicoms
const DicomReadBufferSize = 2 * 1024 * 1024 // 10MB

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

// CorruptDicomError raises a `CorruptDicom` error
func CorruptDicomError(format string, a ...interface{}) *CorruptDicom {
	return &CorruptDicom{fmt.Errorf(format, a...)}
}

// CorruptElement is an error representing that an `Element` is corrupt
type CorruptElement struct {
	error
}

// CorruptElementError raises a `CorruptElement` error
func CorruptElementError(format string, a ...interface{}) *CorruptElement {
	return &CorruptElement{fmt.Errorf(format, a...)}
}

// CorruptElementStream is an error representing that the `ElementStream` encountered a general problem
type CorruptElementStream struct {
	error
}

// CorruptElementStreamError raises a `CorruptElementStream` error
func CorruptElementStreamError(format string, a ...interface{}) *CorruptElementStream {
	return &CorruptElementStream{fmt.Errorf(format, a...)}
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
	reader         *bufio.Reader
	readerPos      int64
	readerSize     int64
	TransferSyntax TransferSyntax
	CharacterSet   *CharacterSet
}

// GetElement yields an `Element` from the active stream, and an `error` if something went wrong.
func (elementStream *ElementStream) GetElement() (Element, error) {
	element := Element{}
	element.sourceElementStream = elementStream

	startBytePos := elementStream.GetPosition()
	lower, err := elementStream.getUint16()
	if err != nil {
		return element, CorruptElementError("GetElement(): %v", err)
	}
	upper, err := elementStream.getUint16()
	if err != nil {
		return element, CorruptElementError("GetElement(): %v", err)
	}
	tagUint32 := (uint32(lower) << 16) | uint32(upper)
	tag, _ := LookupTag(tagUint32)
	element.DictEntry = tag
	if !elementStream.TransferSyntax.Encoding.ImplicitVR { // if explicit, read VR from buffer
		if element.VR == "UN" { // but only if we dont already have VR from dictionary (more reliable)
			VRbytes, err := elementStream.getBytes(2)
			if err != nil {
				return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
			}
			element.VR = string(VRbytes)
		} else { // else just skip two bytes as we are using dictionary value
			err := elementStream.skipBytes(2)
			if err != nil {
				return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
			}
		}
		if element.VR == "OB" || element.VR == "OW" || element.VR == "SQ" || element.VR == "UN" || element.VR == "UT" {
			// these VRs, in explicit VR mode, have two reserved bytes following VR definition
			err := elementStream.skipBytes(2)
			if err != nil {
				return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
			}
			element.ValueLength, err = elementStream.getUint32()
			if err != nil {
				return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
			}
		} else {
			length, err := elementStream.getUint16()
			if err != nil {
				return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
			}
			element.ValueLength = uint32(length)
		}
	} else {
		// implicit VR -- all VR length definitions are 32 bits
		element.ValueLength, err = elementStream.getUint32()
		if err != nil {
			return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
		}
	}
	if element.ValueLength == 0xFFFFFFFF {
		var parseElements = (element.VR == "SQ")
		items, err := elementStream.getSequence(parseElements)
		if err != nil {
			return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
		}
		element.Items = items
	} else {
		// issue #4: Parser allows for element value length to exceed file size
		if int64(element.ValueLength) > elementStream.readerSize {
			return element, CorruptElementError("GetElement(): value length (%d) exceeds file size (%d)", element.ValueLength, elementStream.readerSize)
		}
		// string padding: should remove trailing+leading 0x00 / 0x20 bytes (see: http://dicom.nema.org/dicom/2013/output/chtml/part05/sect_6.2.html)
		// NOTE: some vendors pad with 0x20, some 0x00 -- seems to contradict NEMA spec. Let's account for both then:
		if element.ValueLength > 0 {
			valuebuf, err := elementStream.getBytes(uint(element.ValueLength))
			if err != nil {
				return element, CorruptElementError("GetElement(): [%s] %v", tag.Tag, err)
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
			element.value = bytes.NewBuffer(valuebuf)
		} else {
			element.value = bytes.NewBuffer([]byte{})
		}
	}

	element.ByteLengthTotal = (elementStream.GetPosition() - startBytePos)
	return element, nil
}

// getSequence parses a sequence of "unlimited length" from the bytestream
func (elementStream *ElementStream) getSequence(parseElements bool) ([]Item, error) {
	var items []Item
	for {
		lower, err := elementStream.getUint16()
		if err != nil {
			return items, CorruptElementStreamError("getSequence(%v): %v", parseElements, err)
		}
		upper, err := elementStream.getUint16()
		if err != nil {
			return items, CorruptElementStreamError("getSequence(%v): %v", parseElements, err)
		}
		tagUint32 := (uint32(lower) << 16) | uint32(upper)
		if tagUint32 == 0xFFFEE0DD {
			err := elementStream.skipBytes(4)
			if err != nil {
				return items, CorruptElementStreamError("getSequence(%v): %v", parseElements, err)
			}
			break
		}
		if tagUint32 != 0xFFFEE000 {
			return items, CorruptElementStreamError("getSequence(%v): 0x%08X != 0xFFFEE000", parseElements, tagUint32)
		}
		length, err := elementStream.getUint32()
		if err != nil {
			return items, CorruptElementStreamError("getSequence(%v): %v", parseElements, err)
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
					return items, CorruptDicomError("getSequence(%v): %v", parseElements, err)
				}
				elements[uint32(e.Tag)] = e
				check, err := elementStream.reader.Peek(4)
				if err != nil {
					return items, CorruptElementStreamError("getSequence(%v): %v", parseElements, err)
				}
				if bytes.Compare(check, delimitationItemBytes) == 0 {
					// end
					break
				}
			}

			// now we must skip eight bytes (delimitation item + 0x00{4}) (see: NEMA Table 7.5-3)
			err = elementStream.skipBytes(8)
			if err != nil {
				return items, CorruptElementStreamError("getSequence(%v): %v", parseElements, err)
			}
		} else {
			// try to grab an element according to current TransferSyntax
			if !parseElements {
				valuebuffer, err := elementStream.getBytes(uint(length))
				if err != nil {
					return items, CorruptElementStreamError("getSequence(%v): %v", parseElements, err)
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
					return items, CorruptDicomError("getSequence(%v): %v", parseElements, err)
				}
				elements[uint32(element.Tag)] = element
			}
		}
		item := Item{Elements: elements, UnknownSections: unknownBuffers}
		items = append(items, item)
	}

	return items, nil
}

func (elementStream *ElementStream) skipBytes(num int) error {
	if num == 0 {
		return nil
	}
	nseek, err := elementStream.reader.Discard(num)
	elementStream.readerPos += int64(nseek)
	if nseek < num {
		return CorruptElementStreamError("skipBytes(%d): nseek = %d", num, nseek)
	}
	if err != nil {
		return CorruptElementStreamError("skipBytes(%d): %v", num, err)
	}
	return nil
}

// GetPosition returns the current buffer position
func (elementStream *ElementStream) GetPosition() (pos int64) {
	pos = elementStream.readerPos
	return
}

// GetRemainingBytes returns the number of remaining unread bytes
func (elementStream *ElementStream) GetRemainingBytes() (num int64) {
	num = elementStream.readerSize - elementStream.readerPos
	return
}

func (elementStream *ElementStream) getUint16() (uint16, error) {
	buf := make([]byte, 2)
	nread, err := io.ReadFull(elementStream.reader, buf)
	if err != nil {
		return 0, CorruptElementStreamError("getUint16(): %v", err)
	}
	elementStream.readerPos += int64(nread)
	if nread != 2 {
		return 0, CorruptElementStreamError("getUint16(): nread = %d (!= 2)", nread)
	}
	if elementStream.TransferSyntax.Encoding.LittleEndian {
		return binary.LittleEndian.Uint16(buf), nil
	}
	return binary.BigEndian.Uint16(buf), nil
}

func (elementStream *ElementStream) getUint32() (uint32, error) {
	buf := make([]byte, 4)
	nread, err := io.ReadFull(elementStream.reader, buf)
	if err != nil {
		return 0, CorruptElementStreamError("getUint32(): %v", err)
	}
	elementStream.readerPos += int64(nread)
	if nread != 4 {
		return 0, CorruptElementStreamError("getUint32(): nread = %d (!= 4)", nread)
	}

	if elementStream.TransferSyntax.Encoding.LittleEndian {
		return binary.LittleEndian.Uint32(buf), nil
	}
	return binary.BigEndian.Uint32(buf), nil
}

func (elementStream *ElementStream) getBytes(num uint) ([]byte, error) {
	if num == 0 {
		return []byte{}, nil
	}
	if num > uint(elementStream.GetRemainingBytes()) {
		return nil, CorruptElementStreamError("getBytes(%d): would exceed buffer size (%d bytes)", num, elementStream.GetRemainingBytes())
	}
	buf := make([]byte, num)
	nread, err := io.ReadFull(elementStream.reader, buf)
	if err != nil {
		return buf, CorruptElementStreamError("getBytes(%d): %v", num, err)
	}
	elementStream.readerPos += int64(nread)
	if uint(nread) != num {
		return buf, CorruptElementStreamError("getBytes(%d): nread = %d (!= %d)", num, nread, num)
	}
	return buf, nil
}

// NewElementStream sets up a new `ElementStream`
func NewElementStream(readerPtr *bufio.Reader, readerSize int64) (stream ElementStream) {
	stream = ElementStream{TransferSyntax: TransferSyntax{}}
	stream.CharacterSet = CharacterSetMap["Default"]
	stream.reader = readerPtr
	stream.readerSize = readerSize
	stream.SetTransferSyntax("1.2.840.10008.1.2.1")
	return
}

// SetTransferSyntax sets the `ElementStream`s TransferSyntax according to uid string
func (elementStream *ElementStream) SetTransferSyntax(transferSyntaxUID string) {
	elementStream.TransferSyntax.SetFromUID(transferSyntaxUID)
	elementStream.TransferSyntax.Encoding = GetEncodingForTransferSyntax(elementStream.TransferSyntax)
}

func (df *Dicom) crawlMeta() error {
	preamble, err := df.elementStream.getBytes(128)
	if err != nil {
		return CorruptDicomError("crawlMeta(): %v", err)
	}
	copy(df.Preamble[:], preamble)
	dicmTestString, err := df.elementStream.getBytes(4)
	if err != nil {
		return CorruptDicomError("crawlMeta(): %v", err)
	}
	if string(dicmTestString) != "DICM" {
		return &NotADicom{}
	}

	metaLengthElement, err := df.elementStream.GetElement()
	if err != nil {
		return CorruptDicomError("crawlMeta: %v", err)
	}
	df.Elements[uint32(metaLengthElement.Tag)] = metaLengthElement

	if val, ok := metaLengthElement.Value().(uint32); ok {
		df.TotalMetaBytes = df.elementStream.GetPosition() + int64(val)
	} else {
		return CorruptDicomError("meta length element is corrupt")
	}

	for {
		element, err := df.elementStream.GetElement()

		if err != nil {
			return CorruptDicomError("crawlMeta: %v", err)
		}
		df.Elements[uint32(element.Tag)] = element

		if df.elementStream.GetPosition() >= df.TotalMetaBytes {
			break
		}
	}

	return nil
}

func (df *Dicom) crawlElements() error {
	transfersyntaxuid := "1.2.840.10008.1.2.1"
	// change transfer syntax if necessary
	tsElement, found := df.GetElement(0x0020010)
	if found {
		if transfersyntaxuid, ok := tsElement.Value().(string); ok {
			supported := checkTransferSyntaxSupport(transfersyntaxuid)
			if !supported {
				return &UnsupportedDicom{fmt.Errorf("unsupported transfer syntax: %s", transfersyntaxuid)}
			}
		} else {
			return CorruptDicomError("TransferSyntaxUID is corrupt")
		}
	}
	df.elementStream.SetTransferSyntax(transfersyntaxuid)

	for {
		element, err := df.elementStream.GetElement()
		if err != nil {
			return CorruptDicomError("crawlElements(): %v", err)
		}
		df.Elements[uint32(element.Tag)] = element

		switch element.Tag {
		case 0x00080005:
			if val, ok := element.Value().([]string); ok {
				if len(val) > 0 {
					df.elementStream.CharacterSet = CharacterSetMap[val[0]]
				}
			} // TODO: Should bad CharacterSet result in CorruptDicom, or instead use UTF8?
		}

		if df.elementStream.GetPosition() >= df.elementStream.readerSize {
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

	f, err := os.Open(path)
	if err != nil {
		return dcm, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return dcm, err
	}

	dcm.reader = bufio.NewReaderSize(f, DicomReadBufferSize)
	dcm.elementStream = NewElementStream(dcm.reader, stat.Size())

	if err := dcm.crawlMeta(); err != nil {
		switch err.(type) {
		case *NotADicom:
			return dcm, &NotADicom{fmt.Errorf(`The file "%s" is not a valid dicom`, filepath.Base(path))}
		default:
			return dcm, CorruptDicomError(`The file "%s" is corrupt: %v`, filepath.Base(path), err)
		}
	}
	if err := dcm.crawlElements(); err != nil {
		return dcm, CorruptDicomError(`The dicom "%s" is corrupt: %v`, filepath.Base(path), err)
	}

	return dcm, nil
}

// ParseFromBytes parses a dicom from a bytestream
func ParseFromBytes(source []byte) (Dicom, error) {
	dcm := Dicom{}
	r := bytes.NewReader(source)
	dcm.reader = bufio.NewReaderSize(r, DicomReadBufferSize)
	dcm.elementStream = NewElementStream(dcm.reader, int64(len(source)))
	dcm.Elements = make(map[uint32]Element)

	if err := dcm.crawlMeta(); err != nil {
		switch err.(type) {
		case *NotADicom:
			return dcm, &NotADicom{fmt.Errorf(`The bytes do not form a valid dicom`)}
		default:
			return dcm, CorruptDicomError(`The bytes are corrupt: %v`, err)
		}
	}
	if err := dcm.crawlElements(); err != nil {
		return dcm, CorruptDicomError(`The bytes are corrupt: %v`, err)
	}
	dcm.reader = nil
	dcm.elementStream.reader = nil
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
