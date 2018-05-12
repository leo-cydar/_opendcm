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
	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		usage()
	}
	if len(os.Args) != 2 {
		usage()
	}
	stat, err := os.Stat(os.Args[1])
	check(err)
	if isDir := stat.IsDir(); !isDir {
		dcm, err := od.FromFile(os.Args[1])
		check(err)
		for _, element := range dcm.DataSet {
			fmt.Printf("%08x = %v\n", element.GetTag(), element.GetName())
		}
		pd := dcm.GetPixelData()
		fmt.Printf("NUM PIXEL FRAMES: %d\n", pd.NumFrames())
		for i := 0; i < pd.NumFrames(); i++ {
			fmt.Printf("Frame: (len %d)\n", len(pd.GetFrame(i)))
			f, err := os.Create(fmt.Sprintf("frame-%d.jpg", i))
			check(err)
			f.Write(pd.GetFrame(i)[:len(pd.GetFrame(i))-1])
			f.Close()
		}
		tsuid := ""
		found, err := dcm.GetElementValue(0x00020010, &tsuid)
		check(err)
		if !found {
			panic("not found")
		}
		fmt.Printf("TS = %s\n", tsuid)
	} else {
		errorCount := 0
		successCount := 0
		err := od.ConcurrentlyWalkDir(os.Args[1], func(path string) {
			_, err := od.FromFile(path)
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
