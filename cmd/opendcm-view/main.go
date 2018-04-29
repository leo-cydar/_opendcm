package main

import (
	"fmt"
	"os"
	"path/filepath"

	od "github.com/b71729/opendcm"
)

/*
===============================================================================
    Util: View DICOM File
===============================================================================
*/

var baseFile = filepath.Base(os.Args[0])

func check(err error) {
	if err != nil {
		od.FatalfDepth(3, "error: %v", err)
	}
}

func usage() {
	fmt.Printf("OpenDCM version %s\n", od.OpenDCMVersion)
	fmt.Printf("usage: %s file_or_dir\n", baseFile)
	os.Exit(1)
}

func main() {
	od.GetConfig()
	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		usage()
	}
	if len(os.Args) != 2 {
		usage()
	}
	stat, err := os.Stat(os.Args[1])
	check(err)
	if isDir := stat.IsDir(); !isDir {
		dcm := od.NewDicom()
		err := dcm.FromFile(os.Args[1])
		check(err)
		// var elements []od.Element
		// for _, v := range dcm.Elements {
		// 	elements = append(elements, v)
		// }
		// sort.Sort(od.ByTag(elements))
		for _, element := range *dcm.GetDataSet() {
			// description := element.Describe(0)
			// for _, line := range description {
			// 	fmt.Println(line)
			// }
			fmt.Printf("%08x = %v\n", element.GetTag(), element.GetName())
		}
		name := ""
		var e = od.NewElement()
		dcm.GetDataSet().GetElement(0x00100010, &e)
		e.GetValue(&name)
		fmt.Printf("PatientName = %s\n", name)
	} else {
		errorCount := 0
		successCount := 0
		err := od.ConcurrentlyWalkDir(os.Args[1], func(path string) {
			dcm := od.NewDicom()
			err := dcm.FromFile(path)
			check(err)
			basePath := filepath.Base(path)
			if err != nil {
				od.Errorf(`error parsing "%s": %v`, basePath, err)
				errorCount++
				return
			}
			successCount++
			od.Debugf(`parsed "%s"`, basePath)
		})
		check(err)
		if errorCount == 0 {
			od.Infof("parsed %d files without errors", successCount)
		} else {
			od.Infof("parsed %d files without errors, and failed to parse %d files", successCount, errorCount)
		}
	}
}
