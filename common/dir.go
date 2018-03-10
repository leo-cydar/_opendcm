package core

import (
	"os"
	"path/filepath"
)

// OpenFileLimit restricts the number of concurrently open files
var OpenFileLimit = 64

// ConcurrentlyWalkDir recursively traverses a directory and calls `onFile` for each found file inside a goroutine.
func ConcurrentlyWalkDir(dirPath string, onFile func(file string)) error {
	guard := make(chan bool, OpenFileLimit) // limits number of concurrently open files
	var files []string

	err := filepath.Walk(dirPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		files = append(files, filePath)
		return nil
	})
	if err != nil {
		return err
	}

	// now goroutine each file
	for _, filePath := range files {
		guard <- true // would block if guard channel is already filled
		go func(path string) {
			onFile(path)
			<-guard
		}(filePath)
	}
	return nil
}
