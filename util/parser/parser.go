package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/b71729/opendcm/core"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

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
		dcm, err := core.ParseDicom(os.Args[1])
		var elements []core.Element
		for _, v := range dcm.Elements {
			elements = append(elements, v)
		}
		sort.Sort(core.ByTag(elements))
		for _, element := range elements {
			description := element.Describe()
			for _, line := range description {
				log.Println(line)
			}
		}
		if err != nil {
			log.Fatalf("DICOM parsing error: %v", err)
		}
	} else {
		// parse directory
		var channels []chan core.DicomFileChannel
		guard := make(chan struct{}, 128) // TODO: Handle too many open files
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
			c := make(chan core.DicomFileChannel)
			go core.ParseDicomChannel(path, c, guard)
			channels = append(channels, c)
		}

		for _, v := range channels {
			dcm := <-v
			if dcm.Error != nil {
				log.Printf("%v", dcm.Error)
			} else {
				e, found := dcm.DicomFile.GetElement(0x00080005)
				if found {
					log.Printf("File %s has hncoding: %s", dcm.DicomFile.FilePath, e.Value())
				}
			}
		}
	}
}
