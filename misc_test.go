package opendcm

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntFromEnv(t *testing.T) {
	testCases := []struct {
		input  string
		output int
	}{
		{input: "100", output: 100},
		{input: "-100", output: -100},
	}
	for _, testCase := range testCases {
		err := os.Setenv("OPENDCM_TEST", testCase.input)
		assert.NoError(t, err, testCase)

		val, found := intFromEnv("OPENDCM_TEST")
		assert.True(t, found, testCase)
		assert.Equal(t, testCase.output, val)
	}
	// unset environment variable then try to retrieve
	err := os.Unsetenv("OPENDCM_TEST")
	assert.NoError(t, err)
	_, found := intFromEnv("OPENDCM_TEST")
	assert.False(t, found)
}

func TestIntFromEnvDefault(t *testing.T) {
	// unset environment variable then try to retrieve
	err := os.Unsetenv("OPENDCM_TEST")
	assert.NoError(t, err)

	val := intFromEnvDefault("OPENDCM_TEST", 9000)
	assert.Equal(t, 9000, val)
	err = os.Setenv("OPENDCM_TEST", "42")
	assert.NoError(t, err)
	val = intFromEnvDefault("OPENDCM_TEST", 9000)
	assert.Equal(t, 42, val)
}

func TestStrFromEnv(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{input: "ascii", output: "ascii"},
		{input: "-100", output: "-100"},
		{input: "中文", output: "中文"}, // non-ascii
	}
	for _, testCase := range testCases {
		err := os.Setenv("OPENDCM_TEST", testCase.input)
		assert.NoError(t, err, testCase)
		val, found := strFromEnv("OPENDCM_TEST")
		assert.True(t, found, testCase)
		assert.Equal(t, testCase.output, val, testCase)
	}
	// unset environment variable then try to retrieve
	err := os.Unsetenv("OPENDCM_TEST")
	assert.NoError(t, err)
	_, found := strFromEnv("OPENDCM_TEST")
	assert.False(t, found)
}

func TestStrFromEnvDefault(t *testing.T) {
	// unset environment variable then try to retrieve
	err := os.Unsetenv("OPENDCM_TEST")
	assert.NoError(t, err)
	val := strFromEnvDefault("OPENDCM_TEST", "ascii")
	assert.Equal(t, "ascii", val)
	os.Setenv("OPENDCM_TEST", "42")
	val = strFromEnvDefault("OPENDCM_TEST", "ascii")
	assert.Equal(t, "42", val)
}

func TestBoolFromEnv(t *testing.T) {
	testCases := []struct {
		input  string
		output bool
	}{
		{input: "true", output: true},
		{input: "1", output: true},
		{input: "false", output: false},
		{input: "0", output: false},
	}
	for _, testCase := range testCases {
		err := os.Setenv("OPENDCM_TEST", testCase.input)
		assert.NoError(t, err, testCase)
		val, found := boolFromEnv("OPENDCM_TEST")
		assert.True(t, found, testCase)
		assert.Equal(t, testCase.output, val, testCase)
	}
	// unset environment variable then try to retrieve
	err := os.Unsetenv("OPENDCM_TEST")
	assert.NoError(t, err)
	_, found := boolFromEnv("OPENDCM_TEST")
	assert.False(t, found)
}

func TestBoolFromEnvDefault(t *testing.T) {
	// unset environment variable then try to retrieve
	err := os.Unsetenv("OPENDCM_TEST")
	assert.NoError(t, err)
	val := boolFromEnvDefault("OPENDCM_TEST", true)
	assert.True(t, val)
	os.Setenv("OPENDCM_TEST", "false")
	val = boolFromEnvDefault("OPENDCM_TEST", true)
	assert.False(t, val)
}

func TestInitialiseConfig(t *testing.T) {
	os.Setenv("OPENDCM_OPENFILELIMIT", "100")
	config._set = false
	initialiseConfig()
	assert.Equal(t, 100, config.OpenFileLimit)
}
func TestOverrideConfig(t *testing.T) {
	newcfg := Config{OpenFileLimit: 256}
	OverrideConfig(newcfg)
	assert.Equal(t, 256, config.OpenFileLimit)
}

func TestConcurrentlyWalkDir(t *testing.T) {
	files := make([]string, 0)
	// make temporary directory for tests
	tmpdir, err := ioutil.TempDir("", "opendcm")
	assert.NoError(t, err)
	// be sure to remove up dir afterwards
	defer os.RemoveAll(tmpdir)
	for i := 0; i < 10; i++ {
		_, err = ioutil.TempFile(tmpdir, strconv.Itoa(i))
		assert.NoError(t, err)
	}
	ConcurrentlyWalkDir(tmpdir, func(path string) {
		files = append(files, path)
	})
	assert.NotEqual(t, 0, files)
}

func TestGetImplementationUID(t *testing.T) {
	t.Parallel()
	uid := GetImplementationUID(true)
	expected := fmt.Sprintf("%s%s.1", OpenDCMRootUID, OpenDCMVersion)
	assert.Equal(t, expected, uid)
	uid = GetImplementationUID(false)
	expected = fmt.Sprintf("%s%s.0", OpenDCMRootUID, OpenDCMVersion)
	assert.Equal(t, expected, uid)
}

func TestNewRandInstanceUID(t *testing.T) {
	t.Parallel()
	uid, err := NewRandInstanceUID()
	assert.NoError(t, err)
	index := strings.Index(uid, OpenDCMRootUID)
	assert.Equal(t, 0, index)
}

func TestColourForLevel(t *testing.T) {
	t.Parallel()
	assert.Equal(t, ansiMagenta, colourForLevel("D"))
	assert.Equal(t, ansiGreen, colourForLevel("I"))
	assert.Equal(t, ansiYellow, colourForLevel("W"))
	assert.Equal(t, ansiRed, colourForLevel("E"))
	assert.Equal(t, ansiRed, colourForLevel("F"))
	assert.Equal(t, 0, colourForLevel("T"))
}

func getLogEntries(buf *bytes.Buffer) []string {
	logEntriesBytes := bytes.Split(buf.Bytes(), []byte("\n"))
	logEntries := make([]string, 0)
	for _, entry := range logEntriesBytes {
		if len(entry) == 0 || entry[0] == []byte("\r")[0] {
			continue
		}
		logEntries = append(logEntries, string(entry))
	}
	return logEntries
}

func TestDebugLoggerEnabled(t *testing.T) {
	SetLoggingLevel("debug")
	buf := bytes.NewBuffer(make([]byte, 0))
	debuglog.SetOutput(buf)
	Debugf("%s", "message")
	Debug("message")
	logEntries := getLogEntries(buf)
	assert.Len(t, logEntries, 2)
}

func TestDebugLoggerDisabled(t *testing.T) {
	SetLoggingLevel("info")
	buf := bytes.NewBuffer(make([]byte, 0))
	debuglog.SetOutput(buf)
	Debugf("%s", "message")
	Debug("message")
	logEntries := getLogEntries(buf)
	assert.Len(t, logEntries, 0)
}

func TestInfoLoggerEnabled(t *testing.T) {
	SetLoggingLevel("info")
	buf := bytes.NewBuffer(make([]byte, 0))
	infolog.SetOutput(buf)
	Infof("%s", "message")
	Info("message")
	logEntries := getLogEntries(buf)
	assert.Len(t, logEntries, 2)
}

func TestInfoLoggerDisabled(t *testing.T) {
	SetLoggingLevel("warn")
	buf := bytes.NewBuffer(make([]byte, 0))
	infolog.SetOutput(buf)
	Infof("%s", "message")
	Info("message")
	logEntries := getLogEntries(buf)
	assert.Len(t, logEntries, 0)
}

func TestWarnLoggerEnabled(t *testing.T) {
	SetLoggingLevel("warn")
	buf := bytes.NewBuffer(make([]byte, 0))
	warnlog.SetOutput(buf)
	Warnf("%s", "message")
	Warn("message")
	logEntries := getLogEntries(buf)
	assert.Len(t, logEntries, 2)
}

func TestWarnLoggerDisabled(t *testing.T) {
	SetLoggingLevel("error")
	buf := bytes.NewBuffer(make([]byte, 0))
	warnlog.SetOutput(buf)
	Warnf("%s", "message")
	Warn("message")
	logEntries := getLogEntries(buf)
	assert.Len(t, logEntries, 0)
}
func TestErrorLoggerEnabled(t *testing.T) {
	SetLoggingLevel("error")
	buf := bytes.NewBuffer(make([]byte, 0))
	errorlog.SetOutput(buf)
	Errorf("%s", "message")
	Error("message")
	logEntries := getLogEntries(buf)
	assert.Len(t, logEntries, 2)
}

func TestErrorLoggerDisabled(t *testing.T) {
	SetLoggingLevel("fatal")
	buf := bytes.NewBuffer(make([]byte, 0))
	errorlog.SetOutput(buf)
	Errorf("%s", "message")
	Error("message")
	logEntries := getLogEntries(buf)
	assert.Len(t, logEntries, 0)
}

func TestFatalLoggerEnabled(t *testing.T) {
	SetLoggingLevel("fatal")
	ExitOnFatalLog = false // important
	buf := bytes.NewBuffer(make([]byte, 0))
	fatallog.SetOutput(buf)
	Fatalf("%s", "message")
	Fatal("message")
	FatalfDepth(1, "%s", "message with depth")
	logEntries := getLogEntries(buf)
	assert.True(t, len(logEntries) > 10) // including stack
}

func TestFatalLoggerDisabled(t *testing.T) {
	SetLoggingLevel("none")
	ExitOnFatalLog = false // important
	buf := bytes.NewBuffer(make([]byte, 0))
	fatallog.SetOutput(buf)
	Fatalf("%s", "message")
	Fatal("message")
	FatalfDepth(1, "%s", "message with depth")
	logEntries := getLogEntries(buf)
	assert.Len(t, logEntries, 0)
}
