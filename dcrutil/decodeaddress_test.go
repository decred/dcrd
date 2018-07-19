package dcrutil

import (
	"testing"
)

// TestDecodeAddress checks DecodeAddress is able to decode a real existing address
// https://explorer.dcrdata.org/tx/b473afe8618dedcd261566509cf25d5a2c2bd9c459265b5c0ae3dcdcdc79cc88
func TestDecodeAddress(t *testing.T) {
	addressToDecode := "b473afe8618dedcd261566509cf25d5a2c2bd9c459265b5c0ae3dcdcdc79cc88"
	_, err := DecodeAddress(addressToDecode)
	if err != nil {
		t.Errorf("DecodeAddress() failed: %v", err)
	}
}
