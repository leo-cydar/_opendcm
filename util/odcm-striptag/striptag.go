// 2>/dev/null;/usr/bin/env go run $0 $@; exit $?
// Package main implements a CLI for removing a tag from dicom file(s)
package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/b71729/opendcm/common"
	"github.com/b71729/opendcm/dicom"
)

var log = common.NewConsoleLogger(os.Stdout)

func stripTag(path string, tag uint32, outdir string, deleteSource bool) {
	filename := filepath.Base(path)
	dcm, err := dicom.ParseDicom(path)
	if err != nil {
		log.Warnf("error parsing %s: %v", filename, err)
		return
	}
	element, found := dcm.GetElement(uint32(tag))
	if !found {
		log.Errorf("error parsing %s: tag %08X could not be found", filename, tag)
		return
	}
	log.Infof("tag found at offset %d (length %d)", element.FileOffsetStart, element.ByteLengthTotal)

	// open input file and read all contents to buffer
	infile, err := os.Open(path)
	if err != nil {
		log.Errorf("error parsing %s: %v", filename, err)
		return
	}
	stat, err := infile.Stat()
	if err != nil {
		log.Errorf("error: %v", err)
		return
	}
	inBuffer := make([]byte, stat.Size())
	var outBuffer []byte
	infile.Read(inBuffer) // TODO: this might not read the whole buffer
	outBuffer = append(outBuffer, inBuffer[:element.FileOffsetStart]...)
	outBuffer = append(outBuffer, inBuffer[(element.FileOffsetStart+element.ByteLengthTotal):]...)

	outpath := filepath.Join(outdir, fmt.Sprintf("%s.dcm", sha256.Sum256(outBuffer)))
	//create output file
	outfile, err := os.Create(outpath)
	if err != nil {
		log.Errorf("error: %v", err)
		return
	}
	defer outfile.Close()

	outfile.Write(outBuffer)

	if deleteSource {
		err = os.Remove(path)
		if err != nil {
			log.Errorf("error deleting source: %v", err)
			return
		}
	}
}

func main() {

	if len(os.Args) != 4 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		log.Fatalf("usage: %s in_file_or_dir out_dir (tttt,tttt)", filepath.Base(os.Args[0]))
	}
	tagString := strings.Replace(os.Args[3], ",", "", 1)
	tagInt, err := strconv.ParseUint(tagString, 16, 32)
	if err != nil {
		log.Fatalf("%v", err)
	}

	// validate out_dir
	stat, err := os.Stat(os.Args[2])
	if err != nil {
		log.Fatalf("failed to stat '%s': %v", os.Args[2], err)
	}

	if !stat.IsDir() {
		log.Fatalf("%s is not a valid output directory.", os.Args[2])
	}

	// validate input file/directory
	stat, err = os.Stat(os.Args[1])
	if err != nil {
		log.Fatalf("failed to stat '%s': %v", os.Args[1], err)
	}
	isDir := stat.IsDir()
	if !isDir {
		stripTag(os.Args[1], uint32(tagInt), os.Args[2], true) // WARNING: change to true will delete source data
	} else {
		// parse directory
		var files []string

		filepath.Walk(os.Args[1], func(path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Fatalf("prevent panic by handling failure accessing a path %q: %v", os.Args[1], err)
			}
			if !info.IsDir() {
				files = append(files, path)
			}
			return nil
		})

		for _, path := range files {
			stripTag(path, uint32(tagInt), os.Args[2], true)
		}
	}

}
