package main

import (
	"log"
	"os"
	"sort"

	"github.com/b71729/opendcm/core"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {
	dcm, err := core.ParseDicom(os.Args[1])
	var elements []core.Element
	for _, v := range dcm.Elements {
		elements = append(elements, v)
	}
	sort.Sort(core.ByTag(elements))
	for _, element := range elements {
		log.Printf("[%s] %s (%d bytes) %s", element.VR, element.Tag, element.ValueLength, element.Name)
	}
	if err != nil {
		log.Fatalf("DICOM parsing error: %v", err)
	}
}
