package main

import (
	"flag"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

// in order to test command line arguments and environment variables
// you will need to append the flags to the os.Args variable like so
// os.Args = append(os.Args, "--altdnsnames=\"hostname1,hostname2\"")
// For environment variables you can use the
// os.Setenv("DCRD_ALT_DNSNAMES", "hostname1,hostname2") to set the variable
// before loadConfig() is called
// These args and env variables will then get parsed by loadConfig()

func setup() {
	// Temp config file is used to ensure there are no external influences
	// from previously set env variables or default config files.
	file, _ := ioutil.TempFile("", "dcrd_test_file.cfg")
	defer os.Remove(file.Name())
	// Parse the -test.* flags before removing them from the command line
	// arguments list, which we do to allow go-flags to succeed.
	flag.Parse()
	os.Args = os.Args[:1]
	// Run the tests now that the testing package flags have been parsed.
}

func TestLoadConfig(t *testing.T) {
	_, _, err := loadConfig()
	if err != nil {
		t.Errorf("Failed to load dcrd config: %s\n", err.Error())
	}
}

func TestDefaultAltDNSNames(t *testing.T) {
	cfg, _, _ := loadConfig()
	if len(cfg.AltDNSNames) != 0 {
		t.Errorf("Invalid default value for altdnsnames: %s\n", cfg.AltDNSNames)
	}
}

func TestAltDNSNamesWithEnv(t *testing.T) {
	os.Setenv("DCRD_ALT_DNSNAMES", "hostname1,hostname2")
	cfg, _, _ := loadConfig()
	hostnames := strings.Join(cfg.AltDNSNames, ",")
	if hostnames != "hostname1,hostname2" {
		t.Errorf("altDNSNames should be %s but was %s", "hostname1,hostname2", hostnames)
	}
}

func TestAltDNSNamesWithArg(t *testing.T) {
	setup()
	old := os.Args
	os.Args = append(os.Args, "--altdnsnames=\"hostname1,hostname2\"")
	cfg, _, _ := loadConfig()
	hostnames := strings.Join(cfg.AltDNSNames, ",")
	if hostnames != "hostname1,hostname2" {
		t.Errorf("altDNSNames should be %s but was %s", "hostname1,hostname2", hostnames)
	}
	os.Args = old
}
