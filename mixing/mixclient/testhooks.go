// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixclient

type hook string

type hookFunc func(*Client, *pairedSessions, *sessionRun, *peer)

const (
	hookBeforeRun           hook = "before run"
	hookBeforePeerCTPublish hook = "before CT publish"
	hookBeforePeerSRPublish hook = "before SR publish"
	hookBeforePeerDCPublish hook = "before DC publish"
)
