// Package main implements a dicom inspector CLI
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/b71729/opendcm/dicom"
)

// TermRed provides ansi escape codes for a red section.
func TermRed(s string) string {
	return fmt.Sprintf("\x1b[31;1m%s\x1b[0m", s)
}

// TermYellow provides ansi escape codes for a yellow section.
func TermYellow(s string) string {
	return fmt.Sprintf("\x1b[33;1m%s\x1b[0m", s)
}

// TermGreen provides ansi escape codes for a green section.
func TermGreen(s string) string {
	return fmt.Sprintf("\x1b[92;1m%s\x1b[0m", s)
}

func main() {
	if len(os.Args) != 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		fmt.Printf("  %s Usage: %s FILE_OR_DIR\n", TermRed("!!"), filepath.Base(os.Args[0]))
		return
	}
	stat, err := os.Stat(os.Args[1])
	if err != nil {
		fmt.Printf("  %s Failed to stat '%s': %v\n", TermRed("!!"), os.Args[1], err)
		return
	}
	if isDir := stat.IsDir(); !isDir {
		dcm, err := dicom.ParseDicom(os.Args[1])
		if err != nil {
			fmt.Printf("  %s %v\n", TermRed("!!"), err)
			return
		}
		var elements []dicom.Element
		for _, v := range dcm.Elements {
			elements = append(elements, v)
		}
		sort.Sort(dicom.ByTag(elements))
		for _, element := range elements {
			description := element.Describe()
			for _, line := range description {
				fmt.Printf("  %s %s\n", TermGreen("+"), line)
			}
		}
	} else {
		// parse directory
		var dicomchannels []chan dicom.Dicom
		var errorchannels []chan error
		guard := make(chan struct{}, 64) // TODO: Handle too many open files
		var files []string

		filepath.Walk(os.Args[1], func(path string, info os.FileInfo, err error) error {
			if err != nil {
				fmt.Printf("prevent panic by handling failure accessing a path %q: %v\n", os.Args[1], err)
				return err
			}
			if info.IsDir() {
				return nil
			}

			files = append(files, path)
			return nil
		})

		// now goroutine each file
		for i, path := range files {
			guard <- struct{}{} // would block if guard channel is already filled
			dicomchannels = append(dicomchannels, make(chan dicom.Dicom))
			errorchannels = append(errorchannels, make(chan error))
			go dicom.ParseDicomChannel(path, dicomchannels[i], errorchannels[i], guard)
		}
		for i := 0; i < len(dicomchannels); i++ {
			select {
			case err := <-errorchannels[i]:
				log.Printf("  %s %v", TermRed("!!"), err)
			case dcm := <-dicomchannels[i]:
				if e, found := dcm.GetElement(0x00080005); found {
					if val, ok := e.Value().([]string); ok {
						log.Printf("File %s has CharSet: %s", dcm.FilePath, val)
					} else {
						log.Printf("  %s File %s CharSet is not of string type\n", TermRed("!!"), dcm.FilePath)
						return
					}
				}
			}
		}
	}
}
