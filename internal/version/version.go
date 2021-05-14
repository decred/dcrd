// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package version provides a single location to house the version information
// for dcrd and other utilities provided in the same repository.
package version

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	// semanticAlphabet defines the allowed characters for the pre-release and
	// build metadata portions of a semantic version string.
	semanticAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-."
)

// semverRE is a regular expression used to parse a semantic version string into
// its constituent parts.
var semverRE = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
	`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*` +
	`[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// These variables define the application version and follow the semantic
// versioning 2.0.0 spec (https://semver.org/).
var (
	// Note for maintainers:
	//
	// The expected process for setting the version in releases is as follows:
	// - Create a release branch of the form 'release-vMAJOR.MINOR'
	// - Modify the Version variable below on that branch to:
	//   - Remove the pre-release portion
	//   - Set the build metadata to 'release.local'
	// - Update the Version variable below on the master branch to the next
	//   expected version while retaining a pre-release of 'pre'
	//
	// These steps ensure that building from source produces versions that are
	// distinct from reproducible builds that override the Version via linker
	// flags.

	// Version is the application version per the semantic versioning 2.0.0 spec
	// (https://semver.org/).
	//
	// It is defined as a variable so it can be overridden during the build
	// process with:
	// '-ldflags "-X github.com/decred/dcrd/internal/version.Version=fullsemver"'
	// if needed.
	//
	// It MUST be a full semantic version per the semantic versioning spec or
	// the package will panic at runtime.  Of particular note is the pre-release
	// and build metadata portions MUST only contain characters from
	// semanticAlphabet.
	Version = "1.7.0-pre"

	// NOTE: The following values are set via init by parsing the above Version
	// string.

	// These fields are the individual semantic version components that define
	// the application version.
	Major         uint
	Minor         uint
	Patch         uint
	PreRelease    string
	BuildMetadata string
)

// parseUint converts the passed string to an unsigned integer or returns an
// error if it is invalid.
func parseUint(s string, fieldName string) (uint, error) {
	val, err := strconv.ParseUint(s, 10, 0)
	if err != nil {
		return 0, fmt.Errorf("malformed semver %s: %w", fieldName, err)
	}
	return uint(val), err
}

// checkSemString returns an error if the passed string contains characters that
// are not in the provided alphabet.
func checkSemString(s, alphabet, fieldName string) error {
	for _, r := range s {
		if !strings.ContainsRune(alphabet, r) {
			return fmt.Errorf("malformed semver %s: %q invalid", fieldName, r)
		}
	}
	return nil
}

// parseSemVer parses various semver components from the provided string.
func parseSemVer(s string) (uint, uint, uint, string, string, error) {
	// Parse the various semver component from the version string via a regular
	// expression.
	m := semverRE.FindStringSubmatch(s)
	if m == nil {
		err := fmt.Errorf("malformed version string %q: does not conform to "+
			"semver specification", s)
		return 0, 0, 0, "", "", err
	}

	major, err := parseUint(m[1], "major")
	if err != nil {
		return 0, 0, 0, "", "", err
	}

	minor, err := parseUint(m[2], "minor")
	if err != nil {
		return 0, 0, 0, "", "", err
	}

	patch, err := parseUint(m[3], "patch")
	if err != nil {
		return 0, 0, 0, "", "", err
	}

	preRel := m[4]
	err = checkSemString(preRel, semanticAlphabet, "pre-release")
	if err != nil {
		return 0, 0, 0, s, s, err
	}

	build := m[5]
	err = checkSemString(build, semanticAlphabet, "buildmetadata")
	if err != nil {
		return 0, 0, 0, s, s, err
	}

	return major, minor, patch, preRel, build, nil
}

func init() {
	var err error
	Major, Minor, Patch, PreRelease, BuildMetadata, err = parseSemVer(Version)
	if err != nil {
		panic(err)
	}
}

// String returns the application version as a properly formed string per the
// semantic versioning 2.0.0 spec (https://semver.org/).
func String() string {
	return Version
}

// NormalizeString returns the passed string stripped of all characters which
// are not valid according to the semantic versioning guidelines for pre-release
// and build metadata strings.  In particular they MUST only contain characters
// in semanticAlphabet.
func NormalizeString(str string) string {
	var result bytes.Buffer
	for _, r := range str {
		if strings.ContainsRune(semanticAlphabet, r) {
			result.WriteRune(r)
		}
	}
	return result.String()
}
