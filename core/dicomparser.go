package core

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"

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
	uidptr, err := LookupUID(uidstr)
	if err != nil {
		return err
	}
	ts.UIDEntry = uidptr
	ts.Encoding = GetEncodingForTransferSyntax(*ts)
	log.Printf("Using '%v'  (ImplicitVR: %v, LittleEndian: %v)", ts.UIDEntry.NameHuman, ts.Encoding.ImplicitVR, ts.Encoding.LittleEndian)
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

type VRSpecification struct {
	VR                 string
	MaximumLengthBytes uint32
	FixedLength        bool
	CharsetRe          *regexp.Regexp
}

var defaultCharsetRe = regexp.MustCompile("^[[:ascii:]]+$")
var defaultCharsetReadbleOnlyRe = regexp.MustCompile("^([\x00-\x09|\x13|\x16-\x1A|\x1C-\x5B]|[\x5D-\x7F])+$")
var ageStringRe = regexp.MustCompile("^[0-9DWMY]+$")
var codeStringRe = regexp.MustCompile(`^[A-Z0-9_ \\]+$`)
var dateRe = regexp.MustCompile("^[0-9-]+$")
var decimalStringRe = regexp.MustCompile(`^[0-9+-Ee\.\\]+$`)
var dateTimeRe = regexp.MustCompile("^[0-9+-\\.]+$")
var integerStringRe = regexp.MustCompile("^[0-9+-]+$")
var patientNameRe = regexp.MustCompile("^([\x00-\x09|\x13|\x16-\x5B]|[\x5D-\x7F])+$")
var timeRe = regexp.MustCompile("^[0-9-\\. ]+$")
var uniqueIdentifierRe = regexp.MustCompile("^[0-9\\.]+$")

//  excluding character code 5CH (the BACKSLASH "\"), and control characters LF, FF, CR and ESC.

var VRConformanceMap = map[string]*VRSpecification{
	"AE": &VRSpecification{VR: "AE", MaximumLengthBytes: 16, CharsetRe: defaultCharsetReadbleOnlyRe},
	"AS": &VRSpecification{VR: "AS", MaximumLengthBytes: 4, CharsetRe: ageStringRe, FixedLength: true},
	"AT": &VRSpecification{VR: "AT", MaximumLengthBytes: 4, CharsetRe: nil, FixedLength: true},
	"CS": &VRSpecification{VR: "CS", MaximumLengthBytes: 0xFFFFFFFF, CharsetRe: codeStringRe},
	"DA": &VRSpecification{VR: "DA", MaximumLengthBytes: 18, CharsetRe: dateRe},
	"DS": &VRSpecification{VR: "DS", MaximumLengthBytes: 0xFFFFFFFF, CharsetRe: decimalStringRe},
	"DT": &VRSpecification{VR: "DT", MaximumLengthBytes: 54, CharsetRe: dateTimeRe},
	"FL": &VRSpecification{VR: "FL", MaximumLengthBytes: 4, CharsetRe: nil, FixedLength: true},
	"FD": &VRSpecification{VR: "FD", MaximumLengthBytes: 8, CharsetRe: nil, FixedLength: true},
	"IS": &VRSpecification{VR: "IS", MaximumLengthBytes: 12, CharsetRe: integerStringRe},
	"LO": &VRSpecification{VR: "LO", MaximumLengthBytes: 64, CharsetRe: defaultCharsetRe},
	"LT": &VRSpecification{VR: "LT", MaximumLengthBytes: 10240, CharsetRe: defaultCharsetRe},
	"OB": &VRSpecification{VR: "OB", MaximumLengthBytes: 0xFFFFFFFF},
	"OD": &VRSpecification{VR: "OD", MaximumLengthBytes: 0xFFFFFFF8},
	"OF": &VRSpecification{VR: "OF", MaximumLengthBytes: 0xFFFFFFFC},
	"OW": &VRSpecification{VR: "OW", MaximumLengthBytes: 0xFFFFFFFF},
	"PN": &VRSpecification{VR: "PN", MaximumLengthBytes: (64 * 5), CharsetRe: patientNameRe},
	"SH": &VRSpecification{VR: "SH", MaximumLengthBytes: 16, CharsetRe: defaultCharsetRe}, // NOTE: ambiguity in spec
	"SL": &VRSpecification{VR: "SL", MaximumLengthBytes: 4, FixedLength: true},
	"SQ": &VRSpecification{VR: "SQ", MaximumLengthBytes: 0xFFFFFFFF},
	"SS": &VRSpecification{VR: "SS", MaximumLengthBytes: 2, FixedLength: true},
	"ST": &VRSpecification{VR: "ST", MaximumLengthBytes: 1024, CharsetRe: defaultCharsetRe},
	"TM": &VRSpecification{VR: "TM", MaximumLengthBytes: 28, CharsetRe: timeRe},
	"UI": &VRSpecification{VR: "UI", MaximumLengthBytes: 64, CharsetRe: uniqueIdentifierRe},
	"UL": &VRSpecification{VR: "UL", MaximumLengthBytes: 4, FixedLength: true},
	"UN": &VRSpecification{VR: "UN", MaximumLengthBytes: 0xFFFFFFFF},
	"US": &VRSpecification{VR: "US", MaximumLengthBytes: 2, FixedLength: true},
	"UT": &VRSpecification{VR: "UT", MaximumLengthBytes: 0xFFFFFFFE, CharsetRe: defaultCharsetRe},
}

func (element Element) CheckConformance() bool {
	specification, found := VRConformanceMap[element.VR]
	if !found {
		log.Fatalf("Could not find conformance for VR %s", element.VR)
	}
	if specification.CharsetRe == nil || element.ValueLength == 0 || specification.CharsetRe.Match(element.value.Bytes()) {
		if specification.FixedLength && element.ValueLength == specification.MaximumLengthBytes {
			return true
		}
		if specification.MaximumLengthBytes != 0xFFFFFF {
			if element.ValueLength <= specification.MaximumLengthBytes {
				return true
			}
		}
	}

	return false
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
	return reader, nil
}

// ReadElement yields an `Element` from the active buffer, and an `error` if something went wrong.
func (dr *DicomFileReader) ReadElement() (Element, error) {
	element := Element{}
	element.LittleEndian = dr.TransferSyntax.Encoding.LittleEndian
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
	if element.ValueLength == 0xFFFFFFFF {
		var parseElements = (element.VR == "SQ")
		items, err := dr.readSequence(parseElements)
		if err != nil {
			return element, err
		}
		element.Items = items
	} else {
		valuebuf := make([]byte, element.ValueLength)
		// string padding: should remove trailing+leading 0x00 / 0x20 bytes (see: http://dicom.nema.org/dicom/2013/output/chtml/part05/sect_6.2.html)
		// NOTE: some vendors pad with 0x20, some 0x00 -- seems to contradict NEMA spec. Let's account for both then:
		if element.ValueLength > 0 {
			err = dr.read(valuebuf)
			if err != nil {
				return element, err
			}
			padchars := []byte{0x00, 0x20}
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
	}

	return element, nil
}

func (dr *DicomFileReader) readUntil(delimiter []byte) ([]byte, error) {
	if len(delimiter) > 8 {
		panic("does not support delimiters with length greater than 8 bytes")
	}
	var buf []byte
	for {
		currentBuffer, err := dr.readBytes(136) // 128 bytes plus maximum 8 bytes for delimiter boundary
		if err != nil {
			return buf, fmt.Errorf("readBytes(136) failed: %v", err)
		}
		delimiterPos := bytes.Index(currentBuffer, delimiter)
		if delimiterPos >= 0 { // found
			buf = append(buf[:], currentBuffer[:delimiterPos]...)
			_, err = dr._reader.Seek(-int64(136-delimiterPos), io.SeekCurrent)
			return buf, nil
		}
		buf = append(buf[:], currentBuffer[:128]...)
		_, err = dr._reader.Seek(-8, io.SeekCurrent)
		if err != nil {
			return buf, err
		}
	}
}

// readSequence parses a sequence of "unlimited length" from the bytestream
func (dr *DicomFileReader) readSequence(parseElements bool) ([]Item, error) {
	var items []Item
	for {
		lower, err := dr.readUint16()
		if err != nil {
			return nil, err
		}
		upper, err := dr.readUint16()
		if err != nil {
			return nil, err
		}
		tagUint32 := (uint32(lower) << 16) | uint32(upper)
		if tagUint32 == 0xFFFEE0DD {
			err := dr.skipBytes(4)
			if err != nil {
				return items, err
			}
			break
		}
		if tagUint32 != 0xFFFEE000 {
			return items, fmt.Errorf("0x%08X != 0xFFFEE000", tagUint32)
		}
		length, err := dr.readUint32()
		if err != nil {
			return nil, err
		}

		var elements = make(map[uint32]Element)
		var unknownBuffers [][]byte
		if length == 0xFFFFFFFF { // unlimited length item
			// find next FFFE, E00D = data for item ends
			var delimitationItemBytes []byte
			if dr.TransferSyntax.Encoding.LittleEndian {
				delimitationItemBytes = []byte{0xFE, 0xFF, 0x0D, 0xE0}
			} else {
				delimitationItemBytes = []byte{0xFF, 0xFE, 0xE0, 0x0D}
			}

			for {
				// try to grab an element according to current TransferSyntax
				e, err := dr.ReadElement()
				if err != nil {
					panic(err)
				}
				elements[uint32(e.Tag)] = e
				check, err := dr.readBytes(4)
				if bytes.Compare(check, delimitationItemBytes) == 0 {
					// end
					break
				}
				dr._reader.Seek(-4, io.SeekCurrent)
			}

			// now we must skip four bytes (0x00{4}) (see: NEMA Table 7.5-3)
			err = dr.skipBytes(4)
			if err != nil {
				return items, err
			}
		} else {
			// try to grab an element according to current TransferSyntax
			if !parseElements {
				valuebuffer, err := dr.readBytes(uint(length))
				if err != nil {
					panic(err) // TODO
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
				element, err := dr.ReadElement()
				if err != nil {
					panic(err)
				}
				elements[uint32(element.Tag)] = element
			}
		}
		item := Item{Elements: elements, UnknownSections: unknownBuffers}
		items = append(items, item)

	}

	return items, nil
}

func (dr *DicomFileReader) skipBytes(num int64) error {
	nseek, err := dr._reader.Seek(num, os.SEEK_CUR)
	if nseek < num || err != nil {
		return io.EOF
	}
	return nil
}

func (dr *DicomFileReader) getPosition() int64 {
	pos, _ := dr._reader.Seek(0, io.SeekCurrent)
	return pos
}

func (dr *DicomFileReader) read(v interface{}) error {
	if dr.TransferSyntax.Encoding.LittleEndian {
		return binary.Read(dr._reader, binary.LittleEndian, v)
	}
	return binary.Read(dr._reader, binary.BigEndian, v)
}

func (dr *DicomFileReader) readUint16() (uint16, error) {
	buf := make([]byte, 2)
	nread, err := dr._reader.Read(buf)
	if nread != 2 {
		return 0, io.EOF
	}
	if err != nil {
		return 0, err
	}
	if dr.TransferSyntax.Encoding.LittleEndian {
		return binary.LittleEndian.Uint16(buf), nil
	}
	return binary.BigEndian.Uint16(buf), nil
}

func (dr *DicomFileReader) readUint32() (uint32, error) {
	buf := make([]byte, 4)
	nread, err := dr._reader.Read(buf)
	if nread != 4 {
		return 0, io.EOF
	}
	if err != nil {
		return 0, err
	}
	if dr.TransferSyntax.Encoding.LittleEndian {
		return binary.LittleEndian.Uint32(buf), nil
	}
	return binary.BigEndian.Uint32(buf), nil
}

func (dr *DicomFileReader) readBytes(num uint) ([]byte, error) {
	buf := make([]byte, num)
	nread, err := dr._reader.Read(buf)
	if uint(nread) != num {
		return buf, io.EOF
	}
	if err != nil {
		return buf, err
	}
	return buf, nil
}

func (dr *DicomFileReader) BufferFromFile(nstart int64, nbytes int, acceptPartialRead bool) error {
	f, err := os.Open(dr.FilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}

	var bufferSize int64
	if nbytes == -1 {
		bufferSize = stat.Size()
	} else {
		bufferSize = int64(nbytes)
	}
	if nstart > -1 {
		bufferSize -= nstart
	}
	buffer := make([]byte, bufferSize)

	if nstart > -1 {
		nseek, err := f.Seek(nstart, io.SeekStart)
		if nseek < nstart || err != nil {
			return err
		}
	}
	nread, err := f.Read(buffer)
	if err != nil {
		return err
	}
	if !acceptPartialRead && (int64(nread) < bufferSize) {
		log.Printf("Wanted to read %d bytes, read %d bytes, total size %d bytes", nbytes, nread, stat.Size())
		return io.EOF
	}

	dr._reader = bytes.NewReader(buffer)
	return nil
}

type NotADicomFile struct {
}

func (n NotADicomFile) Error() string {
	return "input is not a DICOM file"
}

func (df *DicomFile) CrawlMeta() error {
	err := df.Reader.BufferFromFile(-1, 1024, true)
	if err != nil {
		return err
	}
	if df.Reader._reader == nil {
		return fmt.Errorf("Reader has nil pointer: %v", df.Reader)
	}

	if df.Reader._reader.Len() < 132 {
		return NotADicomFile{}
	}

	preamble, err := df.Reader.readBytes(128)
	if err != nil {
		return err
	}
	copy(df.Preamble[:], preamble)
	dicmTestString, err := df.Reader.readBytes(4)
	if err != nil {
		return err
	}
	if string(dicmTestString) != "DICM" {
		return NotADicomFile{}
	}

	metaLengthElement, err := df.Reader.ReadElement()
	check(err)
	df.Elements[uint32(metaLengthElement.Tag)] = metaLengthElement
	df.TotalMetaBytes = df.Reader.getPosition() + int64(metaLengthElement.Value().(uint32))
	for {
		element, err := df.Reader.ReadElement()
		if err != nil {
			log.Printf("Error parsing %v (SeekPos: %d)", err, (df.Reader.getPosition()))
			return err
		}
		df.Elements[uint32(element.Tag)] = element

		if df.Reader.getPosition() >= df.TotalMetaBytes {
			break
		}
	}

	return nil
}

func (df *DicomFile) CrawlElements() error {
	err := df.Reader.BufferFromFile(df.TotalMetaBytes, -1, false)
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
	fileStat, err := os.Stat(df.filepath)
	if err != nil {
		return err
	}
	fileSize := fileStat.Size()
	for {
		element, err := df.Reader.ReadElement()
		if err != nil {
			log.Printf("Error parsing %v (SeekPos: %d)", err, (df.Reader.getPosition() + df.TotalMetaBytes))
			return err
		}
		df.Elements[uint32(element.Tag)] = element

		if df.Reader.getPosition()+df.TotalMetaBytes >= fileSize {
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

	if err := dcm.CrawlMeta(); err != nil {
		return dcm, err
	}
	if err = dcm.CrawlElements(); err != nil {
		return dcm, err
	}

	return dcm, nil
}

func ParseDicomChannel(path string, c chan DicomFileChannel, s chan struct{}) {
	dcm, err := ParseDicom(path)
	<-s
	c <- DicomFileChannel{dcm, err}
}
