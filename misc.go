package opendcm

import (
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
)

/*
===============================================================================
    Configuration
===============================================================================
*/

// OpenDCMRootUID contains the official designated root UID prefix for OpenDCM
// Issued by Medical Connections Ltd
const OpenDCMRootUID = "1.2.826.0.1.3680043.9.7484."

// OpenDCMVersion equals the current (or aimed for) version of the software.
// It is used commonly in creating ImplementationClassUID(0002,0012)
const OpenDCMVersion = "0.1"

// ExitOnFatalLog specifies whether the application should `os.Exit(1)` on a fatal log message
var ExitOnFatalLog = true

// Config represents the application configuration
type Config struct {
	Version       string
	OpenFileLimit int
	RootUID       string
	LogLevel      string
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

// intFromEnv retrieves `key` from the OS environment.
// if the key is not found, or cannot be expressed as an integer,
// `found` will be false.
func intFromEnv(key string) (val int, found bool) {
	valStr, found := os.LookupEnv(key)
	if !found {
		return
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		found = false
	}
	return
}

func intFromEnvDefault(key string, def int) (val int) {
	val, found := intFromEnv(key)
	if !found {
		val = def
	}
	return
}

func strFromEnv(key string) (string, bool) {
	return os.LookupEnv(key)
}

func strFromEnvDefault(key string, def string) (val string) {
	val, found := strFromEnv(key)
	if !found {
		val = def
	}
	return
}

func boolFromEnv(key string) (val bool, found bool) {
	valStr, found := os.LookupEnv(key)
	if !found {
		return
	}
	val, err := strconv.ParseBool(valStr)
	if err != nil {
		found = false
	}
	return
}

func boolFromEnvDefault(key string, def bool) (val bool) {
	val, found := boolFromEnv(key)
	if !found {
		val = def
	}
	return
}

var config Config

// GetConfig returns the application configuration.
// Will set from environment if not already set.
func GetConfig() Config {
	if !config._set {
		config.OpenFileLimit = intFromEnvDefault("OPENDCM_OPENFILELIMIT", 64)
		config.StrictMode = boolFromEnvDefault("OPENDCM_STRICTMODE", false)
		config.DicomReadBufferSize = intFromEnvDefault("OPENDCM_BUFFERSIZE", 2*1024*1024)
		config.LogLevel = strings.ToLower(strFromEnvDefault("OPENDCM_LOGLEVEL", "info"))
		switch config.LogLevel {
		case "debug", "info", "warn", "error", "fatal", "none", "disabled", "0", "1", "2", "3", "4", "5":
			SetLoggingLevel(config.LogLevel)
		default:
			panic(`Invalid "OPENDCM_LOGLEVEL". Choose from "debug", "info", "warn", "error", "fatal", or "none".`)
		}
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

/*
===============================================================================
    Logging
===============================================================================
*/

const (
	ansiRed     = 31
	ansiGreen   = 32
	ansiYellow  = 33
	ansiMagenta = 35
)

// colourForLevel returns the ANSI colour code for `level`
func colourForLevel(level string) (ansiColour int) {
	switch level {
	case "D":
		ansiColour = ansiMagenta
	case "I":
		ansiColour = ansiGreen
	case "W":
		ansiColour = ansiYellow
	case "E", "F":
		ansiColour = ansiRed
	default:
		ansiColour = 0
	}
	return
}

var (
	infolog  = newLogger("I", os.Stdout)
	debuglog = newLogger("D", os.Stdout)
	warnlog  = newLogger("W", os.Stdout)
	errorlog = newLogger("E", os.Stderr)
	fatallog = newLogger("F", os.Stderr)
)

// awareLogger encapsulates a `log.Logger` to provide awareness of both
// whether the logger is enabled, and whether the output is a character device.
type awareLogger struct {
	*log.Logger
	Enabled           bool
	IsCharacterDevice bool
}

// isCharacterDevice returns whether `f` is a character device (UNIX terminal)
func isCharacterDevice(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		panic(err)
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// Infof calls `infolog.Output` to print to the logger.
// Arguments are handled in the manner of fmt.Printf
func Infof(format string, v ...interface{}) {
	if infolog.Enabled {
		infolog.Output(2, fmt.Sprintf(format, v...))
	}
}

// Info calls `infolog.Output` to print to the logger.
// Arguments are handled in the manner of fmt.Print
func Info(v ...interface{}) {
	if infolog.Enabled {
		infolog.Output(2, fmt.Sprint(v...))
	}
}

// Debugf calls `debuglog.Output` to print to the logger.
// Arguments are handled in the manner of fmt.Printf
func Debugf(format string, v ...interface{}) {
	if debuglog.Enabled {
		debuglog.Output(2, fmt.Sprintf(format, v...))
	}
}

// Debug calls `debuglog.Output` to print to the logger.
// Arguments are handled in the manner of fmt.Print
func Debug(v ...interface{}) {
	if debuglog.Enabled {
		debuglog.Output(2, fmt.Sprint(v...))
	}
}

// Warnf calls `warnlog.Output` to print to the logger.
// Arguments are handled in the manner of fmt.Printf
func Warnf(format string, v ...interface{}) {
	if warnlog.Enabled {
		warnlog.Output(2, fmt.Sprintf(format, v...))
	}
}

// Warn calls `warnlog.Output` to print to the logger.
// Arguments are handled in the manner of fmt.Print
func Warn(v ...interface{}) {
	if warnlog.Enabled {
		warnlog.Output(2, fmt.Sprint(v...))
	}
}

// Errorf calls `errorlog.Output` to print to the logger.
// Arguments are handled in the manner of fmt.Printf
func Errorf(format string, v ...interface{}) {
	if errorlog.Enabled {
		errorlog.Output(2, fmt.Sprintf(format, v...))
	}
}

// Error calls `errorlog.Output` to print to the logger.
// Arguments are handled in the manner of fmt.Print
func Error(v ...interface{}) {
	if errorlog.Enabled {
		errorlog.Output(2, fmt.Sprint(v...))
	}
}

// Fatalf calls `fatallog.Output` to print to the logger.
// ANSI Red colour is added if the output is a character device
// Stack is also printed to `os.Stderr`
// Arguments are handled in the manner of fmt.Printf
func Fatalf(format string, v ...interface{}) {
	if fatallog.Enabled {
		if fatallog.IsCharacterDevice {
			fatallog.Output(2, "\x1b[31m"+fmt.Sprintf(format, v...)+"\x1b[0m")
		} else {
			fatallog.Output(2, fmt.Sprintf(format, v...))
		}
		debug.PrintStack()
	}
	if ExitOnFatalLog {
		os.Exit(1)
	}
}

// FatalfDepth calls `fatallog.Output` to print to the logger.
// `calldepth` is used to allow for better formatting in case of `check()`
// ANSI Red colour is added if the output is a character device
// Stack is also printed to `os.Stderr`
// Arguments are handled in the manner of fmt.Printf
func FatalfDepth(calldepth int, format string, v ...interface{}) {
	if fatallog.Enabled {
		if fatallog.IsCharacterDevice {
			fatallog.Output(calldepth, "\x1b[31m"+fmt.Sprintf(format, v...)+"\x1b[0m")
		} else {
			fatallog.Output(calldepth, fmt.Sprintf(format, v...))
		}
		debug.PrintStack()
	}
	if ExitOnFatalLog {
		os.Exit(1)
	}
}

// Fatal calls `fatallog.Output` to print to the logger.
// ANSI Red colour is added if the output is a character device
// Stack is also printed to `os.Stderr`
// Arguments are handled in the manner of fmt.Print
func Fatal(v ...interface{}) {
	if fatallog.Enabled {
		if fatallog.IsCharacterDevice {
			fatallog.Output(2, "\x1b[31m"+fmt.Sprint(v...)+"\x1b[0m")
		} else {
			fatallog.Output(2, fmt.Sprint(v...))
		}
		debug.PrintStack()
	}
	if ExitOnFatalLog {
		os.Exit(1)
	}
}

// newLogger returns a new `awareLogger` for the given `level`.
// logs to `output`.
func newLogger(level string, output io.Writer) (al awareLogger) {
	al.Enabled = true
	fmtline := "|" + level + "| "
	flags := log.LstdFlags
	if level == "D" || level == "F" {
		flags |= log.Lshortfile
	}
	// avoid colouring output if output is not an input device
	al.IsCharacterDevice = true
	if file, ok := output.(*os.File); ok {
		if !isCharacterDevice(file) {
			al.IsCharacterDevice = false
			al.Logger = log.New(output, fmtline, flags)
		}
	}
	if al.IsCharacterDevice {
		al.Logger = log.New(output, fmt.Sprintf("\x1b[%dm%s\x1b[0m", colourForLevel(level), fmtline), flags)
	}
	return
}

// SetLoggingLevel takes a level string and accordingly enables/disables loggers
// Supported values:
// "debug" / "5": all logging enabled
// "info" / "4":  info and above enabled
// "warn" / "3":  warn and above enabled
// "error" / "2": error and above enabled
// "fatal" / "1": only fatal enabled
// "disabled" / "none" / "off", "0": all loggers disabled
func SetLoggingLevel(level string) {
	switch strings.ToLower(level) {
	case "debug", "5":
		debuglog.Enabled = true
		infolog.Enabled = true
		warnlog.Enabled = true
		errorlog.Enabled = true
		fatallog.Enabled = true
	case "info", "4":
		debuglog.Enabled = false
		infolog.Enabled = true
		warnlog.Enabled = true
		errorlog.Enabled = true
		fatallog.Enabled = true
	case "warn", "3":
		debuglog.Enabled = false
		infolog.Enabled = false
		warnlog.Enabled = true
		errorlog.Enabled = true
		fatallog.Enabled = true
	case "error", "2":
		debuglog.Enabled = false
		infolog.Enabled = false
		warnlog.Enabled = false
		errorlog.Enabled = true
		fatallog.Enabled = true
	case "fatal", "1":
		debuglog.Enabled = false
		infolog.Enabled = false
		warnlog.Enabled = false
		errorlog.Enabled = false
		fatallog.Enabled = true
	case "disabled", "none", "off", "0":
		debuglog.Enabled = false
		infolog.Enabled = false
		warnlog.Enabled = false
		errorlog.Enabled = false
		fatallog.Enabled = false
	}
}

/*
===============================================================================
    Misc
===============================================================================
*/

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
