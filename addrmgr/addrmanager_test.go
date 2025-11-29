// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package addrmgr

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/wire"
)

const (
	// Convenience IP strings.
	routableIPv4Addr    = "173.194.115.66" // Points to google.
	routableIPv6Addr    = "2003::"
	nonRoutableIPv4Addr = "255.255.255.255"
	nonRoutableIPv6Addr = "::1"
	torv3Host           = "xa4r2iadxm55fbnqgwwi5mymqdcofiu3w6rpbtqn7b2dyn7mgwj64jyd.onion"
)

// natfAny defines a filter that will allow network addresses of any type.
func natfAny(addrType NetAddressType) bool {
	return true
}

// natfOnlyIPv4 defines a filter that will only allow IPv4 netAddrs.
func natfOnlyIPv4(addrType NetAddressType) bool {
	return addrType == IPv4Address
}

// natfOnlyIPv6 defines a filter that will only allow IPv6 netAddrs.
func natfOnlyIPv6(addrType NetAddressType) bool {
	return addrType == IPv6Address
}

// natfOnlyTORv3 defines a filter that will only allow TORv3 netAddrs.
func natfOnlyTORv3(addrType NetAddressType) bool {
	return addrType == TORv3Address
}

// addAddressByIP is a convenience function that adds an address to the
// address manager given a valid string representation of an ip address and
// a port.
func (a *AddrManager) addAddressByIP(addr string, port uint16) {
	ip := net.ParseIP(addr)
	na := NewNetAddressFromIPPort(ip, port, 0)
	a.addOrUpdateAddress(na, na)
}

// TestAddOrUpdateAddress ensures that non-routable addresses are not added,
// that addresses are correctly added and/or updated, and that tried addresses
// are not re-added.
func TestAddOrUpdateAddress(t *testing.T) {
	amgr := New("testaddaddressupdate")
	amgr.Start()
	if ka := amgr.GetAddress(natfAny); ka != nil {
		t.Fatal("address manager should contain no addresses")
	}

	// Attempt to add a non-routable address.
	ip := net.ParseIP(nonRoutableIPv4Addr)
	if ip == nil {
		t.Fatalf("invalid IP address %s", nonRoutableIPv4Addr)
	}
	na := NewNetAddressFromIPPort(net.ParseIP(nonRoutableIPv4Addr), 8333, 0)
	amgr.addOrUpdateAddress(na, na)
	if ka := amgr.GetAddress(natfAny); ka != nil {
		t.Fatal("address manager should contain no addresses")
	}

	// Add a good address.
	ip = net.ParseIP(routableIPv4Addr)
	if ip == nil {
		t.Fatalf("invalid IP address %s", routableIPv4Addr)
	}
	na = NewNetAddressFromIPPort(net.ParseIP(routableIPv4Addr), 8333, 0)
	amgr.addOrUpdateAddress(na, na)
	ka := amgr.GetAddress(natfAny)
	newlyAddedAddr := ka.NetAddress()
	if ka == nil {
		t.Fatal("address manager should contain newly added known address")
	}
	if newlyAddedAddr == na {
		t.Fatal("newly added known address should have a new network address " +
			"reference, but a previously held reference was found")
	}
	if !reflect.DeepEqual(newlyAddedAddr, na) {
		t.Fatalf("address manager should contain address that was added - "+
			"got %v, want %v", newlyAddedAddr, na)
	}
	// Add the same address again, but with different timestamp to trigger
	// an update rather than an insert.
	ts := na.Timestamp.Add(time.Second)
	na.Timestamp = ts
	amgr.addOrUpdateAddress(na, na)

	// The address should be in the address manager with a new timestamp.
	// The network address reference held by the known address should also
	// differ.
	updatedKnownAddress := amgr.GetAddress(natfAny)
	netAddrFromUpdate := updatedKnownAddress.NetAddress()
	if updatedKnownAddress == nil {
		t.Fatal("address manager should contain updated known address")
	}
	if ka != updatedKnownAddress {
		t.Fatalf("updated known address returned by the address manager " +
			"should not be a new known address reference")
	}
	if netAddrFromUpdate == newlyAddedAddr || netAddrFromUpdate == na {
		t.Fatal("updated known address should have a new network address " +
			"reference, but a previously held reference was found")
	}
	if !reflect.DeepEqual(netAddrFromUpdate, na) {
		t.Fatalf("address manager should contain address that was updated - "+
			"got %v, want %v", netAddrFromUpdate, na)
	}
	if !netAddrFromUpdate.Timestamp.Equal(ts) {
		t.Fatal("address manager did not update timestamp")
	}

	// Mark the address as tried.
	err := amgr.Good(ka.NetAddress())
	if err != nil {
		t.Fatalf("marking address as good failed: %v", err)
	}
	if _, exists := amgr.addrNew[0][na.Key()]; exists {
		t.Fatalf("expected address %s to not exist in new bucket", na)
	}
	// Attempt to add the same address again, even though it's already tried.
	amgr.addOrUpdateAddress(na, na)

	// Stop the address manager.
	if err := amgr.Stop(); err != nil {
		t.Fatalf("address manager failed to stop - %v", err)
	}
}

// TestExpireNew ensures that expireNew will correctly throw away bad addresses.
func TestExpireNew(t *testing.T) {
	n := New("testexpirenew")

	now := time.Now()

	// Create a good address and a bad address.
	lastAttempt := now.Add(-2 * time.Minute)
	recentTimestamp := now.Add(-1 * time.Hour)
	oldTimestamp := now.Add(-40 * 24 * time.Hour)

	goodAddr := newKnownAddress(recentTimestamp, 0, lastAttempt, now, false, 2)
	badAddr := newKnownAddress(oldTimestamp, 0, lastAttempt, now, false, 1)

	// Add the addresses to bucket 0.
	bucket := 0
	n.addrNew[bucket]["goodKey"] = goodAddr
	n.addrNew[bucket]["badKey"] = badAddr
	n.addrIndex["goodKey"] = goodAddr
	n.addrIndex["badKey"] = badAddr
	n.nNew = 2

	numAddrs := n.numAddresses()

	if numAddrs != 2 {
		t.Errorf("Expected 2 addresses, got %d", numAddrs)
	}

	n.expireNew(bucket)

	if _, exists := n.addrNew[bucket]["badKey"]; exists {
		t.Error("Expected bad address to be removed")
	}
	if _, exists := n.addrIndex["badKey"]; exists {
		t.Error("Expected bad address to be removed from addrIndex")
	}
	if _, exists := n.addrNew[bucket]["goodKey"]; !exists {
		t.Error("Expected good address to remain")
	}
	numAddrs = n.numAddresses()
	if numAddrs != 1 {
		t.Errorf("Only expected 1 address, got %d", numAddrs)
	}
}

// TestLoadPeersWithCorruptPeersFile ensures that starting and stopping the
// address manager will correctly remove an empty peers file.
func TestLoadPeersWithCorruptPeersFile(t *testing.T) {
	dir := t.TempDir()
	peersFile := filepath.Join(dir, peersFilename)
	// create corrupt (empty) peers file.
	fp, err := os.Create(peersFile)
	if err != nil {
		t.Fatalf("Could not create empty peers file: %s", peersFile)
	}
	if err := fp.Close(); err != nil {
		t.Fatalf("Could not write empty peers file: %s", peersFile)
	}
	amgr := New(dir)
	amgr.Start()
	amgr.Stop()
	if _, err := os.Stat(peersFile); err != nil {
		t.Fatalf("Corrupt peers file has not been removed: %s", peersFile)
	}
}

// TestStartStop tests the behavior of the address manager when it is started
// and stopped.
func TestStartStop(t *testing.T) {
	dir := t.TempDir()

	// Ensure the peers file does not exist before starting the address manager.
	peersFile := filepath.Join(dir, peersFilename)
	if _, err := os.Stat(peersFile); !os.IsNotExist(err) {
		t.Fatalf("peers file exists though it should not: %s", peersFile)
	}

	amgr := New(dir)
	amgr.Start()

	// Add single network address to the address manager.
	amgr.addAddressByIP(routableIPv4Addr, 8333)

	// Stop the address manager to force the known addresses to be flushed
	// to the peers file.
	if err := amgr.Stop(); err != nil {
		t.Fatalf("address manager failed to stop: %v", err)
	}

	// Verify that the peers file has been written to.
	if _, err := os.Stat(peersFile); err != nil {
		t.Fatalf("peers file does not exist: %s", peersFile)
	}

	// Start a new address manager, which initializes it from the peers file.
	amgr = New(dir)
	amgr.Start()

	knownAddress := amgr.GetAddress(natfAny)
	if knownAddress == nil {
		t.Fatal("address manager should contain known address")
	}

	// Verify that the known address matches what was added to the address
	// manager previously.
	wantNetAddrKey := net.JoinHostPort(routableIPv4Addr, "8333")
	gotNetAddrKey := knownAddress.na.Key()
	if gotNetAddrKey != wantNetAddrKey {
		t.Fatalf("address manager does not contain expected address - "+
			"got %v, want %v", gotNetAddrKey, wantNetAddrKey)
	}

	if err := amgr.Stop(); err != nil {
		t.Fatalf("address manager failed to stop: %v", err)
	}
}

// TestNeedMoreAddresses adds 1000 addresses and then checks to see if
// NeedMoreAddresses correctly determines that no more addresses are needed.
func TestNeedMoreAddresses(t *testing.T) {
	n := New("testneedmoreaddresses")
	addrsToAdd := needAddressThreshold
	b := n.NeedMoreAddresses()
	if !b {
		t.Fatal("expected the address manager to need more addresses")
	}
	addrs := make([]*NetAddress, addrsToAdd)

	for i := 0; i < addrsToAdd; i++ {
		s := fmt.Sprintf("%d.%d.173.147", i/128+60, i%128+60)
		addrs[i] = NewNetAddressFromIPPort(net.ParseIP(s), 8333, wire.SFNodeNetwork)
	}

	srcAddr := NewNetAddressFromIPPort(net.ParseIP("173.144.173.111"), 8333, 0)

	n.AddAddresses(addrs, srcAddr)
	numAddrs := n.numAddresses()
	if numAddrs > addrsToAdd {
		t.Fatalf("number of addresses is too many %d vs %d", numAddrs,
			addrsToAdd)
	}

	b = n.NeedMoreAddresses()
	if b {
		t.Fatal("expected address manager to not need more addresses")
	}
}

// TestAddressCache ensures that AddressCache doesn't return bad addresses,
// never-attempted addresses, or addresses which don't match the given filter.
func TestAddressCache(t *testing.T) {
	// Test that the randomized subset is nil if no addresses are known.
	n := New("testaddresscacheisempty")
	addrList := n.AddressCache(natfAny)
	if addrList != nil {
		t.Fatalf("expected empty AddressCache. Got %v", addrList)
	}

	// Test that bad and never-attempted addresses aren't shared.
	n = New("testaddresscachewithbad")
	srcAddr := NewNetAddressFromIPPort(net.ParseIP("173.144.173.111"), 8333, 0)
	// Create and add a bad address from the future.
	futureTime := time.Now().AddDate(0, 1, 0)
	badAddr := NetAddress{
		Type:      IPv4Address,
		IP:        net.ParseIP("6.6.6.6"),
		Port:      uint16(8333),
		Timestamp: time.Unix(futureTime.Unix(), 0),
		Services:  wire.SFNodeNetwork,
	}
	badAddrSlice := make([]*NetAddress, 1)
	badAddrSlice[0] = &badAddr
	n.AddAddresses(badAddrSlice, srcAddr)
	// Create and add a good address, but don't mark it as attempted.
	goodAddrSlice := make([]*NetAddress, 1)
	goodAddrSlice[0] = NewNetAddressFromIPPort(net.ParseIP("1.1.1.1"), 8333, wire.SFNodeNetwork)
	n.AddAddresses(goodAddrSlice, srcAddr)
	// Neither address should be returned.
	addrList = n.AddressCache(natfAny)
	if len(addrList) != 0 {
		t.Fatalf("expected empty AddressCache. Got %v", addrList)
	}

	// Test that a filter will prevent certain addresses from being shared.
	n = New("testaddresscachewithfilter")
	goodIPv4 := NewNetAddressFromIPPort(net.ParseIP(routableIPv4Addr), 0, wire.SFNodeNetwork)
	goodIPv6 := NewNetAddressFromIPPort(net.ParseIP(routableIPv6Addr), 0, wire.SFNodeNetwork)
	n.addOrUpdateAddress(goodIPv4, srcAddr)
	n.addOrUpdateAddress(goodIPv6, srcAddr)
	// Mark them both as Good.
	n.Good(goodIPv4)
	n.Good(goodIPv6)
	// Only the IPv6 address should be returned.
	addrList = n.AddressCache(natfOnlyIPv6)
	if len(addrList) != 1 {
		t.Fatalf("expected only 1 address in the cache. Got %d", len(addrList))
	}
	if !reflect.DeepEqual(addrList[0].IP, goodIPv6.IP) {
		t.Fatalf("expected only the IPv6 address. Got %s", net.IP(addrList[0].IP))
	}
}

func TestGetAddress(t *testing.T) {
	n := New("testgetaddress")

	// Get an address from an empty set (should error).
	if rv := n.GetAddress(natfAny); rv != nil {
		t.Fatalf("GetAddress failed - got: %v, want: %v", rv, nil)
	}

	// Add a new address and get it.
	n.addAddressByIP(routableIPv4Addr, 8333)
	ka := n.GetAddress(natfAny)
	if ka == nil {
		t.Fatal("did not get an address where there is one in the pool")
	}

	ipStringA := ka.NetAddress().String()
	ipKey := net.JoinHostPort(routableIPv4Addr, "8333")
	if ipStringA != ipKey {
		t.Fatalf("unexpected ip - got %s, want %s", ipStringA, ipKey)
	}

	// Mark this as a good address and get it.
	err := n.Good(ka.NetAddress())
	if err != nil {
		t.Fatalf("marking address as good failed: %v", err)
	}

	// Verify that the previously added address still exists in the address
	// manager after being marked as good.
	ka = n.GetAddress(natfAny)
	if ka == nil {
		t.Fatal("did not get an address when one was expected")
	}

	ipStringB := ka.NetAddress().String()
	if ipStringB != ipKey {
		t.Fatalf("unexpected ip - got %s, want %s", ipStringB, ipKey)
	}

	numAddrs := n.numAddresses()
	if numAddrs != 1 {
		t.Fatalf("unexpected number of addresses - got %d, want 1", numAddrs)
	}

	// Attempting to mark an unknown address as good should return an error.
	unknownIP := net.ParseIP("1.2.3.4")
	unknownNetAddress := NewNetAddressFromIPPort(unknownIP, 1234, wire.SFNodeNetwork)
	err = n.Good(unknownNetAddress)
	if err == nil {
		t.Fatal("attempting to mark unknown address as good should have " +
			"returned an error")
	}
}

// TestAttempt ensures that Attempt will correctly update the lastAttempt time.
func TestAttempt(t *testing.T) {
	n := New("testattempt")

	// Add a new address and get it.
	n.addAddressByIP(routableIPv4Addr, 8333)
	ka := n.GetAddress(natfAny)

	if !ka.LastAttempt().IsZero() {
		t.Fatal("address should not have been attempted")
	}

	na := ka.NetAddress()
	err := n.Attempt(na)
	if err != nil {
		t.Fatalf("marking address as attempted failed - %v", err)
	}

	if ka.LastAttempt().IsZero() {
		t.Fatal("address should have an attempt, but does not")
	}

	// Attempt an ip not known to the address manager.
	unknownIP := net.ParseIP("1.2.3.4")
	unknownNetAddress := NewNetAddressFromIPPort(unknownIP, 1234, wire.SFNodeNetwork)
	err = n.Attempt(unknownNetAddress)
	if err == nil {
		t.Fatal("attempting unknown address should have returned an error")
	}
}

// TestConnected ensures that Connected will correctly update the connected timestamp.
func TestConnected(t *testing.T) {
	n := New("testconnected")

	// Add a new address and get it
	n.addAddressByIP(routableIPv4Addr, 8333)
	ka := n.GetAddress(natfAny)
	na := ka.NetAddress()
	// make it an hour ago
	na.Timestamp = time.Unix(time.Now().Add(time.Hour*-1).Unix(), 0)

	err := n.Connected(na)
	if err != nil {
		t.Fatalf("marking address as connected failed - %v", err)
	}

	if !ka.NetAddress().Timestamp.After(na.Timestamp) {
		t.Fatal("address should have a new timestamp, but does not")
	}

	// Attempt to flag an ip address not known to the address manager as
	// connected.
	unknownIP := net.ParseIP("1.2.3.4")
	unknownNetAddress := NewNetAddressFromIPPort(unknownIP, 1234, wire.SFNodeNetwork)
	err = n.Connected(unknownNetAddress)
	if err == nil {
		t.Fatal("attempting to mark unknown address as connected should have " +
			"returned an error")
	}
}

// TestGood tests the behavior of address management in three phases:
// Phase 1 generates 4096 new addresses and tests adding them to the address
// manager, ensuring they are correctly added and marked as good.
// Phase 2 tests the internal behavior of address management between the "new"
// and "tried" address buckets.  It ensures that when an address is marked as
// good, it is then moved from the new bucket into the tried bucket.  It also
// ensures that Good correctly handles the case when the tried bucket overflows.
// Phase 3 tests that Good will correctly error when trying to mark an address
// as good if it hasn't first been added to a new bucket.
func TestGood(t *testing.T) {
	// Phase 1: test adding addresses and marking them good.
	n := New("testgood")
	addrsToAdd := 64 * 64
	addrs := make([]*NetAddress, addrsToAdd)

	for i := 0; i < addrsToAdd; i++ {
		s := fmt.Sprintf("%d.173.147.%d", i/64+60, i%64+60)
		addrs[i] = NewNetAddressFromIPPort(net.ParseIP(s), 8333, wire.SFNodeNetwork)
	}

	srcAddr := NewNetAddressFromIPPort(net.ParseIP("173.144.173.111"), 8333, wire.SFNodeNetwork)

	n.AddAddresses(addrs, srcAddr)
	for _, addr := range addrs {
		n.Good(addr)
	}

	numAddrs := n.numAddresses()
	if numAddrs >= addrsToAdd {
		t.Fatalf("Number of addresses is too many: %d vs %d", numAddrs,
			addrsToAdd)
	}

	numCache := len(n.AddressCache(natfAny))
	if numCache >= numAddrs/4 {
		t.Fatalf("Number of addresses in cache: got %d, want %d", numCache,
			numAddrs/4)
	}

	// Phase 2:
	// Test internal behavior of how addresses are managed between the new and
	// tried address buckets.  When an address is initially added it should
	// enter the new bucket, and when marked good it should move to the tried
	// bucket.  If the tried bucket is full then it should make room for the
	// newly tried address by moving the old one back to the new bucket.
	n = New("testgood_tried_overflow")
	n.triedBucketSize = 1
	n.getNewBucket = func(netAddr, srcAddr *NetAddress) int {
		return 0
	}
	n.getTriedBucket = func(netAddr *NetAddress) int {
		return 0
	}

	addrA := NewNetAddressFromIPPort(net.ParseIP("173.144.173.1"), 8333, 0)
	addrB := NewNetAddressFromIPPort(net.ParseIP("173.144.173.2"), 8333, 0)
	addrAKey := addrA.Key()
	addrBKey := addrB.Key()

	// Neither address should exist in the address index prior to being
	// added to the address manager.  The new and tried buckets should also be
	// empty.
	if len(n.addrIndex) > 0 {
		t.Fatal("expected address index to be empty prior to adding addresses" +
			" to the address manager")
	}
	if len(n.addrNew[0]) > 0 {
		t.Fatal("expected new bucket to be empty prior to adding addresses" +
			" to the address manager")
	}
	if len(n.addrTried[0]) > 0 {
		t.Fatal("expected tried bucket to be empty prior to adding addresses" +
			" to the address manager")
	}

	n.AddAddresses([]*NetAddress{addrA, addrB}, srcAddr)

	// Both addresses should exist in the address index and new bucket after
	// being added to the address manager.  The tried bucket should be empty.
	if _, exists := n.addrIndex[addrAKey]; !exists {
		t.Fatalf("expected address %s to exist in address index", addrAKey)
	}
	if _, exists := n.addrIndex[addrBKey]; !exists {
		t.Fatalf("expected address %s to exist in address index", addrBKey)
	}
	if _, exists := n.addrNew[0][addrAKey]; !exists {
		t.Fatalf("expected address %s to exist in new bucket", addrAKey)
	}
	if _, exists := n.addrNew[0][addrBKey]; !exists {
		t.Fatalf("expected address %s to exist in new bucket", addrBKey)
	}
	if len(n.addrTried[0]) > 0 {
		t.Fatal("expected tried bucket to contain no elements")
	}

	// Flagging the first address as good should move it to the tried bucket and
	// remove it from the new bucket.
	n.Good(addrA)
	if _, exists := n.addrNew[0][addrAKey]; exists {
		t.Fatalf("expected address %s to not exist in new bucket", addrAKey)
	}
	if len(n.addrTried[0]) != 1 {
		t.Fatal("expected tried bucket to contain exactly one element")
	}
	if n.addrTried[0][0].na.Key() != addrAKey {
		t.Fatalf("expected address %s to exist in tried bucket", addrAKey)
	}

	// Flagging the first address as good again should do nothing.  It should
	// remain in the tried bucket.
	n.Good(addrA)
	if _, exists := n.addrNew[0][addrAKey]; exists {
		t.Fatalf("expected address %s to not exist in new bucket", addrAKey)
	}
	if len(n.addrTried[0]) != 1 {
		t.Fatal("expected tried bucket to contain exactly one element")
	}
	if n.addrTried[0][0].na.Key() != addrAKey {
		t.Fatalf("expected address %s to exist in tried bucket", addrAKey)
	}

	// Flagging the second address as good should cause it to move from the new
	// bucket to the tried bucket.  It should also cause the first address to be
	// evicted from the tried bucket and move back to the new bucket since the
	// tried bucket has been limited in capacity to one element.
	n.Good(addrB)
	if _, exists := n.addrNew[0][addrBKey]; exists {
		t.Fatalf("expected address %s to not exist in the new bucket", addrBKey)
	}
	if len(n.addrTried[0]) != 1 {
		t.Fatalf("expected tried bucket to contain exactly one element - "+
			"got %d", len(n.addrTried[0]))
	}
	if n.addrTried[0][0].na.Key() != addrBKey {
		t.Fatalf("expected address %s to exist in tried bucket", addrBKey)
	}
	if _, exists := n.addrNew[0][addrAKey]; !exists {
		t.Fatalf("expected address %s to exist in the new bucket after being "+
			"evicted from the tried bucket", addrAKey)
	}

	// Phase 3: Test trying to mark an address as good without first adding it
	// to the new bucket
	n = New("testgood_not_new")
	// Directly add an address to the address index without adding it to New
	n.addrIndex[addrAKey] = &KnownAddress{na: addrA, srcAddr: srcAddr, tried: false}
	// Try to mark the address as good
	err := n.Good(addrA)
	if err == nil {
		t.Fatal("expected an error when trying to mark an address as good" +
			"before adding it to the new bucket")
	}

	expectedErrMsg := fmt.Sprintf("%s is not marked as a new address", addrA)
	if err.Error() != expectedErrMsg {
		t.Fatalf("unexpected error str: got %s, want %s", err, expectedErrMsg)
	}
}

// TestSetServices ensures that a known address' services are updated as
// expected and that the services field is not mutated when new services are
// added.
func TestSetServices(t *testing.T) {
	addressManager := New("testSetServices")
	const services = wire.SFNodeNetwork

	// Attempt to set services for an address not known to the address manager.
	notKnownAddr := NewNetAddressFromIPPort(net.ParseIP("1.2.3.4"), 8333, services)
	err := addressManager.SetServices(notKnownAddr, services)
	if err == nil {
		t.Fatal("setting services for unknown address should return error")
	}

	// Add a new address to the address manager.
	netAddr := NewNetAddressFromIPPort(net.ParseIP("1.2.3.4"), 8333, services)
	srcAddr := NewNetAddressFromIPPort(net.ParseIP("5.6.7.8"), 8333, services)
	addressManager.addOrUpdateAddress(netAddr, srcAddr)

	// Ensure that the services field for a network address returned from the
	// address manager is not mutated by a call to SetServices.
	knownAddress := addressManager.GetAddress(natfAny)
	if knownAddress == nil {
		t.Fatal("expected known address, got nil")
	}
	netAddrA := knownAddress.na
	if netAddrA.Services != services {
		t.Fatalf("unexpected network address services - got %x, want %x",
			netAddrA.Services, services)
	}

	// Set the new services for the network address and verify that the
	// previously seen network address netAddrA's services are not modified.
	const newServiceFlags = services << 1
	addressManager.SetServices(netAddr, newServiceFlags)
	netAddrB := knownAddress.na
	if netAddrA == netAddrB {
		t.Fatal("expected known address to have new network address reference")
	}
	if netAddrA.Services != services {
		t.Fatal("netAddrA services flag was mutated")
	}
	if netAddrB.Services != newServiceFlags {
		t.Fatalf("netAddrB has invalid services - got %x, want %x",
			netAddrB.Services, newServiceFlags)
	}
}

// TestAddLocalAddress tests multiple different test cases for adding a local
// address, and ensures that nodes won't accidentally add a non-routable address
// as their local address.
func TestAddLocalAddress(t *testing.T) {
	var tests = []struct {
		name     string
		host     string
		priority AddressPriority
		valid    bool
	}{{
		name:     "non-routable local IPv4 address",
		host:     "192.168.0.100",
		priority: InterfacePrio,
		valid:    false,
	}, {
		name:     "routable IPv4 address",
		host:     "204.124.1.1",
		priority: InterfacePrio,
		valid:    true,
	}, {
		name:     "routable IPv4 address with bound priority",
		host:     "204.124.1.1",
		priority: BoundPrio,
		valid:    true,
	}, {
		name:     "non-routable local IPv6 address",
		host:     "::1",
		priority: InterfacePrio,
		valid:    false,
	}, {
		name:     "non-routable local IPv6 address 2",
		host:     "fe80::1",
		priority: InterfacePrio,
		valid:    false,
	}, {
		name:     "routable IPv6 address",
		host:     "2620:100::1",
		priority: InterfacePrio,
		valid:    true,
	}, {
		name:     "routable TORv3 address",
		host:     torv3Host,
		priority: ManualPrio,
		valid:    true,
	}}

	const testPort = 8333
	const testServices = wire.SFNodeNetwork

	amgr := New("testaddlocaladdress")
	validLocalAddresses := make(map[string]struct{})
	for _, test := range tests {
		addrType, addrBytes := EncodeHost(test.host)
		netAddr, err := NewNetAddressFromParams(addrType, addrBytes, testPort,
			time.Unix(time.Now().Unix(), 0), testServices)
		if err != nil {
			t.Fatalf("%q: failed to create NetAddress: %v", test.name, err)
			return
		}

		result := amgr.AddLocalAddress(netAddr, test.priority)
		if result == nil && !test.valid {
			t.Errorf("%q: address should have been accepted", test.name)
			continue
		}
		if result != nil && test.valid {
			t.Errorf("%q: address should not have been accepted", test.name)
			continue
		}
		if test.valid && !amgr.HasLocalAddress(netAddr) {
			t.Errorf("%q: expected to have local address", test.name)
			continue
		}
		if !test.valid && amgr.HasLocalAddress(netAddr) {
			t.Errorf("%q: expected to not have local address", test.name)
			continue
		}
		if test.valid {
			// Set up data to test behavior of a call to LocalAddresses() for
			// addresses that were added to the local address manager.
			validLocalAddresses[netAddr.Key()] = struct{}{}
		}
	}

	// Ensure that all of the addresses that were expected to be added to the
	// address manager are also returned from a call to LocalAddresses.
	for _, localAddr := range amgr.LocalAddresses() {
		addrType, addrBytes := EncodeHost(localAddr.Address)
		netAddr, err := NewNetAddressFromParams(addrType, addrBytes, testPort,
			time.Unix(time.Now().Unix(), 0), testServices)
		if err != nil {
			t.Fatalf("failed to create NetAddress from LocalAddr: %v", err)
		}
		netAddrKey := netAddr.Key()
		if _, ok := validLocalAddresses[netAddrKey]; !ok {
			t.Errorf("expected to find local address with key %v", netAddrKey)
		}
	}
}

func TestGetBestLocalAddress(t *testing.T) {
	newAddressFromIP := func(ip net.IP) *NetAddress {
		const port = 0
		return NewNetAddressFromIPPort(ip, port, wire.SFNodeNetwork)
	}

	// Not publicly routable.
	privateLocalAddrs := []*NetAddress{
		newAddressFromIP(net.ParseIP("192.168.0.100")),
		newAddressFromIP(net.ParseIP("::1")),
		newAddressFromIP(net.ParseIP("fe80::1")),
	}

	// Publicly routable.
	publicLocalAddrs := []*NetAddress{
		newAddressFromIP(net.ParseIP("204.124.8.100")),
		newAddressFromIP(net.ParseIP("2001:470::1")),
	}

	// TORv3 address.
	torAddrType, torAddrBytes := EncodeHost(torv3Host)
	torAddr, err := NewNetAddressFromParams(torAddrType, torAddrBytes, 0,
		time.Unix(time.Now().Unix(), 0), wire.SFNodeNetwork)
	if err != nil {
		t.Fatalf("failed to create TORv3 NetAddress: %v", err)
	}

	var tests = []struct {
		remoteAddr *NetAddress
		want0      *NetAddress
		want1      *NetAddress
		want2      *NetAddress
		want3      *NetAddress
		want4      *NetAddress
	}{{
		// Remote connection from public IPv4.
		newAddressFromIP(net.ParseIP("204.124.8.1")),
		newAddressFromIP(net.IPv4zero),
		newAddressFromIP(net.IPv4zero),
		newAddressFromIP(net.ParseIP("204.124.8.100")),
		newAddressFromIP(net.IPv4zero),
		torAddr,
	}, {
		// Remote connection from private IPv4.
		newAddressFromIP(net.ParseIP("172.16.0.254")),
		newAddressFromIP(net.IPv4zero),
		newAddressFromIP(net.IPv4zero),
		newAddressFromIP(net.IPv4zero),
		newAddressFromIP(net.IPv4zero),
		newAddressFromIP(net.IPv4zero),
	}, {
		// Remote connection from public IPv6.
		newAddressFromIP(net.ParseIP("2602:100:abcd::102")),
		newAddressFromIP(net.IPv6zero),
		newAddressFromIP(net.IPv6zero),
		newAddressFromIP(net.ParseIP("2001:470::1")),
		newAddressFromIP(net.ParseIP("2001:470::1")),
		torAddr,
	}}

	amgr := New("testgetbestlocaladdress")

	// Test0: Default response (non-routable) when there's no stored addresses.
	for x, test := range tests {
		got := amgr.GetBestLocalAddress(test.remoteAddr, natfAny)
		if !reflect.DeepEqual(test.want0.IP, got.IP) {
			remoteIP := net.IP(test.remoteAddr.IP)
			wantIP := net.IP(test.want0.IP)
			gotIP := net.IP(got.IP)
			t.Errorf("TestGetBestLocalAddress test0 #%d failed for remote address %s: want %s got %s",
				x, remoteIP, wantIP, gotIP)
			continue
		}
	}

	// Store some local addresses which are not publicly accessible.
	for _, localAddr := range privateLocalAddrs {
		amgr.AddLocalAddress(localAddr, InterfacePrio)
	}

	// Test1: Only have private local addresses to share.
	for x, test := range tests {
		got := amgr.GetBestLocalAddress(test.remoteAddr, natfAny)
		if !reflect.DeepEqual(test.want1.IP, got.IP) {
			remoteIP := net.IP(test.remoteAddr.IP)
			wantIP := net.IP(test.want1.IP)
			gotIP := net.IP(got.IP)
			t.Errorf("TestGetBestLocalAddress test1 #%d failed for remote address %s: want %s got %s",
				x, remoteIP, wantIP, gotIP)
			continue
		}
	}

	// Store some local addresses which can be accessed by the public.
	for _, localAddr := range publicLocalAddrs {
		amgr.AddLocalAddress(localAddr, InterfacePrio)
	}

	// Test2: Have some publicly accessible local addresses to share.
	for x, test := range tests {
		got := amgr.GetBestLocalAddress(test.remoteAddr, natfAny)
		if !reflect.DeepEqual(test.want2.IP, got.IP) {
			remoteIP := net.IP(test.remoteAddr.IP)
			wantIP := net.IP(test.want2.IP)
			gotIP := net.IP(got.IP)
			t.Errorf("TestGetBestLocalAddress test2 #%d failed for remote address %s: want %s got %s",
				x, remoteIP, wantIP, gotIP)
			continue
		}
	}

	// Test3: Test when there's a filter to prevent certain addresses being shared.
	for x, test := range tests {
		got := amgr.GetBestLocalAddress(test.remoteAddr, natfOnlyIPv6)
		if !reflect.DeepEqual(test.want3.IP, got.IP) {
			remoteIP := net.IP(test.remoteAddr.IP)
			wantIP := net.IP(test.want3.IP)
			gotIP := net.IP(got.IP)
			t.Errorf("TestGetBestLocalAddress test3 #%d failed for remote address %s: want %s got %s",
				x, remoteIP, wantIP, gotIP)
			continue
		}
	}

	// Test4: Add TORv3 address with ManualPrio
	amgr.AddLocalAddress(torAddr, ManualPrio)
	for x, test := range tests {
		remoteAddr := test.remoteAddr
		want := test.want4
		got := amgr.GetBestLocalAddress(remoteAddr, natfAny)
		if got.Type != want.Type || !reflect.DeepEqual(got.IP, want.IP) {
			t.Errorf("TestGetBestLocalAddress test4 #%d failed for remote address %s: want %s, got %s",
				x, remoteAddr, want, got)
		}
	}
}

// TestIsExternalAddrCandidate makes sure that when a remote peer suggests that
// this node has a certain localAddr, that this function can correctly identify
// if that localAddr is a good candidate to be our public net address.
// IsExternalAddrCandidate should return the expected boolean, as well as the
// expected reach the localAddr has to the remoteAddr.
func TestIsExternalAddrCandidate(t *testing.T) {
	rfc4380IPAddress := rfc4380Net.IP.String()
	rfc3964IPAddress := rfc3964Net.IP.String()
	rfc6052IPAddress := rfc6052Net.IP.String()
	rfc6145IPAddress := rfc6145Net.IP.String()

	tests := []struct {
		name          string
		localAddr     string
		remoteAddr    string
		expectedBool  bool
		expectedReach NetAddressReach
	}{{
		name:          "remote peer suggested a local address",
		localAddr:     "127.0.0.1",
		remoteAddr:    routableIPv4Addr,
		expectedBool:  false,
		expectedReach: Unreachable,
	}, {
		name:          "rfc4380 to rfc4380",
		localAddr:     rfc4380IPAddress,
		remoteAddr:    rfc4380IPAddress,
		expectedBool:  true,
		expectedReach: Teredo,
	}, {
		name:          "non-routable ipv4 to rfc4380",
		localAddr:     nonRoutableIPv4Addr,
		remoteAddr:    rfc4380IPAddress,
		expectedBool:  false,
		expectedReach: Default,
	}, {
		name:          "routable ipv4 to rfc4380",
		localAddr:     routableIPv4Addr,
		remoteAddr:    rfc4380IPAddress,
		expectedBool:  true,
		expectedReach: Ipv4,
	}, {
		name:          "routable ipv6 to rfc4380",
		localAddr:     routableIPv6Addr,
		remoteAddr:    rfc4380IPAddress,
		expectedBool:  true,
		expectedReach: Ipv6Weak,
	}, {
		name:          "routable ipv4 to routable ipv4",
		localAddr:     routableIPv4Addr,
		remoteAddr:    routableIPv4Addr,
		expectedBool:  true,
		expectedReach: Ipv4,
	}, {
		name:          "routable ipv6 to routable ipv4",
		localAddr:     routableIPv6Addr,
		remoteAddr:    routableIPv4Addr,
		expectedBool:  false,
		expectedReach: Unreachable,
	}, {
		name:          "non-routable ipv4 to routable ipv6",
		localAddr:     nonRoutableIPv4Addr,
		remoteAddr:    routableIPv6Addr,
		expectedBool:  false,
		expectedReach: Default,
	}, {
		name:          "non-routable ipv6 to routable ipv6",
		localAddr:     nonRoutableIPv6Addr,
		remoteAddr:    routableIPv6Addr,
		expectedBool:  false,
		expectedReach: Default,
	}, {
		name:          "non-routable ipv4 to routable ipv6",
		localAddr:     nonRoutableIPv4Addr,
		remoteAddr:    routableIPv6Addr,
		expectedBool:  false,
		expectedReach: Default,
	}, {
		name:          "routable ipv4 to non-routable ipv6",
		localAddr:     routableIPv4Addr,
		remoteAddr:    nonRoutableIPv6Addr,
		expectedBool:  false,
		expectedReach: Unreachable,
	}, {
		name:          "routable ivp6 rfc4380 to routable ipv6",
		localAddr:     rfc4380IPAddress,
		remoteAddr:    routableIPv6Addr,
		expectedBool:  true,
		expectedReach: Teredo,
	}, {
		name:          "routable ipv4 to routable ipv6",
		localAddr:     routableIPv4Addr,
		remoteAddr:    routableIPv6Addr,
		expectedBool:  true,
		expectedReach: Ipv4,
	}, {
		name:          "tunnelled ipv6 rfc3964 to routable ipv6",
		localAddr:     rfc3964IPAddress,
		remoteAddr:    routableIPv6Addr,
		expectedBool:  true,
		expectedReach: Ipv6Weak,
	}, {
		name:          "tunnelled ipv6 rfc6052 to routable ipv6",
		localAddr:     rfc6052IPAddress,
		remoteAddr:    routableIPv6Addr,
		expectedBool:  true,
		expectedReach: Ipv6Weak,
	}, {
		name:          "tunnelled ipv6 rfc6145 to routable ipv6",
		localAddr:     rfc6145IPAddress,
		remoteAddr:    routableIPv6Addr,
		expectedBool:  true,
		expectedReach: Ipv6Weak,
	}, {
		name:          "torv3 to torv3",
		localAddr:     torAddress,
		remoteAddr:    torAddress,
		expectedBool:  false,
		expectedReach: Private,
	}, {
		name:          "routable ipv4 to torv3",
		localAddr:     routableIPv4Addr,
		remoteAddr:    torAddress,
		expectedBool:  true,
		expectedReach: Ipv4,
	}, {
		name:          "non-routable ipv4 to torv3",
		localAddr:     nonRoutableIPv4Addr,
		remoteAddr:    torAddress,
		expectedBool:  false,
		expectedReach: Default,
	}, {
		name:          "routable ipv6 to torv3",
		localAddr:     routableIPv6Addr,
		remoteAddr:    torAddress,
		expectedBool:  true,
		expectedReach: Default,
	}, {
		name:          "non-routable ipv6 to torv3",
		localAddr:     nonRoutableIPv6Addr,
		remoteAddr:    torAddress,
		expectedBool:  false,
		expectedReach: Default,
	}, {
		name:          "torv3 to routable ipv4",
		localAddr:     torAddress,
		remoteAddr:    routableIPv4Addr,
		expectedBool:  false,
		expectedReach: Ipv4,
	}, {
		name:          "torv3 to routable ipv6",
		localAddr:     torAddress,
		remoteAddr:    routableIPv6Addr,
		expectedBool:  false,
		expectedReach: Ipv6Strong,
	}}

	createNetAddr := func(addr string) *NetAddress {
		addrType, addrBytes := EncodeHost(addr)
		if addrType == UnknownAddressType {
			t.Fatalf("unable to parse address: %s", addr)
		}
		na, err := NewNetAddressFromParams(addrType, addrBytes, 8333,
			time.Time{}, wire.SFNodeNetwork)
		if err != nil {
			t.Fatalf("failed to create NetAddress from %s: %v", addr, err)
		}
		return na
	}

	addressManager := New("TestIsExternalAddrCandidate")
	for _, test := range tests {
		localNa := createNetAddr(test.localAddr)
		remoteNa := createNetAddr(test.remoteAddr)

		goodReach, reach := addressManager.IsExternalAddrCandidate(localNa, remoteNa)
		if goodReach != test.expectedBool {
			t.Errorf("%q: unexpected return value for bool - want '%v', got '%v'",
				test.name, test.expectedBool, goodReach)
			continue
		}
		if reach != test.expectedReach {
			t.Errorf("%q: unexpected return value for reach - want '%v', got '%v'",
				test.name, test.expectedReach, reach)
		}
	}
}

// TestGetAddressWithFilter ensures that GetAddress returns addresses matching
// the provided filter.
func TestGetAddressWithFilter(t *testing.T) {
	ipv4Addr := NewNetAddressFromIPPort(net.ParseIP(routableIPv4Addr), 8333, 0)
	ipv6Addr := NewNetAddressFromIPPort(net.ParseIP(routableIPv6Addr), 8333, 0)

	addrType, addrBytes := EncodeHost(torv3Host)
	torv3Addr, _ := NewNetAddressFromParams(addrType, addrBytes, 8333,
		time.Unix(time.Now().Unix(), 0), 0)

	tests := []struct {
		name      string
		addresses []*NetAddress
		filter    NetAddressTypeFilter
		wantType  NetAddressType
		wantNil   bool
	}{{
		name:      "returns address matching IPv4 filter",
		addresses: []*NetAddress{ipv4Addr, ipv6Addr},
		filter:    natfOnlyIPv4,
		wantType:  IPv4Address,
	}, {
		name:      "returns address matching IPv6 filter",
		addresses: []*NetAddress{ipv4Addr, ipv6Addr},
		filter:    natfOnlyIPv6,
		wantType:  IPv6Address,
	}, {
		name:      "returns address matching TORv3 filter",
		addresses: []*NetAddress{ipv4Addr, ipv6Addr, torv3Addr},
		filter:    natfOnlyTORv3,
		wantType:  TORv3Address,
	}, {
		name:      "returns nil when no matching IPv4 addresses",
		addresses: []*NetAddress{ipv6Addr},
		filter:    natfOnlyIPv4,
		wantNil:   true,
	}, {
		name:      "returns nil when no matching IPv6 addresses",
		addresses: []*NetAddress{ipv4Addr},
		filter:    natfOnlyIPv6,
		wantNil:   true,
	}, {
		name:      "returns nil when address manager empty",
		addresses: []*NetAddress{},
		filter:    natfAny,
		wantNil:   true,
	}}

	for _, test := range tests {
		amgr := New("TestGetAddressWithFilter")
		amgr.AddAddresses(test.addresses, ipv4Addr)

		ka := amgr.GetAddress(test.filter)

		if test.wantNil {
			if ka != nil {
				t.Errorf("%q: expected nil, got address: %v", test.name, ka.NetAddress())
			}
			continue
		}

		if ka == nil {
			t.Errorf("%q: expected address, got nil", test.name)
			continue
		}

		// Verify the address type matches expected type.
		gotType := ka.NetAddress().Type
		if gotType != test.wantType {
			t.Errorf("%q: unexpected address type: got %v, want %v",
				test.name, gotType, test.wantType)
		}
	}
}
