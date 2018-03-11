package fuzz

import (
	"github.com/b71729/opendcm/dicom"
)

// Fuzz is ran by go-fuzz
func Fuzz(data []byte) int {
	dcm, err := dicom.ParseFromBytes(data)
	if err != nil {
		switch err.(type) {
		case *dicom.CorruptDicom, *dicom.NotADicom:
			return 0
		default:
			return 1
		}
	}

	// ensure all parsed elements have the correct type:
	for _, e := range dcm.Elements {
		v := e.Value()
		switch e.VR {
		case "AE", "AS", "CS", "DA", "DS", "DT", "IS", "LO", "PN", "SH", "TM", "UI":
			switch v.(type) {
			case []string:
			case string:
			case nil:
			default:
				panic("String-like VR returned non-string-like type")
			}
		case "LT", "ST", "UT":
			switch v.(type) {
			case string:
			default:
				panic("String-like VR returned non-string-like type")
			}
		case "FL":
			switch v.(type) {
			case []float32:
			case float32:
			case nil:
			default:
				panic("float32 VR returned non-float32 type")
			}
		case "FD":
			switch v.(type) {
			case []float64:
			case float64:
			case nil:
			default:
				panic("float64 VR returned non-float64 type")
			}
		case "SS":
			switch v.(type) {
			case []int16:
			case int16:
			case nil:
			default:
				panic("int16 VR returned non-int16 type")
			}
		case "SL":
			switch v.(type) {
			case []int32:
			case int32:
			case nil:
			default:
				panic("int32 VR returned non-int32 type")
			}
		case "US":
			switch v.(type) {
			case []uint16:
			case uint16:
			case nil:
			default:
				panic("uint16 VR returned non-uint16 type")
			}
		case "UL":
			switch v.(type) {
			case []uint32:
			case uint32:
			case nil:
			default:
				panic("uint32 VR returned non-uint32 type")
			}

		}
	}
	return 1
}
