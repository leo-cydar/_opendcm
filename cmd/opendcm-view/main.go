package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	. "github.com/b71729/opendcm" // yes, dot imports are discouraged, but otherwise prefixing everything is a pain in the arse
)

/*
===============================================================================
    Util: View DICOM File
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
	fmt.Printf("usage: %s file_or_dir\n", baseFile)
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
	stat, err := os.Stat(os.Args[1])
	check(err)
	if isDir := stat.IsDir(); !isDir {
		dcm, err := ParseDicom(os.Args[1])
		check(err)
		var elements []Element
		for _, v := range dcm.Elements {
			elements = append(elements, v)
		}
		sort.Sort(ByTag(elements))
		for _, element := range elements {
			description := element.Describe(0)
			for _, line := range description {
				fmt.Println(line)
			}
		}
	} else {
		errorCount := 0
		successCount := 0
		err := ConcurrentlyWalkDir(os.Args[1], func(path string) {
			_, err := ParseDicom(path)
			basePath := filepath.Base(path)
			if err != nil {
				Errorf(`error parsing "%s": %v`, basePath, err)
				errorCount++
				return
			}
			successCount++
			Debugf(`parsed "%s"`, basePath)
		})
		check(err)
		if errorCount == 0 {
			Infof("parsed %d files without errors", successCount)
		} else {
			Infof("parsed %d files without errors, and failed to parse %d files", successCount, errorCount)
		}
	}
}
