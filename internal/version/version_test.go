// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package version

import "testing"

// TestSemVerParsing ensures parsing a semantic version string works as
// expected.
func TestSemVerParsing(t *testing.T) {
	tests := []struct {
		ver     string // semantic version string to parse
		major   uint   // expected major version
		minor   uint   // expected minor version
		patch   uint   // expected patch version
		pre     string // expected pre-release string
		build   string // expected build metadata string
		invalid bool   // expected error
	}{{
		ver:   "0.0.4",
		major: 0,
		minor: 0,
		patch: 4,
	}, {
		ver:   "1.2.3",
		major: 1,
		minor: 2,
		patch: 3,
	}, {
		ver:   "10.20.30",
		major: 10,
		minor: 20,
		patch: 30,
	}, {
		ver:   "1.1.2-prerelease+meta",
		major: 1,
		minor: 1,
		patch: 2,
		pre:   "prerelease",
		build: "meta",
	}, {
		ver:   "1.1.2+meta",
		major: 1,
		minor: 1,
		patch: 2,
		build: "meta",
	}, {
		ver:   "1.1.2+meta-valid",
		major: 1,
		minor: 1,
		patch: 2,
		build: "meta-valid",
	}, {
		ver:   "1.0.0-alpha",
		major: 1,
		minor: 0,
		patch: 0,
		pre:   "alpha",
	}, {
		ver:   "1.0.0-beta",
		major: 1,
		minor: 0,
		patch: 0,
		pre:   "beta",
	}, {
		ver:   "1.0.0-alpha.beta",
		major: 1,
		minor: 0,
		patch: 0,
		pre:   "alpha.beta",
	}, {
		ver:   "1.0.0-alpha.beta.1",
		major: 1,
		minor: 0,
		patch: 0,
		pre:   "alpha.beta.1",
	}, {
		ver:   "1.0.0-alpha.1",
		major: 1,
		minor: 0,
		patch: 0,
		pre:   "alpha.1",
	}, {
		ver:   "1.0.0-alpha0.valid",
		major: 1,
		minor: 0,
		patch: 0,
		pre:   "alpha0.valid",
	}, {
		ver:   "1.0.0-alpha.0valid",
		major: 1,
		minor: 0,
		patch: 0,
		pre:   "alpha.0valid",
	}, {
		ver:   "1.0.0-alpha-a.b-c-somethinglong+build.1-aef.1-its-okay",
		major: 1,
		minor: 0,
		patch: 0,
		pre:   "alpha-a.b-c-somethinglong",
		build: "build.1-aef.1-its-okay",
	}, {
		ver:   "1.0.0-rc.1+build.1",
		major: 1,
		minor: 0,
		patch: 0,
		pre:   "rc.1",
		build: "build.1",
	}, {
		ver:   "2.0.0-rc.1+build.123",
		major: 2,
		minor: 0,
		patch: 0,
		pre:   "rc.1",
		build: "build.123",
	}, {
		ver:   "1.2.3-beta",
		major: 1,
		minor: 2,
		patch: 3,
		pre:   "beta",
	}, {
		ver:   "10.2.3-DEV-SNAPSHOT",
		major: 10,
		minor: 2,
		patch: 3,
		pre:   "DEV-SNAPSHOT",
	}, {
		ver:   "1.2.3-SNAPSHOT-123",
		major: 1,
		minor: 2,
		patch: 3,
		pre:   "SNAPSHOT-123",
	}, {
		ver:   "1.0.0",
		major: 1,
		minor: 0,
		patch: 0,
	}, {
		ver:   "2.0.0",
		major: 2,
		minor: 0,
		patch: 0,
	}, {
		ver:   "1.1.7",
		major: 1,
		minor: 1,
		patch: 7,
	}, {
		ver:   "2.0.0+build.1848",
		major: 2,
		minor: 0,
		patch: 0,
		build: "build.1848",
	}, {
		ver:   "2.0.1-alpha.1227",
		major: 2,
		minor: 0,
		patch: 1,
		pre:   "alpha.1227",
	}, {
		ver:   "1.0.0-alpha+beta",
		major: 1,
		minor: 0,
		patch: 0,
		pre:   "alpha",
		build: "beta",
	}, {
		ver:   "1.2.3----RC-SNAPSHOT.12.9.1--.12+788",
		major: 1,
		minor: 2,
		patch: 3,
		pre:   "---RC-SNAPSHOT.12.9.1--.12",
		build: "788",
	}, {
		ver:   "1.2.3----R-S.12.9.1--.12+meta",
		major: 1,
		minor: 2,
		patch: 3,
		pre:   "---R-S.12.9.1--.12",
		build: "meta",
	}, {
		ver:   "1.2.3----RC-SNAPSHOT.12.9.1--.12",
		major: 1,
		minor: 2,
		patch: 3,
		pre:   "---RC-SNAPSHOT.12.9.1--.12",
	}, {
		ver:   "1.0.0+0.build.1-rc.10000aaa-kk-0.1",
		major: 1,
		minor: 0,
		patch: 0,
		build: "0.build.1-rc.10000aaa-kk-0.1",
	}, {
		ver:   "1.0.0-0A.is.legal",
		major: 1,
		minor: 0,
		patch: 0,
		pre:   "0A.is.legal",
	}, {
		ver:     "1",
		invalid: true,
	}, {
		ver:     "1.2",
		invalid: true,
	}, {
		ver:     "1.2.3-0123",
		invalid: true,
	}, {
		ver:     "1.2.3-0123.0123",
		invalid: true,
	}, {
		ver:     "1.1.2+.123",
		invalid: true,
	}, {
		ver:     "+invalid",
		invalid: true,
	}, {
		ver:     "-invalid",
		invalid: true,
	}, {
		ver:     "-invalid+invalid",
		invalid: true,
	}, {
		ver:     "-invalid.01",
		invalid: true,
	}, {
		ver:     "alpha",
		invalid: true,
	}, {
		ver:     "alpha.beta",
		invalid: true,
	}, {
		ver:     "alpha.beta.1",
		invalid: true,
	}, {
		ver:     "alpha.1",
		invalid: true,
	}, {
		ver:     "alpha+beta",
		invalid: true,
	}, {
		ver:     "alpha_beta",
		invalid: true,
	}, {
		ver:     "alpha.",
		invalid: true,
	}, {
		ver:     "alpha..",
		invalid: true,
	}, {
		ver:     "beta",
		invalid: true,
	}, {
		ver:     "1.0.0-alpha_beta",
		invalid: true,
	}, {
		ver:     "-alpha.",
		invalid: true,
	}, {
		ver:     "1.0.0-alpha..",
		invalid: true,
	}, {
		ver:     "1.0.0-alpha..1",
		invalid: true,
	}, {
		ver:     "1.0.0-alpha...1",
		invalid: true,
	}, {
		ver:     "1.0.0-alpha....1",
		invalid: true,
	}, {
		ver:     "1.0.0-alpha.....1",
		invalid: true,
	}, {
		ver:     "1.0.0-alpha......1",
		invalid: true,
	}, {
		ver:     "1.0.0-alpha.......1",
		invalid: true,
	}, {
		ver:     "01.1.1",
		invalid: true,
	}, {
		ver:     "1.01.1",
		invalid: true,
	}, {
		ver:     "1.1.01",
		invalid: true,
	}, {
		ver:     "1.2",
		invalid: true,
	}, {
		ver:     "1.2.3.DEV",
		invalid: true,
	}, {
		ver:     "1.2-SNAPSHOT",
		invalid: true,
	}, {
		ver:     "1.2.31.2.3----RC-SNAPSHOT.12.09.1--..12+788",
		invalid: true,
	}, {
		ver:     "1.2-RC-SNAPSHOT",
		invalid: true,
	}, {
		ver:     "-1.0.3-gamma+b7718",
		invalid: true,
	}, {
		ver:     "+justmeta",
		invalid: true,
	}, {
		ver:     "9.8.7+meta+meta",
		invalid: true,
	}, {
		ver:     "9.8.7-whatever+meta+meta",
		invalid: true,
	}, {
		// Would be valid except major is > max uint64.
		ver:     "99999999999999999999999.999999999999999999.99999999999999999",
		invalid: true,
	}, {
		ver: "999999999.999999999.999999999----RC-SNAPSHOT.12.09.1-----------" +
			"---------------------..12",
		invalid: true,
	}}

	for _, test := range tests {
		major, minor, patch, pre, build, err := parseSemVer(test.ver)
		if test.invalid && err == nil {
			t.Errorf("%q: did not receive expected error", test.ver)
			continue
		}
		if !test.invalid && err != nil {
			t.Errorf("%q: unexpected err: %v", test.ver, err)
			continue
		}

		if major != test.major {
			t.Errorf("%q: mismatched major -- got %d, want %d", test.ver,
				major, test.major)
			continue
		}

		if minor != test.minor {
			t.Errorf("%q: mismatched minor -- got %d, want %d", test.ver,
				minor, test.minor)
			continue
		}

		if patch != test.patch {
			t.Errorf("%q: mismatched patch -- got %d, want %d", test.ver,
				patch, test.patch)
			continue
		}

		if pre != test.pre {
			t.Errorf("%q: mismatched pre-release -- got %s, want %s", test.ver,
				pre, test.pre)
			continue
		}

		if build != test.build {
			t.Errorf("%q: mismatched buildmetadata -- got %s, want %s",
				test.ver, build, test.build)
			continue
		}
	}
}
