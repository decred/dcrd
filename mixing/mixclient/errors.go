package mixclient

import "errors"

var (
	ErrTooFewPeers = errors.New("not enough peers required to mix")

	ErrUnknownPRs = errors.New("unable to participate in reformed session referencing unknown PRs")
)

// testPeerBlamedError describes the error condition of a misbehaving peer
// being removed from a mix run following blame assignment.  It should never
// occur outside of unit tests.
type testPeerBlamedError struct {
	p *peer
}

func (e *testPeerBlamedError) Error() string {
	return "peer removed during blame assignment"
}
