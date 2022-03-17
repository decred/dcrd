### Table of Contents
1. [Per-RPC Versioning](#PerRPCVersioning)<br />
2. [Versioning Scheme](#VersioningScheme)<br />
3. [Versioned Endpoint Accessibilty Scheme](#AccessibiltyScheme)<br />
4. [Versioned Enpoint Example](#Example)<br />
5. [Versioned Enpoint Deprecation And Removal Scheme](#DeprecationAndRemovalScheme)<br />
6. [Versioned Endpoint JSON-RPC Unit Testing](#UnitTesting)<br />
7. [Versioned JSON-RPC Documentation](#Documentation)<br />


<a name="PerRPCVersioning" />

### 1. Per-RPC Versioning

To streamline API access in the process of transitioning between release 
versions, decred's RPC endpoints will be versioned individually for each 
JSON-RPC endpoint. 
Per-RPC versioning provides the flexibility needed to isolate the development 
of JSON-RPC endpoints. This allows endpoints to evolve individually and 
exclusively of each other.

<a name="VersioningScheme" />

### 2. Versioning Scheme

The versioning scheme constitutes appending the version to the type name or 
handler name. 
 - JSON-RPC types will have the version spliced between the method name and 
 `Cmd` suffix. For example, the `V2` version of the type `ExistsAddressCmd` 
 becomes `ExistsAddressV2Cmd`.
 
 - JSON-RPC handler names will have the version appended to the handler name. 
 For example, the `V2` version of the handler `handleExistsAddress` becomes
`handleExistsAddressV2`. 

 - JSON-RPC method names will have the lowercase version appended to the 
 method name. For example the `V2` version of method name `existsaddress` 
 becomes `existsaddressv2`. 

<a name="AccessibiltyScheme" />

### 3. Versioned Endpoint Accessibilty Scheme

A versioned JSON-RPC endpoint infers there is a current version and an older 
version of the endpoint. To ensure a smooth transition between JSON-RPC 
versions for consumers, both versions should be accessible. The current and 
older versions should be explicitly accessible via their respective method 
names while the most recent version becomes the version defaulted to. 
Using `existsaddress` as an example: 
 - The `V2` version of the endpoint, which is the current version, should be 
 explictly accessible via `existsaddressv2`.
 
 - The `V1` version of the endpoint, which is now the older version, should be 
 explicitly accessible via `existsaddressv1`.

 - The default method name, `existsaddress` will resolve to the current version 
 of the endpoint which is `existsaddressv2` in this case.

 This scheme will be enforced via the JSON-RPC type and handler registrations. 

<a name="Example" />

### 4. Versioned Enpoint Example

Taking into consideration the accessibility scheme outlined above, implementing 
the `V2` version of the `existaddress` JSON-RPC endpoint will comprise of the 
following components: 

```go
     // JSON-RPC versioned endpoint command definition.

    // ExistsAddressV2Cmd defines the existsaddressv2 JSON-RPC command.
    type ExistsAddressV2Cmd struct {
        Address       string
        PaymentScript string
    }
    // ...
```
```go
    // ExistsAddress JSON-RPC type registrations.
 	dcrjson.MustRegister(Method("existsaddress"), (*ExistsAddressV2Cmd)(nil), flags)
	dcrjson.MustRegister(Method("existsaddressv1"), (*ExistsAddressCmd)(nil), flags)
	dcrjson.MustRegister(Method("existsaddressv2"), (*ExistsAddressV2Cmd)(nil), flags)
    // ...
```

```go
    // ExistsAddressV2 JSON-RPC Handler definition.

    // handleExistsAddress implements the existsaddress command.
    //
    // Deprecated: This will be removed on the next versioned endpoint
    // implementation.
    func handleExistsAddress(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
        if s.cfg.ExistsAddresser == nil {
            return nil, rpcInternalError("Exists address index disabled",
                "Configuration")
        }

        c := cmd.(*types.ExistsAddressCmd)
        // ...
    }

    // handleExistsAddressV2 implements the existsaddressv2 command.
    func handleExistsAddressV2(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
        if s.cfg.ExistsAddresser == nil {
            return nil, rpcInternalError("Exists address index disabled",
                "Configuration")
        }

        c := cmd.(*types.ExistsAddressV2Cmd)
        // ...
    }

``` 

```go
    // ExistsAddress JSON-RPC handler registrations.
    var rpcHandlersBeforeInit = map[types.Method]commandHandler{
        // ...
        "existsaddress":         handleExistsAddressV2,
        "existsaddressv1":       handleExistsAddress,
        "existsaddressv2":       handleExistsAddressV2,
        // ...
    }
```

```go
// ExistsAddress JSON-RPC result type registrations.
var rpcResultTypes = map[types.Method][]interface{}{
    // ...
	"existsaddress":         {(*bool)(nil)},
	"existsaddressv1":       {(*bool)(nil)},
	"existsaddressv2":       {(*bool)(nil)},
    // ...
}
```

<a name="DeprecationAndRemovalScheme" />

### 5. Versioned Enpoint Deprecation And Removal Scheme

Only two versioned endpoints will be maintained per JSON-RPC endpoint. The 
introduction of a versioned endpoint immediately marks the oldest maintained 
endpoint for deprecation. This is expected to be done by the contributor in the 
pull request introducing the new versioned endpoint. Using `existsaddress` as 
an example, with the introduction of the versioned endpoint `existsaddressv2` 
`existsaddressv1` will be marked as deprecated and eventually removed when 
the next versioned endpoint `existsaddressv3` is introduced.

<a name="UnitTesting" />

### 6. Versioned Endpoint JSON-RPC Unit Testing

All versioned JSON-RPC handlers are required to have their own set of unit tests.
Contributors versioning these endpoints will be required to include unit tests 
in the pull request introducing the versioned JSON-RPC endpoint. Usually the 
existing unit tests for the endpoint being versioned will serve as the base 
for the new set of tests, these units will be copied and modified as needed.

Using `existsaddress` as an example, a new unit test for the versioned endpoint
`existsaddressv2` will be added to the rpcserver test set. Note that the unit 
test for the `existaddress` will remain in the rpcserver unit test set. It will
only be removed when the deprecated endpoint is being removed, when 
`existsaddressv3` is introduced.

```go
// handleExistsAddressV2 JSON-RPC unit test
func TestHandleExistsAddressV2(t *testing.T) {
	t.Parallel()

	validAddr := "DcurAwesomeAddressmqDctW5wJCW1Cn2MF"
	validPaymentScript := "a914f59833f104faa3c7fd0c7dc1e3967fe77a9c152387"
	wrongPaymentScript := "a114f59833f104faa3c7fd0c7dc1e3967fe77a9c152354"
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleExistsAddressV2: ok, index is synced",
		handler: handleExistsAddressV2,
		cmd: &types.ExistsAddressV2Cmd{
			Address:       validAddr,
			PaymentScript: validPaymentScript,
		},
		result: false,
	}, {
		name:    "handleExistsAddressV2: wrong payment script, index is synced",
		handler: handleExistsAddressV2,
		cmd: &types.ExistsAddressV2Cmd{
			Address:       validAddr,
			PaymentScript: wrongPaymentScript,
		},
		result: false,
	}
    // ...
    })
}

// ...
```

<a name="Documentation" />

### 7. Versioned JSON-RPC Documentation

Documentation for the newly versioned JSON-RPC endpoint will be expected in the 
pull request introducing the versioned endpoint. Documentation for versioned 
endpoints are expected to be grouped, contributors should add the new 
documentation entry after the old version's documentation. The old version's 
handler documentation should also indicate it has been deprecated in favour of 
the new version and scheduled to the removed on the introduction of the next 
endpoint version.

Using `existsaddress` as an example, with the introduction of `existsaddressv2` 
the JSON-RPC API documentation will be updated to include documentation for 
`existsaddressv2`.

```
    ...

    ====existsaddressv2====
    {|
    !Method
    |existsaddressv2
    |-
    !Parameters
    |
    # <code>address</code>: <code>(string, required)</code> The address to check.
    # <code>paymentscript</code>: <code>(string, required)</code> The hex encoded payment script to check.
    |-
    !Description
    |Returns the existence of the provided address with the provided payment script.
    |-
    !Returns
    |<code>boolean</code>
    |-
    !Example Return
    |<code>true</code>
    |}

    ----
    
    ...

    |[[#existsaddress|existsaddress]]
    |Y
    |Returns the existence of the provided address.
    |-
    |[[#existsaddressv2|existsaddressv2]]
    |Y
    |Returns the existence of the provided address with the provided payment script.
    |-
    ...
```

``` go
    // RPC Server Help Documentation
	
    // ...
    // ExistsAddressCmd help.
	"existsaddress--synopsis": "Test for the existence of the provided address",
	"existsaddress-address":   "The address to check",
	"existsaddress--result0":  "Bool showing if address exists or not",

    // ExistsAddressV2Cmd help.
	"existsaddressv2--synopsis":     "Test for the existence of the provided address and whether it has the provided payment script",
	"existsaddressv2-address":       "The address to check",
	"existsaddressv2-paymentscript": "The address payment script to check",
	"existsaddressv2--result0":      "Bool showing if address exists or not",
    // ...
```

With the outlined schemes adhered to, the versioned endpoint should be ready 
for use once `dcrctl` is rebuilt once the `rpc/jsonrpc/types` dependency is 
replaced with the updated local changes.
