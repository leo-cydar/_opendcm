package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/b71729/opendcm"
)

var console = opendcm.NewConsoleLogger(os.Stdout)

var baseFile = filepath.Base(os.Args[0])

func check(err error) {
	if err != nil {
		console.Fatal(err)
	}
}

func startInspect() {
	if len(os.Args) != 3 {
		console.Fatalf("usage: %s inspect file_or_dir", baseFile)
	}
	stat, err := os.Stat(os.Args[2])
	check(err)
	if isDir := stat.IsDir(); !isDir {
		dcm, err := opendcm.ParseDicom(os.Args[2])
		if err != nil {
			console.Fatalf(`error parsing "%s": %v`, dcm.FilePath, err)
		}
		var elements []opendcm.Element
		for _, v := range dcm.Elements {
			elements = append(elements, v)
		}
		sort.Sort(opendcm.ByTag(elements))
		for _, element := range elements {
			description := element.Describe()
			for _, line := range description {
				console.Info(line)
			}
		}
	} else {
		err := opendcm.ConcurrentlyWalkDir(os.Args[2], func(path string) {
			_, err := opendcm.ParseDicom(path)
			if err == nil {
				console.Infof("%s: parsed ok", filepath.Base(path))
			} else {
				console.Errorf("%s: %v", filepath.Base(path), err)
			}

		})
		check(err)
	}
}

func exitWithUsage() {
	console.Fatalf("usage: %s [%s] [flags]", baseFile, strings.Join([]string{"inspect", "reduce"}, " / "))
}

func main() {
	console.Infof("opendcm version %s", opendcm.OpenDCMVersion)
	if len(os.Args) == 1 || (os.Args[1] == "--help" || os.Args[1] == "-h") {
		exitWithUsage()
	}
	cmd := os.Args[1]
	switch cmd {
	case "inspect":
		startInspect()
	default:
		exitWithUsage()
	}
}
