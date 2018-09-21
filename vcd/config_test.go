package vcd

// This module provides initialization routines for the whole suite

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"
)

type StringMap map[string]interface{}

// Structure to get info from a config json file that the user specifies
type TestConfig struct {
	Provider struct {
		User                     string `json:"user"`
		Password                 string `json:"password"`
		Url                      string `json:"url"`
		SysOrg                   string `json:"sysOrg"`
		AllowInsecure            bool   `json:"allowInsecure"`
		TerraformAcceptanceTests bool   `json:"tfAcceptanceTests"`
	} `json:"provider"`
	VCD struct {
		Org     string `json:"org"`
		Vdc     string `json:"vdc"`
		Catalog struct {
			Name        string `json:"name,omitempty"`
			Catalogitem string `json:"catalogItem,omitempty"`
		} `json:"catalog"`
	} `json:"vcd"`
	Networking struct {
		ExternalIp   string `json:"externalIp,omitempty"`
		InternalIp   string `json:"internalIp,omitempty"`
		EdgeGateway  string `json:"edgeGateway,omitempty"`
		SharedSecret string `json:"sharedSecret"`
		Local        struct {
			LocalIp            string `json:"localIp"`
			LocalSubnetGateway string `json:"localSubnetGw"`
		} `json:"local"`
		Peer struct {
			PeerIp            string `json:"peerIp"`
			PeerSubnetGateway string `json:"peerSubnetGw"`
		} `json:"peer"`
	} `json:"networking"`
	/*
		// FOR FUTURE USE
		Logging struct {
			Enabled         bool   `json:"enabled,omitempty"`
			LogFileName     string `json:"logFileName,omitempty"`
			LogHttpRequest  bool   `json:"logHttpRequest,omitempty"`
			LogHttpResponse bool   `json:"logHttpResponse,omitempty"`
			VerboseCleanup  bool   `json:"verboseCleanup,omitempty"`
		} `json:"logging"`
	*/
}

// This is a global variable shared across all tests. It contains
// the information from the configuration file.
var testConfig TestConfig

// Checks if a directory exists
func dirExists(filename string) bool {
	f, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	filemode := f.Mode()
	return filemode.IsDir()
}

// Fills a template with data provided as a StringMap
// Returns the text of a ready-to-use Terraform directive.
// It also saves the filled template to a file, for further troubleshooting.
func templateFill(tmpl string, data StringMap) string {

	// Gets the name of the function containing the template
	caller := callFuncName()
	// Removes the full path to the function, leaving only package + function name
	caller = filepath.Base(caller)

	// If the call comes from a function that does not have a good descriptive name,
	// (for example when it's an auxiliary function that builds the template but does not
	// run the test) users can add the function name in the data, and it will be used instead of
	// the caller name.
	funcName, ok := data["FuncName"]
	if ok {
		caller = funcName.(string)
	}

	// Creates a template. The template gets the same name of the calling function, to generate a better
	// error message in case of failure
	unfilledTemplate := template.Must(template.New(caller).Parse(tmpl))
	buf := &bytes.Buffer{}

	// If an error occurs, returns an empty string
	if err := unfilledTemplate.Execute(buf, data); err != nil {
		return ""
	}
	// Writes the populated template into a directory named "test-artifacts"
	// These templates will help investigate failed tests using Terraform
	// Writing is enabled by default. It can be skipped using an environment variable.
	TemplateWriting := true
	if os.Getenv("VCD_SKIP_TEMPLATE_WRITING") != "" {
		TemplateWriting = false
	}
	if TemplateWriting {
		testArtifacts := "test-artifacts"
		if !dirExists(testArtifacts) {
			err := os.Mkdir(testArtifacts, 0755)
			if err != nil {
				panic(fmt.Errorf("Error creating directory %s: %s", testArtifacts, err))
			}
		}
		templateFile := path.Join(testArtifacts, caller)
		file, err := os.Create(templateFile)
		if err != nil {
			panic(fmt.Errorf("Error creating file %s: %s", templateFile, err))
		}
		writer := bufio.NewWriter(file)
		count, err := writer.Write(buf.Bytes())
		if err != nil || count == 0 {
			panic(fmt.Errorf("Error writing to file %s. Reported %d bytes written. %s", templateFile, count, err))
		}
		writer.Flush()
		file.Close()
	}
	// Returns the populated template
	return buf.String()
}

// Returns the name of the function that called the
// current function.
func callFuncName() string {
	fpcs := make([]uintptr, 1)
	n := runtime.Callers(3, fpcs)
	if n > 0 {
		fun := runtime.FuncForPC(fpcs[0] - 1)
		if fun != nil {
			return fun.Name()
		}
	}
	return ""
}

// Reads the configuration file and returns its contents as a TestConfig structure
// The default file is called vcd_test_config.json in the same directory where
// the test files are.
// Users may define a file in a different location using the environment variable
// VCD_CONFIG
// This function doesn't return an error. It panics immediately because its failure
// will prevent the whole test suite from running
func getConfigStruct() TestConfig {
	// First, we see whether the user has indicated a custom configuration file
	// from a non-standard location
	config := os.Getenv("VCD_CONFIG")
	var config_struct TestConfig

	// If there was no custom file, we look for the default one
	if config == "" {
		// Finds the current directory, through the path of this running test
		_, current_filename, _, _ := runtime.Caller(0)
		current_directory := filepath.Dir(current_filename)
		config = current_directory + "/vcd_test_config.json"
	}
	// Looks if the configuration file exists before attempting to read it
	_, err := os.Stat(config)
	if os.IsNotExist(err) {
		panic(fmt.Errorf("Configuration file %s not found: %s", config, err))
	}
	jsonFile, err := ioutil.ReadFile(config)
	if err != nil {
		panic(fmt.Errorf("could not read config file %s: %v", config, err))
	}
	err = json.Unmarshal(jsonFile, &config_struct)
	if err != nil {
		panic(fmt.Errorf("could not unmarshal json file: %v", err))
	}

	// Reading the configuration file was successful.
	// Now we fill the environment variables that the library is using for its own initialization.
	if config_struct.Provider.TerraformAcceptanceTests {
		// defined in vendor/github.com/hashicorp/terraform/helper/resource/testing.go
		os.Setenv("TF_ACC", "1")
	}
	// The following variables are used in ./provider.go
	// TODO: eliminate also these variables and replace them with a proper structure
	os.Setenv("VCD_USER", config_struct.Provider.User)
	os.Setenv("VCD_PASSWORD", config_struct.Provider.Password)
	os.Setenv("VCD_URL", config_struct.Provider.Url)
	os.Setenv("VCD_ORG", config_struct.Provider.SysOrg)
	if config_struct.Provider.AllowInsecure {
		os.Setenv("VCD_ALLOW_UNVERIFIED_SSL", "1")
	}
	return config_struct
}

// This function is called before any other test
func TestMain(m *testing.M) {
	// Fills the configuration variable: it will be available to all tests,
	// or the whole suite will fail if it is not found.
	if os.Getenv("VCD_SHORT_TEST") == "" {
		testConfig = getConfigStruct()
	}

	// Runs all test functions
	exitCode := m.Run()

	// TODO: cleanup leftovers
	os.Exit(exitCode)
}