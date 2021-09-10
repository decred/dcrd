// Copyright (c) 2019-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package types implements concrete types for marshalling to and from the dcrd
JSON-RPC commands, return values, and notifications.

When communicating via the JSON-RPC protocol, all requests and responses must be
marshalled to and from the wire in the appropriate format.  This package
provides data structures and primitives that are registered with dcrjson to ease
this process.  An overview specific to this package is provided here, however it
is also instructive to read the documentation for the dcrjson package
(https://pkg.go.dev/github.com/decred/dcrd/dcrjson/v4).

Marshalling and Unmarshalling

The types in this package map to the required parts of the protocol as discussed
in the dcrjson documentation

  - Request Objects (type Request)
    - Commands (type <Foo>Cmd)
    - Notifications (type <Foo>Ntfn)
  - Response Objects (type Response)
    - Result (type <Foo>Result)

To simplify the marshalling of the requests and responses, the
dcrjson.MarshalCmd and dcrjson.MarshalResponse functions may be used.  They
return the raw bytes ready to be sent across the wire.

Unmarshalling a received Request object is a two step process:
  1) Unmarshal the raw bytes into a dcrjson.Request struct instance via
     json.Unmarshal
  2) Use dcrjson.ParseParams on the Method and Params fields of the unmarshalled
     Request to create a concrete command or notification instance with all
     struct fields set accordingly.

This approach is used since it provides the caller with access to the additional
fields in the request that are not part of the command such as the ID.

Unmarshalling a received Response object is also a two step process:
  1) Unmarshal the raw bytes into a dcrjson.Response struct instance via
     json.Unmarshal
  2) Depending on the ID, unmarshal the Result field of the unmarshalled
     Response to create a concrete type instance

As above, this approach is used since it provides the caller with access to the
fields in the response such as the ID and Error.

Command Creation

This package provides two approaches for creating a new command.  This first,
and preferred, method is to use one of the New<Foo>Cmd functions.  This allows
static compile-time checking to help ensure the parameters stay in sync with
the struct definitions.

The second approach is the dcrjson.NewCmd function which takes a method
(command) name and variable arguments.  Since this package registers all of its
types with dcrjson, the function will recognize them and includes full checking
to ensure the parameters are accurate according to provided method, however
these checks are, obviously, run-time which means any mistakes won't be found
until the code is actually executed.  However, it is quite useful for
user-supplied commands that are intentionally dynamic.

Help Generation

To facilitate providing consistent help to users of the RPC server, the dcrjson
package exposes the GenerateHelp and function which uses reflection on commands
and notifications registered by this package, as well as the provided expected
result types, to generate the final help text.

In addition, the dcrjson.MethodUsageText function may be used to generate
consistent one-line usage for registered commands and notifications using
reflection.
*/
package types
