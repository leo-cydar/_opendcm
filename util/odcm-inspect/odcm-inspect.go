// Package main implements a dicom inspector CLI
package main

import (
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/b71729/opendcm/common"
	"github.com/b71729/opendcm/dicom"
)

var console = common.NewConsoleLogger(os.Stdout)

func main() {
	if len(os.Args) != 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		console.Fatalf("usage: %s file_or_dir", filepath.Base(os.Args[0]))
	}
	stat, err := os.Stat(os.Args[1])
	if err != nil {
		console.Fatalf(`failed to stat "%s": %v`, os.Args[1], err)
	}
	if isDir := stat.IsDir(); !isDir {
		dcm, err := dicom.ParseDicom(os.Args[1])
		if err != nil {
			console.Fatalf(`error parsing "%s": %v`, dcm.FilePath, err)
		}
		var elements []dicom.Element
		for _, v := range dcm.Elements {
			elements = append(elements, v)
		}
		sort.Sort(dicom.ByTag(elements))
		for _, element := range elements {
			description := element.Describe()
			for _, line := range description {
				console.Info(line)
			}
		}
	} else {
		err := common.ConcurrentlyWalkDir(os.Args[1], func(filepath string) {
			dcm, err := dicom.ParseDicom(filepath)
			if err != nil {
				console.Errorf("%v", err)
			}
			// CharSet detection:
			// if e, found := dcm.GetElement(0x00080005); found {
			// 	if val, ok := e.Value().([]string); ok {
			// 		console.Infof("file %s has CharSet: %s", dcm.FilePath, val)
			// 	} else {
			// 		console.Infof("file %s CharSet is not of string type", dcm.FilePath)
			// 		return
			// 	}
			// }
			for _, e := range dcm.Elements {
				val := e.Value()
				switch val.(type) {
				case []string:
					if len(val.([]string)) > 10 {
						console.Infof("file %s has %d strings @ %s", dcm.FilePath, len(val.([]string)), e.Tag)
					}
				case []uint32:
					if len(val.([]uint32)) > 10 {
						console.Infof("file %s has %d uint32s @ %s", dcm.FilePath, len(val.([]uint32)), e.Tag)
					}
				case []float32:
					if len(val.([]float32)) > 10 {
						console.Infof("file %s has %d float32s @ %s", dcm.FilePath, len(e.Items), e.Tag)
					}
				case []int16:
					if len(val.([]int16)) > 10 {
						console.Infof("file %s has %d int16s @ %s", dcm.FilePath, len(val.([]int16)), e.Tag)
					}
				case []int32:
					if len(val.([]int32)) > 10 {
						console.Infof("file %s has %d int32s @ %s", dcm.FilePath, len(e.Items), e.Tag)
					}
				}
			}

		})
		if err != nil {
			log.Fatalf("error walking directory: %v", err)
		}
	}
}
