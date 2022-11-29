// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017-2022 The Decred developers

// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcserver

import (
	"net/http"
	"testing"
)

func TestCheckAuthUserPass(t *testing.T) {
	s, err := New(&Config{
		RPCUser:      "user",
		RPCPass:      "pass",
		RPCLimitUser: "limit",
		RPCLimitPass: "limit",
	})
	if err != nil {
		t.Fatalf("unable to create RPC server: %v", err)
	}
	tests := []struct {
		name       string
		user       string
		pass       string
		wantAuthed bool
		wantAdmin  bool
	}{
		{
			name:       "correct admin",
			user:       "user",
			pass:       "pass",
			wantAuthed: true,
			wantAdmin:  true,
		},
		{
			name:       "correct limited user",
			user:       "limit",
			pass:       "limit",
			wantAuthed: true,
			wantAdmin:  false,
		},
		{
			name:       "invalid admin",
			user:       "user",
			pass:       "p",
			wantAuthed: false,
			wantAdmin:  false,
		},
		{
			name:       "invalid limited user",
			user:       "limit",
			pass:       "",
			wantAuthed: false,
			wantAdmin:  false,
		},
		{
			name:       "invalid empty user",
			user:       "",
			pass:       "",
			wantAuthed: false,
			wantAdmin:  false,
		},
	}
	for _, test := range tests {
		authed, isAdmin := s.checkAuthUserPass(test.user, test.pass, "addr")
		if authed != test.wantAuthed {
			t.Errorf("%q: unexpected authed -- got %v, want %v", test.name, authed,
				test.wantAuthed)
		}
		if isAdmin != test.wantAdmin {
			t.Errorf("%q: unexpected isAdmin -- got %v, want %v", test.name, isAdmin,
				test.wantAdmin)
		}
	}
}

func TestCheckAuth(t *testing.T) {
	{
		s, err := New(&Config{})
		if err != nil {
			t.Fatalf("unable to create RPC server: %v", err)
		}
		for i := 0; i <= 1; i++ {
			authed, isAdmin, err := s.checkAuth(&http.Request{}, i == 0)
			if !authed {
				t.Errorf(" unexpected authed -- got %v, want %v", authed, true)
			}
			if !isAdmin {
				t.Errorf("unexpected isAdmin -- got %v, want %v", isAdmin, true)
			}
			if err != nil {
				t.Errorf("unexpected err -- got %v, want %v", err, nil)
			}
		}
	}
	{
		s, err := New(&Config{
			RPCUser:      "user",
			RPCPass:      "pass",
			RPCLimitUser: "limit",
			RPCLimitPass: "limit",
		})
		if err != nil {
			t.Fatalf("unable to create RPC server: %v", err)
		}
		for i := 0; i <= 1; i++ {
			authed, isAdmin, err := s.checkAuth(&http.Request{}, i == 0)
			if authed {
				t.Errorf(" unexpected authed -- got %v, want %v", authed, false)
			}
			if isAdmin {
				t.Errorf("unexpected isAdmin -- got %v, want %v", isAdmin, false)
			}
			if i == 0 && err == nil {
				t.Errorf("unexpected err -- got %v, want auth failure", err)
			} else if i != 0 && err != nil {
				t.Errorf("unexpected err -- got %v, want <nil>", err)
			}
		}
		for i := 0; i <= 1; i++ {
			r := &http.Request{Header: make(map[string][]string, 1)}
			r.Header["Authorization"] = []string{"Basic Nothing"}
			authed, isAdmin, err := s.checkAuth(r, i == 0)
			if authed {
				t.Errorf(" unexpected authed -- got %v, want %v", authed, false)
			}
			if isAdmin {
				t.Errorf("unexpected isAdmin -- got %v, want %v", isAdmin, false)
			}
			if err == nil {
				t.Errorf("unexpected err -- got %v, want auth failure", err)
			}
		}
	}
}
