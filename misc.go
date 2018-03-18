package opendcm

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// OpenDCMRootUID contains the official designated root UID prefix for OpenDCM
// Issued by Medical Connections Ltd
const OpenDCMRootUID = "1.2.826.0.1.3680043.9.7484."

// OpenDCMVersion equals the current (or aimed for) version of the software.
// It is used commonly in creating ImplementationClassUID(0002,0012)
const OpenDCMVersion = "0.1"

// Config represents the application configuration
type Config struct {
	Version       string
	OpenFileLimit int
	RootUID       string
	/* By enabling `StrictMode`, the parser will reject DICOM inputs which either:
	   - TODO: Contain an element with a value length exceeding the maximum allowed for its VR
	   - Contain an element with a value length exceeding the remaining file size. For example incomplete Pixel Data.
	*/
	StrictMode bool

	// DicomReadBufferSize is the number of bytes to be buffered from disk when parsing dicoms
	DicomReadBufferSize int

	// do not access / write `_set`. It is used internally.
	_set bool
}

func intFromEnv(key string) (int, bool) {
	val, found := os.LookupEnv(key)
	if !found {
		return -1, false
	}
	valInt, err := strconv.Atoi(val)
	if err != nil {
		return -1, false
	}
	return valInt, true
}

func intFromEnvDefault(key string, def int) int {
	val, found := intFromEnv(key)
	if found {
		return val
	}
	return def
}

func strFromEnv(key string) (string, bool) {
	return os.LookupEnv(key)
}

func strFromEnvDefault(key string, def string) string {
	val, found := strFromEnv(key)
	if found {
		return val
	}
	return def
}

func boolFromEnv(key string) (bool, bool) {
	val, found := os.LookupEnv(key)
	if !found {
		return false, false
	}
	valBool, err := strconv.ParseBool(val)
	if err != nil {
		return false, false
	}
	return valBool, true
}

func boolFromEnvDefault(key string, def bool) bool {
	val, found := boolFromEnv(key)
	if found {
		return val
	}
	return def
}

var config Config

// GetConfig returns the application configuration.
// Will set from environment if not already set.
func GetConfig() Config {
	if !config._set {
		config.OpenFileLimit = intFromEnvDefault("OPENDCM_OPENFILELIMIT", 64)
		config.StrictMode = boolFromEnvDefault("OPENDCM_STRICTMODE", false)
		config.DicomReadBufferSize = intFromEnvDefault("OPENDCM_BUFFERSIZE", 2*1024*1024)
		config._set = true
	}
	return config
}

// OverrideConfig overrides the configuration parsed from environment with the one provided
func OverrideConfig(newconfig Config) {
	if !newconfig._set { // to prevent being reverted with subsequent calls to `GetConfig`
		newconfig._set = true
	}
	config = newconfig
}

// ConcurrentlyWalkDir recursively traverses a directory and calls `onFile` for each found file inside a goroutine.
func ConcurrentlyWalkDir(dirPath string, onFile func(file string)) error {
	guard := make(chan bool, GetConfig().OpenFileLimit) // limits number of concurrently open files
	var files []string
	wg := sync.WaitGroup{}

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
		wg.Add(1)
		guard <- true // would block if guard channel is already filled
		go func(path string) {
			onFile(path)
			<-guard

			wg.Done()
		}(filePath)
	}
	wg.Wait()
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
