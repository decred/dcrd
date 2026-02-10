// Copyright (c) 2020-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"
	"unicode"

	"github.com/decred/dcrd/crypto/rand"
	flags "github.com/jessevdk/go-flags"
	"golang.org/x/net/idna"
)

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func usage(parser *flags.Parser) {
	parser.WriteHelp(os.Stderr)
	os.Exit(2)
}

type config struct {
	CA    string   `short:"C" description:"sign generated certificate using CA cert (requires -K)"`
	CAKey string   `short:"K" description:"key of CA certificate"`
	Hosts []string `short:"H" description:"hostname or IP certificate is valid for; may be specified multiple times"`
	Local bool     `short:"L" description:"append localhost, 127.0.0.1, and ::1 to hosts if not already specified"`
	Signs bool     `short:"S" description:"allow certificate to sign leaf certificates"`
	Org   string   `short:"o" description:"organization"`
	Algo  string   `short:"a" description:"key algorithm (one of: P-256, P-384, P-521, Ed25519, RSA4096)"`
	Years int      `short:"y" description:"years certificate is valid for"`
	Force bool     `short:"f" description:"overwrite existing certs/keys"`
}

func main() {
	cfg := config{
		Algo:  "P-256",
		Years: 10,
		Org:   "gencerts",
	}
	parser := flags.NewParser(&cfg, flags.Default)
	parser.Usage = "[OPTIONS] cert key"
	args, err := parser.Parse()
	if err != nil {
		var e *flags.Error
		if errors.As(err, &e) {
			if e.Type != flags.ErrHelp {
				os.Exit(1)
			}
			os.Exit(0)
		}
		os.Exit(1)
	}

	if len(args) != 2 {
		usage(parser)
	}
	certname, keyname := args[0], args[1]

	var keygen func() (pub, priv any)
	switch cfg.Algo {
	case "P-256":
		keygen = ecKeyGen(elliptic.P256())
	case "P-384":
		keygen = ecKeyGen(elliptic.P384())
	case "P-521":
		keygen = ecKeyGen(elliptic.P521())
	case "Ed25519":
		keygen = ed25519KeyGen
	case "RSA4096":
		keygen = rsaKeyGen(4096)
	default:
		fmt.Fprintf(os.Stderr, "unknown algorithm %q\n", cfg.Algo)
		usage(parser)
	}

	if cfg.CA == "" != (cfg.CAKey == "") {
		fatalf("-C and -K must be used together\n")
	}

	if cfg.Local {
		var localhost, v4Loopback, v6Loopback bool
		for _, h := range cfg.Hosts {
			switch h {
			case "localhost":
				localhost = true
			case "127.0.0.1":
				v4Loopback = true
			case "::1":
				v6Loopback = true
			}
		}
		if !localhost {
			cfg.Hosts = append(cfg.Hosts, "localhost")
		}
		if !v4Loopback {
			cfg.Hosts = append(cfg.Hosts, "127.0.0.1")
		}
		if !v6Loopback {
			cfg.Hosts = append(cfg.Hosts, "::1")
		}
	}

	var cert *certWithPEM
	pub, priv := keygen()
	keyBlock, err := marshalPrivateKey(priv)
	if err != nil {
		fatalf("%s\n", err)
	}
	keyUsage := x509.KeyUsageDigitalSignature
	if cfg.Signs {
		keyUsage |= x509.KeyUsageCertSign
	}
	if cfg.CA == "" {
		cert, err = generateAuthority(pub, priv, cfg.Hosts, cfg.Org,
			cfg.Years, keyUsage)
		if err != nil {
			fatalf("generate certificate authority: %v\n", err)
		}
	} else {
		var ca *x509.Certificate
		var caPriv any
		tlsCert, err := tls.LoadX509KeyPair(cfg.CA, cfg.CAKey)
		if err != nil {
			fatalf("open CA keypair: %s\n", err)
		}
		// will never error, as this was already parsed by LoadX509KeyPair
		ca, _ = x509.ParseCertificate(tlsCert.Certificate[0])
		caPriv = tlsCert.PrivateKey

		cert, err = createIssuedCert(pub, caPriv, ca,
			cfg.Hosts, cfg.Org, cfg.Years, keyUsage)
		if err != nil {
			fatalf("issue certificate: %v\n", err)
		}
	}

	if !cfg.Force && fileExists(certname) {
		fatalf("certificate file %q already exists\n", certname)
	}
	if !cfg.Force && fileExists(keyname) {
		fatalf("key file %q already exists\n", keyname)
	}

	if err = os.WriteFile(certname, cert.PEMBlock, 0644); err != nil {
		fatalf("cannot write cert: %v\n", err)
	}
	if err = os.WriteFile(keyname, keyBlock, 0600); err != nil {
		os.Remove(certname)
		fatalf("cannot write key: %v\n", err)
	}
}

func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func ed25519KeyGen() (pub, priv any) {
	seed := make([]byte, ed25519.SeedSize)
	rand.Read(seed)
	key := ed25519.NewKeyFromSeed(seed)
	return key.Public(), key
}

func ecKeyGen(curve elliptic.Curve) func() (pub, priv any) {
	return func() (pub, priv any) {
		var key *ecdsa.PrivateKey
		key, err := ecdsa.GenerateKey(curve, rand.Reader())
		if err != nil {
			fatalf("generate random EC key: %v\n", err)
		}
		return key.Public(), key
	}
}

func rsaKeyGen(bits int) func() (pub, priv any) {
	return func() (pub, priv any) {
		var key *rsa.PrivateKey
		key, err := rsa.GenerateKey(rand.Reader(), bits)
		if err != nil {
			fatalf("generate random RSA key: %v\n", err)
		}
		return key.Public(), key
	}
}

func randomX509SerialNumber() *big.Int {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber := rand.BigInt(serialNumberLimit)
	return serialNumber
}

// End of ASN.1 time.
var endOfTime = time.Date(2049, 12, 31, 23, 59, 59, 0, time.UTC)

type certWithPEM struct {
	PEMBlock []byte
	Cert     *x509.Certificate
}

func isASCII(s string) bool {
	for _, c := range s {
		if c > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func newTemplate(hosts []string, org string, validUntil time.Time) (*x509.Certificate, error) {
	now := time.Now()
	if validUntil.After(endOfTime) {
		validUntil = endOfTime
	}
	if validUntil.Before(now) {
		return nil, fmt.Errorf("valid until date %v already elapsed", validUntil)
	}
	serialNumber := randomX509SerialNumber()
	cn := org
	if len(hosts) > 0 {
		cn = hosts[0]
	}

	var hostnames []string
	var ips []net.IP
	for _, h := range hosts {
		if !isASCII(h) {
			var err error
			h, err = idna.ToASCII(h)
			if err != nil {
				return nil, err
			}
		}
		if ip := net.ParseIP(h); ip != nil {
			ips = append(ips, ip)
			continue
		}
		hostnames = append(hostnames, h)
	}

	if !isASCII(cn) {
		var err error
		cn, err = idna.ToASCII(cn)
		if err != nil {
			return nil, err
		}
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{org},
		},
		NotBefore:   now.Add(-time.Hour * 24),
		NotAfter:    validUntil,
		DNSNames:    hostnames,
		IPAddresses: ips,
	}
	return template, nil
}

func generateAuthority(pub, priv any, hosts []string, org string,
	years int, keyUsage x509.KeyUsage) (*certWithPEM, error) {

	validUntil := time.Now().Add(time.Hour * 24 * 365 * time.Duration(years))
	template, err := newTemplate(hosts, org, validUntil)
	if err != nil {
		return nil, err
	}
	template.KeyUsage = keyUsage
	template.BasicConstraintsValid = true
	template.IsCA = true

	cert, err := x509.CreateCertificate(rand.Reader(), template, template, pub, priv)
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	err = pem.Encode(buf, &pem.Block{Type: "CERTIFICATE", Bytes: cert})
	if err != nil {
		return nil, fmt.Errorf("failed to encode certificate: %w", err)
	}
	pemBlock := buf.Bytes()

	x509Cert, err := x509.ParseCertificate(cert)
	if err != nil {
		return nil, err
	}

	ca := &certWithPEM{
		PEMBlock: pemBlock,
		Cert:     x509Cert,
	}
	return ca, nil
}

func createIssuedCert(pub, caPriv any, ca *x509.Certificate,
	hosts []string, org string, years int, keyUsage x509.KeyUsage) (*certWithPEM, error) {

	if ca.KeyUsage&x509.KeyUsageCertSign == 0 {
		return nil, fmt.Errorf("parent certificate cannot sign other certificates")
	}
	validUntil := time.Now().Add(time.Hour * 24 * 365 * time.Duration(years))
	if validUntil.After(ca.NotAfter) {
		validUntil = ca.NotAfter
	}
	template, err := newTemplate(hosts, org, validUntil)
	if err != nil {
		return nil, err
	}
	template.KeyUsage = keyUsage
	template.BasicConstraintsValid = true

	cert, err := x509.CreateCertificate(rand.Reader(), template, ca, pub, caPriv)
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	err = pem.Encode(buf, &pem.Block{Type: "CERTIFICATE", Bytes: cert})
	if err != nil {
		return nil, fmt.Errorf("failed to encode certificate: %w", err)
	}
	pemBlock := buf.Bytes()

	x509Cert, err := x509.ParseCertificate(cert)
	if err != nil {
		return nil, err
	}

	issuedCert := &certWithPEM{
		PEMBlock: pemBlock,
		Cert:     x509Cert,
	}
	return issuedCert, nil
}

func marshalPrivateKey(key any) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	buf := new(bytes.Buffer)
	err = pem.Encode(buf, &pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if err != nil {
		return nil, fmt.Errorf("failed to encode private key: %w", err)
	}
	return buf.Bytes(), nil
}
