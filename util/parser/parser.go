package main

import (
	"log"
	"os"

	"github.com/b71729/opendcm/core"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {
	dcm, err := core.ParseDicom(os.Args[1])
	for _, element := range dcm.Meta.Elements {
		log.Printf("[%s] %s = %v", element.VR, element.Name, element.Value())
	}
	//check(err)
	if err != nil {
		log.Fatalf("DICOM parsing error: %v", err)
	}
}
