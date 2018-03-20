package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	. "github.com/b71729/opendcm"
)

/*
===============================================================================
    Util: Reduce DICOM Directory
===============================================================================
*/

// This scans the input directory for unique dicoms (unique SeriesInstanceUID) and copies those dicoms
//   to the output directory.

var baseFile = filepath.Base(os.Args[0])

func check(err error) {
	if err != nil {
		FatalfDepth(3, "error: %v", err)
	}
}

func usage() {
	fmt.Printf("OpenDCM version %s\n", OpenDCMVersion)
	fmt.Printf("usage: %s in_dir out_dir\n", baseFile)
	os.Exit(1)
}

func main() {
	GetConfig()
	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		usage()
	}
	if len(os.Args) != 3 {
		usage()
	}

	dirIn := os.Args[1]
	dirOut := os.Args[2]

	statIn, err := os.Stat(dirIn)
	check(err)
	if !statIn.IsDir() {
		Fatalf(`"%s" is not a directory. please provide a directory`, dirIn)
	}

	statOut, err := os.Stat(dirOut)
	check(err)
	if !statOut.IsDir() {
		Fatalf(`"%s" is not a directory. please provide a directory`, dirOut)
	}

	seriesInstanceUIDs := make(map[string]bool, 0)
	ConcurrentlyWalkDir(dirIn, func(filePath string) {
		dcm, err := ParseDicom(filePath)
		check(err)
		if e, found := dcm.GetElement(0x0020000E); found {
			if val, ok := e.Value().(string); ok {
				_, found := seriesInstanceUIDs[val]
				if !found {
					Infof("found unique: %s", val)
					seriesInstanceUIDs[val] = true
					outputFilePath := filepath.Join(dirOut, fmt.Sprintf("%s.dcm", val))
					if _, err := os.Stat(outputFilePath); os.IsNotExist(err) {
						// file does not exist - lets create it
						err := copy(dcm.FilePath, outputFilePath)
						check(err)
					} else {
						Infof(`skip "%s": file exists`, outputFilePath)
					}
				}
			}
		}
	})
}

// copy the src file to dst. Any existing file will be overwritten and will not
// copy file attributes.
// Source: https://stackoverflow.com/a/21061062
func copy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}
