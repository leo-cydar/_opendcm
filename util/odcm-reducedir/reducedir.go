// 2>/dev/null;/usr/bin/env go run $0 $@; exit $?
// Recursively searches a directory for unique dicoms
// unique being defined as previously unseen SeriesInstanceUID

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/b71729/opendcm/common"
	"github.com/b71729/opendcm/dicom"
)

var console = common.NewConsoleLogger(os.Stdout)

// Copy the src file to dst. Any existing file will be overwritten and will not
// copy file attributes.
func Copy(src, dst string) error {
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

func main() {
	if len(os.Args) != 3 {
		console.Fatalf("usage: %s in_dir out_dir", filepath.Base(os.Args[0]))
	}

	dirIn := os.Args[1]
	dirOut := os.Args[2]

	statIn, err := os.Stat(dirIn)
	if err != nil {
		console.Fatal(err)
	}
	if !statIn.IsDir() {
		console.Fatalf("%s is not a directory", dirIn)
	}

	statOut, err := os.Stat(dirOut)
	if err != nil {
		console.Fatal(err)
	}
	if !statOut.IsDir() {
		console.Fatalf("%s is not a directory", dirOut)
	}

	seriesInstanceUIDs := make(map[string]bool, 0)
	common.ConcurrentlyWalkDir(dirIn, func(filePath string) {
		dcm, err := dicom.ParseDicom(filePath)
		if err != nil {
			console.Errorf("error parsing %s: %v", filePath, err)
			return
		}
		if e, found := dcm.GetElement(0x0020000E); found {
			if val, ok := e.Value().(string); ok {
				_, found := seriesInstanceUIDs[val]
				if !found {
					console.Infof("%s", val)
					seriesInstanceUIDs[val] = true
					outputFilePath := filepath.Join(dirOut, fmt.Sprintf("%s.dcm", val))
					if _, err := os.Stat(outputFilePath); os.IsNotExist(err) {
						// file does not exist - lets create it
						err := Copy(dcm.FilePath, outputFilePath)
						if err != nil {
							console.Fatalf("error copying: %v", err)
						}
					} else {
						console.Infof("skip %s: file exists", outputFilePath)
					}
				}
			}
		}
	})
}
