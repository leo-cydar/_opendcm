package opendcm

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"testing"
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
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		val, found := intFromEnv("OPENDCM_TEST")
		if !found {
			t.Fatal("OPENDCM_TEST was not found in environment")
		}
		if val != testCase.output {
			t.Fatalf("got %d (!= %d)", val, testCase.output)
		}
	}
	// unset environment variable then try to retrieve
	err := os.Unsetenv("OPENDCM_TEST")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_, found := intFromEnv("OPENDCM_TEST")
	if found {
		t.Fatalf("OPENDCM_TEST was found after unsetting")
	}
}

func TestIntFromEnvDefault(t *testing.T) {
	// unset environment variable then try to retrieve
	err := os.Unsetenv("OPENDCM_TEST")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	val := intFromEnvDefault("OPENDCM_TEST", 9000)
	if val != 9000 {
		t.Fatalf("got %d (!= 9000)", val)
	}
	os.Setenv("OPENDCM_TEST", "42")
	val = intFromEnvDefault("OPENDCM_TEST", 9000)
	if val != 42 {
		t.Fatalf("got %d (!= 42)", val)
	}
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
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		val, found := strFromEnv("OPENDCM_TEST")
		if !found {
			t.Fatal("OPENDCM_TEST was not found in environment")
		}
		if val != testCase.output {
			t.Fatalf("got %s (!= %s)", val, testCase.output)
		}
	}
	// unset environment variable then try to retrieve
	err := os.Unsetenv("OPENDCM_TEST")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_, found := strFromEnv("OPENDCM_TEST")
	if found {
		t.Fatalf("OPENDCM_TEST was found after unsetting")
	}
}

func TestStrFromEnvDefault(t *testing.T) {
	// unset environment variable then try to retrieve
	err := os.Unsetenv("OPENDCM_TEST")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	val := strFromEnvDefault("OPENDCM_TEST", "ascii")
	if val != "ascii" {
		t.Fatalf(`got "%s" (!= "ascii")`, val)
	}
	os.Setenv("OPENDCM_TEST", "42")
	val = strFromEnvDefault("OPENDCM_TEST", "ascii")
	if val != "42" {
		t.Fatalf(`got "%s" (!= "42")`, val)
	}
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
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		val, found := boolFromEnv("OPENDCM_TEST")
		if !found {
			t.Fatal("OPENDCM_TEST was not found in environment")
		}
		if val != testCase.output {
			t.Fatalf("got %t (!= %t)", val, testCase.output)
		}
	}
	// unset environment variable then try to retrieve
	err := os.Unsetenv("OPENDCM_TEST")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_, found := boolFromEnv("OPENDCM_TEST")
	if found {
		t.Fatalf("OPENDCM_TEST was found after unsetting")
	}
}

func TestBoolFromEnvDefault(t *testing.T) {
	// unset environment variable then try to retrieve
	err := os.Unsetenv("OPENDCM_TEST")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	val := boolFromEnvDefault("OPENDCM_TEST", true)
	if val != true {
		t.Fatalf(`got %t (!= true)`, val)
	}
	os.Setenv("OPENDCM_TEST", "false")
	val = boolFromEnvDefault("OPENDCM_TEST", true)
	if val != false {
		t.Fatalf(`got %t (!= false)`, val)
	}
}

func TestGetConfig(t *testing.T) {
	os.Setenv("OPENDCM_OPENFILELIMIT", "100")
	config._set = false
	cfg := GetConfig()
	if cfg.OpenFileLimit != 100 {
		t.Fatalf("OpenFileLimit = %d (!= 100)", cfg.OpenFileLimit)
	}
}
func TestOverrideConfig(t *testing.T) {
	newcfg := Config{OpenFileLimit: 256}
	OverrideConfig(newcfg)
	cfg := GetConfig()
	if cfg.OpenFileLimit != 256 {
		t.Fatalf("OpenFileLimit = %d (!= 256)", cfg.OpenFileLimit)
	}
}

func TestConcurrentlyWalkDir(t *testing.T) {
	files := make([]string, 0)
	// make temporary directory for tests
	tmpdir, err := ioutil.TempDir("", "opendcm")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// be sure to remove up dir afterwards
	defer os.RemoveAll(tmpdir)
	for i := 0; i < 10; i++ {
		_, err = ioutil.TempFile(tmpdir, strconv.Itoa(i))
		if err != nil {
			t.Fatalf("error: %v", err)
		}
	}
	ConcurrentlyWalkDir(tmpdir, func(path string) {
		files = append(files, path)
	})
	if len(files) == 0 {
		t.Fatalf("did not report any files")
	}
}

func TestGetImplementationUID(t *testing.T) {
	t.Parallel()
	uid := GetImplementationUID(true)
	expected := fmt.Sprintf("%s%s.1", OpenDCMRootUID, OpenDCMVersion)
	if uid != expected {
		t.Fatalf(`got "%s" (!= "%s")`, uid, expected)
	}
	uid = GetImplementationUID(false)
	expected = fmt.Sprintf("%s%s.0", OpenDCMRootUID, OpenDCMVersion)
	if uid != expected {
		t.Fatalf(`got "%s" (!= "%s")`, uid, expected)
	}
}

func TestNewRandInstanceUID(t *testing.T) {
	t.Parallel()
	uid, err := NewRandInstanceUID()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	index := strings.Index(uid, OpenDCMRootUID)
	if index != 0 {
		t.Fatalf("uid did not start with OpenDCMRootUID")
	}

}
