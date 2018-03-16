package opendcm

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
)

// OpenDCMRootUID contains the official designated root UID prefox for OpenDCM
// Many thanks to Medical Connections Ltd for providing this.
const OpenDCMRootUID = "1.2.826.0.1.3680043.9.7484."

// OpenDCMVersion equals the current (or aimed for) version of the software.
// It is used commonly in creating ImplementationClassUID(0002,0012)
const OpenDCMVersion = "0.1"

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

// GetImplementationUID generates a DICOM implementation UID from OpenDCMRootUID and OpenDCMVersion
// NOTE: OpenDCM Implementation UIDs conform to the format:
// <<ROOT>>.<<VERSION>>.<<InstanceType>>
// Where ROOT = OpenDCMRootUID, VERSION = OpenDCMVersion, InstanceType= (1 for synthetic data, 0 for others)
func GetImplementationUID(synthetic bool) string {
	instanceType := "0"
	if synthetic {
		instanceType = "1"
	}
	return fmt.Sprintf("%s%s.%s", OpenDCMRootUID, OpenDCMVersion, instanceType)
}

// NewRandInstanceUID generates a DICOM random instance UID from OpenDCMRootUID
func NewRandInstanceUID() (string, error) {
	prefix := OpenDCMRootUID
	max := big.Int{}
	max.SetString(strings.Repeat("9", 64-len(prefix)), 10)
	randval, err := rand.Int(rand.Reader, &max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d", prefix, randval), nil
}
