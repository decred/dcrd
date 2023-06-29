// Copyright (c) 2018-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	flag.Parse()

	// Trim -test.* flags from the command line arguments list to allow
	// go-flags tests to succeed.
	os.Args = append([]string{os.Args[0]}, flag.Args()...)

	os.Exit(m.Run())
}

// In order to test command line arguments and environment variables, append
// the flags to the os.Args variable like so:
//   os.Args = append(os.Args, "--altdnsnames=\"hostname1,hostname2\"")
//
// For environment variables, use the following to set the variable before the
// func that loads the configuration is called:
//   os.Setenv("DCRD_ALT_DNSNAMES", "hostname1,hostname2")
//
// These args and env variables will then get parsed during configuration load.

// TestLoadConfig ensures that basic configuration loading succeeds.
func TestLoadConfig(t *testing.T) {
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	_, _, err := loadConfig(appName)
	if err != nil {
		t.Fatalf("Failed to load dcrd config: %s", err)
	}
}

// TestDefaultAltDNSNames ensures that there are no additional hostnames added
// by default during the configuration load phase.
func TestDefaultAltDNSNames(t *testing.T) {
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	cfg, _, err := loadConfig(appName)
	if err != nil {
		t.Fatalf("Failed to load dcrd config: %s", err)
	}
	if len(cfg.AltDNSNames) != 0 {
		t.Fatalf("Invalid default value for altdnsnames: %s", cfg.AltDNSNames)
	}
}

// TestAltDNSNamesWithEnv ensures the DCRD_ALT_DNSNAMES environment variable is
// parsed into a slice of additional hostnames as intended.
func TestAltDNSNamesWithEnv(t *testing.T) {
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	os.Setenv("DCRD_ALT_DNSNAMES", "hostname1,hostname2")
	cfg, _, err := loadConfig(appName)
	if err != nil {
		t.Fatalf("Failed to load dcrd config: %s", err)
	}
	hostnames := strings.Join(cfg.AltDNSNames, ",")
	if hostnames != "hostname1,hostname2" {
		t.Fatalf("altDNSNames should be %s but was %s", "hostname1,hostname2",
			hostnames)
	}
}

// TestAltDNSNamesWithArg ensures the altdnsnames configuration option parses
// additional hostnames into a slice of hostnames as intended.
func TestAltDNSNamesWithArg(t *testing.T) {
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	old := os.Args
	os.Args = append(os.Args, "--altdnsnames=\"hostname1,hostname2\"")
	cfg, _, err := loadConfig(appName)
	if err != nil {
		t.Fatalf("Failed to load dcrd config: %s", err)
	}
	hostnames := strings.Join(cfg.AltDNSNames, ",")
	if hostnames != "hostname1,hostname2" {
		t.Fatalf("altDNSNames should be %s but was %s", "hostname1,hostname2",
			hostnames)
	}
	os.Args = old
}
