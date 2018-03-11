// 2>/dev/null;/usr/bin/env go run $0 $@; exit $?
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/b71729/opendcm/common"
	"github.com/b71729/opendcm/dicom"
)

var console = common.NewConsoleLogger(os.Stdout)

func main() {
	if len(os.Args) != 3 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		console.Fatalf("usage: %s in_file (tttt,tttt)", filepath.Base(os.Args[0]))
	}

	inFile := os.Args[1]
	tagString := strings.Replace(os.Args[2], ",", "", 1)
	tagInt, err := strconv.ParseUint(tagString, 16, 32)
	if err != nil {
		console.Fatalf("%v", err)
	}

	// validate inFile
	stat, err := os.Stat(inFile)
	if err != nil {
		console.Fatalf(`failed to stat "%s": %v`, inFile, err)
	}
	if stat.IsDir() {
		console.Fatalf("%s is a directory. please specify one file.", inFile)
	}

	dcm, err := dicom.ParseDicom(inFile)
	if err != nil {
		console.Fatalf("error parsing dicom: %v", err)
	}
	element, found := dcm.GetElement(uint32(tagInt))
	if !found {
		console.Fatalf("tag %08X could not be found in file %s", tagInt, inFile)
	}
	f, err := os.Open(inFile)
	if err != nil {
		console.Fatalf("error opening %s: %v", inFile, err)
	}
	buffer := make([]byte, element.ByteLengthTotal)
	nread, err := f.ReadAt(buffer, element.FileOffsetStart)
	if err != nil {
		console.Fatalf("error reading file: %v", err)
	}
	if int64(nread) != element.ByteLengthTotal {
		console.Fatalf("nread = %d (!= %d)", nread, element.ByteLengthTotal)
		return
	}
	fmt.Printf("\nContents:\n\n[]byte{")
	for i, b := range buffer {
		if i+1 == len(buffer) {
			fmt.Printf("0x%02X}", b)
			break
		}
		fmt.Printf("0x%02X, ", b)
	}
	fmt.Printf("\n\n")

}
