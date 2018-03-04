// Package main implements a dicom parser CLI
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/b71729/opendcm/file"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("Usage: parser FILE_OR_DIR")
	}
	stat, err := os.Stat(os.Args[1])
	if err != nil {
		log.Fatalf("Failed to stat '%s': %v", os.Args[1], err)
	}
	isDir := stat.IsDir()
	if !isDir {
		dcm, err := file.ParseDicom(os.Args[1])
		if err != nil {
			log.Printf("%v", err)
		} else {
			var elements []file.Element
			for _, v := range dcm.Elements {
				elements = append(elements, v)
			}
			sort.Sort(file.ByTag(elements))
			for _, element := range elements {
				description := element.Describe()
				for _, line := range description {
					log.Println(line)
				}
			}
		}
	} else {
		// parse directory
		var dicomchannels []chan file.DicomFile
		var errorchannels []chan error
		guard := make(chan struct{}, 64) // TODO: Handle too many open files
		var files []string

		filepath.Walk(os.Args[1], func(path string, info os.FileInfo, err error) error {
			if err != nil {
				fmt.Printf("prevent panic by handling failure accessing a path %q: %v\n", os.Args[1], err)
				return err
			}
			if !info.IsDir() {
				files = append(files, path)
			}
			return nil
		})

		// now goroutine each file
		for _, path := range files {
			guard <- struct{}{} // would block if guard channel is already filled
			dicomchannel := make(chan file.DicomFile)
			errorchannel := make(chan error)
			go file.ParseDicomChannel(path, dicomchannel, errorchannel, guard)
			dicomchannels = append(dicomchannels, dicomchannel)
			errorchannels = append(errorchannels, errorchannel)
		}
		for i := 0; i < len(dicomchannels); i++ {
			select {
			case err := <-errorchannels[i]:
				log.Printf("%v", err)
			case dcm := <-dicomchannels[i]:
				e, found := dcm.GetElement(0x00080005)
				if found {
					log.Printf("File %s has CharSet: %s", dcm.FilePath, e.Value())
				}
			}
		}
	}
}
