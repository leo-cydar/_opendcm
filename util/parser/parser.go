package main

import (
	"os"

	"github.com/b71729/dcm/core"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {
	_, err := core.ParseDicom(os.Args[1])
	check(err)
}
