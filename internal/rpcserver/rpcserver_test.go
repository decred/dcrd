// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017-2026 The Decred developers

// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcserver

import (
	"net/http"
	"testing"

	"github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

// TestAuth_UserPass_AdminAndLimited validates the user/pass authentication when
// both admin and limited credentials are configured.
func TestAuth_UserPass_AdminAndLimited(t *testing.T) {
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
		t.Run(test.name, func(t *testing.T) {
			authed, isAdmin := s.checkAuthUserPass(test.user, test.pass, "addr")
			if authed != test.wantAuthed {
				t.Errorf("unexpected authed -- got %v, want %v", authed,
					test.wantAuthed)
			}
			if isAdmin != test.wantAdmin {
				t.Errorf("unexpected isAdmin -- got %v, want %v", isAdmin,
					test.wantAdmin)
			}
		})
	}
}

// TestAuth_Header_NoCredentials validates the header authentication when no
// credentials are configured.
func TestAuth_Header_NoCredentials(t *testing.T) {
	s, err := New(&Config{})
	if err != nil {
		t.Fatalf("unable to create RPC server: %v", err)
	}
	for _, require := range []bool{true, false} {
		authed, isAdmin, err := s.checkAuth(&http.Request{}, require)
		if !authed {
			t.Errorf("unexpected authed -- got %v, want %v", authed, true)
		}
		if !isAdmin {
			t.Errorf("unexpected isAdmin -- got %v, want %v", isAdmin, true)
		}
		if err != nil {
			t.Errorf("unexpected err -- got %v, want %v", err, nil)
		}
	}
}

// TestAuth_Header_Require validates that setting the require flag causes an
// error to be returned if the request does not contain an Authorization header.
func TestAuth_Header_Require(t *testing.T) {
	s, err := New(&Config{
		RPCUser:      "user",
		RPCPass:      "pass",
		RPCLimitUser: "limit",
		RPCLimitPass: "limit",
	})
	if err != nil {
		t.Fatalf("unable to create RPC server: %v", err)
	}

	// Test without Authorization header.
	for _, require := range []bool{true, false} {
		authed, isAdmin, err := s.checkAuth(&http.Request{}, require)
		if authed {
			t.Errorf("unexpected authed -- got %v, want %v", authed, false)
		}
		if isAdmin {
			t.Errorf("unexpected isAdmin -- got %v, want %v", isAdmin, false)
		}
		if require && err == nil {
			t.Errorf("unexpected err -- got %v, want auth failure", err)
		} else if !require && err != nil {
			t.Errorf("unexpected err -- got %v, want <nil>", err)
		}
	}

	// Test with Authorization header.
	for _, require := range []bool{true, false} {
		r := &http.Request{Header: make(map[string][]string, 1)}
		r.Header["Authorization"] = []string{"Basic Nothing"}
		authed, isAdmin, err := s.checkAuth(r, require)
		if authed {
			t.Errorf("unexpected authed -- got %v, want %v", authed, false)
		}
		if isAdmin {
			t.Errorf("unexpected isAdmin -- got %v, want %v", isAdmin, false)
		}
		if err == nil {
			t.Errorf("unexpected err -- got %v, want auth failure", err)
		}
	}
}

// TestLimitedMethodsExist ensures all RPC methods listed in the limited RPC
// methods map have associated handlers defined.
func TestLimitedMethodsExist(t *testing.T) {
	for methodStr := range rpcLimited {
		method := types.Method(methodStr)
		_, haveRegularHandler := rpcHandlers[method]
		_, haveWebsocketHandler := wsHandlers[method]
		if !haveRegularHandler && !haveWebsocketHandler {
			t.Errorf("no handler found for limited method %q", method)
		}
	}
}
