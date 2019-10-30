// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"crypto/elliptic"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"
	"testing"
)

// TestCertCreationWithHosts creates a certificate pair with extra hosts and
// ensures the extra hosts are present in the generated files.
func TestCertCreationWithHosts(t *testing.T) {
	certFile, err := ioutil.TempFile("", "certfile")
	if err != nil {
		t.Fatalf("Unable to create temp certfile: %s", err)
	}
	certFile.Close()
	defer os.Remove(certFile.Name())

	keyFile, err := ioutil.TempFile("", "keyfile")
	if err != nil {
		t.Fatalf("Unable to create temp keyfile: %s", err)
	}
	keyFile.Close()
	defer os.Remove(keyFile.Name())

	// Generate cert pair with extra hosts.
	hostnames := []string{"hostname1", "hostname2"}
	err = genCertPair(certFile.Name(), keyFile.Name(), hostnames, elliptic.P521())
	if err != nil {
		t.Fatalf("Certificate was not created correctly: %s", err)
	}
	certBytes, err := ioutil.ReadFile(certFile.Name())
	if err != nil {
		t.Fatalf("Unable to read the certfile: %s", err)
	}
	pemCert, _ := pem.Decode(certBytes)
	x509Cert, err := x509.ParseCertificate(pemCert.Bytes)
	if err != nil {
		t.Fatalf("Unable to parse the certificate: %s", err)
	}

	// Ensure the specified extra hosts are present.
	for _, host := range hostnames {
		err := x509Cert.VerifyHostname(host)
		if err != nil {
			t.Fatalf("failed to verify extra host '%s'", host)
		}
	}
}

// TestCertCreationWithOutHosts ensures the creating a certificate pair without
// any hosts works as intended.
func TestCertCreationWithOutHosts(t *testing.T) {
	certFile, err := ioutil.TempFile("", "certfile")
	if err != nil {
		t.Fatalf("Unable to create temp certfile: %s", err)
	}
	certFile.Close()
	defer os.Remove(certFile.Name())

	keyFile, err := ioutil.TempFile("", "keyfile")
	if err != nil {
		t.Fatalf("Unable to create temp keyfile: %s", err)
	}
	keyFile.Close()
	defer os.Remove(keyFile.Name())

	// Generate cert pair with no extra hosts.
	err = genCertPair(certFile.Name(), keyFile.Name(), nil, elliptic.P521())
	if err != nil {
		t.Fatalf("Certificate was not created correctly: %s", err)
	}
}
