// Copyright (c) 2024-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/netip"
	"strconv"
	"sync"
	"time"
)

// portToLocalHostAddr prepends a default host of 127.0.0.1 when the provided
// address is solely a port number.
func portToLocalHostAddr(addr string) string {
	if _, err := strconv.Atoi(addr); err == nil {
		addr = net.JoinHostPort("127.0.0.1", addr)
	}
	return addr
}

// validateProfileAddr ensures the provided address is of the form "host:port"
// and that the port is between 1024 and 65535.
func validateProfileAddr(addr string) error {
	// Ensure the address is valid host:port syntax.
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}

	// Ensure the port is in range.
	if port, _ := strconv.Atoi(portStr); port < 1024 || port > 65535 {
		str := "address %q: port must be between 1024 and 65535"
		return fmt.Errorf(str, addr)
	}

	return nil
}

// profileServer provides facilities for dynamically starting and stopping an
// HTTP server that serves the pprof profiling endpoints.
type profileServer struct {
	wg         sync.WaitGroup
	mtx        sync.Mutex
	registered bool
	server     *http.Server
	listeners  []string
}

// Start binds a listener to the provided address and launches an HTTP server
// that handles profiling endpoints in the background using that listener.  An
// error is returned when the listener fails to bind.
//
// When the flag to allow non loopback addresses is set, an error is returned
// when the provided listen address does not normalize to an IPv4 or IPv6
// loopback address.
//
// It has no effect when the server is already running, so it may be called
// multiple times without error.
//
// Callers can make use of [Listeners] to determine if the server is already
// running since there will only be active listeners when it is.
//
// It is the caller's responsibility to call the Stop method to shutdown the
// server.
func (s *profileServer) Start(listenAddr string, allowNonLoopback bool) error {
	defer s.mtx.Unlock()
	s.mtx.Lock()

	// Nothing to do when the server is already running.
	if s.server != nil {
		return nil
	}

	// Potentially convert a raw port to an IPv4 localhost address (aka prepend
	// 127.0.0.1).
	listenAddr = portToLocalHostAddr(listenAddr)

	// Ensure the provided address is a valid hostname and port with a port that
	// is in range.
	if err := validateProfileAddr(listenAddr); err != nil {
		return err
	}

	// Resolve interface names to associated IP addresses and remove duplicates.
	// No default port is specified since a port is guaranteed to be present per
	// the previous validation.
	listenAddrs := []string{listenAddr}
	listenAddrs = normalizeAddresses(listenAddrs, "", normalizeInterfaceAddrs)

	// Reject non loopback addresses when requested.
	if !allowNonLoopback {
		for _, addrStr := range listenAddrs {
			addr, _ := netip.ParseAddrPort(addrStr)
			if !addr.IsValid() || !addr.Addr().IsLoopback() {
				return fmt.Errorf("not permitted to listen on non loopback "+
					"address %q without setting the flag to allow it", addrStr)
			}
		}
	}

	// Expand the provided listen address into relevant IPv4 and IPv6 net.Addrs
	// to listen on with TCP.  This includes properly detecting addresses which
	// apply to "all interfaces" and adds the address as both IPv4 and IPv6.
	netAddrs, err := parseListeners(listenAddrs)
	if err != nil {
		return err
	}

	listeners := make([]net.Listener, 0, len(netAddrs))
	s.listeners = make([]string, 0, len(netAddrs))
	for _, addr := range netAddrs {
		listener, err := net.Listen(addr.Network(), addr.String())
		if err != nil {
			return fmt.Errorf("unable to listen on %s: %w", listenAddr, err)
		}
		listeners = append(listeners, listener)
		s.listeners = append(s.listeners, listener.Addr().String())
	}

	// Register a redirect to the profiling endpoints registered by the pprof
	// package when not already done.
	if !s.registered {
		redirect := http.RedirectHandler("/debug/pprof", http.StatusSeeOther)
		http.Handle("/", redirect)
		s.registered = true
	}

	// Create a new HTTP server and serve it in a separate goroutine for each
	// listener.
	s.server = &http.Server{
		Addr:              listenAddr,
		ReadHeaderTimeout: time.Second * 3,
	}
	for listenerIdx := range listeners {
		listener := listeners[listenerIdx]
		dcrdLog.Infof("Profiling server listening on %s", listener.Addr())
		s.wg.Add(1)
		go func(httpServer *http.Server) {
			defer s.wg.Done()

			err := httpServer.Serve(listener)
			if !errors.Is(err, http.ErrServerClosed) {
				dcrdLog.Errorf("Profiling server listening on %s exited with "+
					"unexpected error: %v", listener.Addr(), err)
			}
		}(s.server)
	}

	return nil
}

// Stop immediately closes the active listener and any connections to the
// profile server.
//
// It has no effect when the server is not running, so it may be called multiple
// times without error.
func (s *profileServer) Stop() error {
	defer s.mtx.Unlock()
	s.mtx.Lock()

	// Nothing to do when the server is not running.
	if s.server == nil {
		return nil
	}

	// Shutdown the server and wait for the serving goroutines to finish.  Also,
	// clear the server field and listeners since they are no longer valid.
	err := s.server.Close()
	s.server = nil
	s.listeners = nil
	s.wg.Wait()
	if err != nil {
		dcrdLog.Errorf("Profiling server stopped with unexpected error: %v",
			err)
		return err
	}

	dcrdLog.Info("Profiling server stopped")
	return nil
}

// Listeners returns all listeners the profile server is currently listening on.
// It may also be used as a means to tell if the server is currently running
// since there will only be active listeners when it is.
func (s *profileServer) Listeners() []string {
	defer s.mtx.Unlock()
	s.mtx.Lock()

	return s.listeners
}
