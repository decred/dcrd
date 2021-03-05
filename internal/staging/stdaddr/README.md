stdaddr
=======

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/internal/staging/stdaddr)

## Decred Address Overview

An address is a human-readable representation of a possible destination for a
payment.  More specifically, it encodes information needed to create a smart
contract via a
[transaction script](https://devdocs.decred.org/developer-guides/transactions/txscript/overview/)
that imposes constraints required to redeem funds.

Under the hood, an address typically consists of 4 primary components:

- The network for which the address is valid
- The data necessary to create a payment script with the appropriate constraints
- The scripting language version of the aforementioned script
- A checksum to prevent errors due to improper entry

It is also worth keeping in mind that the scripting language version may be
implicit depending on the specific concrete encoding of the address.

### Supported Version 0 Addresses

The following table lists the `version 0` address types this package supports
along with whether the type is supported by the staking system, whether it has
an associated `hash160`, and some additional notes and recommendations:

Version 0 Address Type    | Staking? | Hash160? | Notes / Recommendations
--------------------------|----------|----------|----------------------------------
p2pk-ecdsa-secp256k1      |    N     |    N     | Prefer p2pkh in on-chain txns [1]
p2pk-ed25519              |    N     |    N     | Not recommended [2]
p2pk-schnorr-secp256k1    |    N     |    N     | Prefer p2pkh, single party [1,3]
**p2pkh-ecdsa-secp256k1** |  **Y**   |  **Y**   | **Preferred v0 address**
p2pkh-ed25519             |    N     |    Y     | Not recommended [2]
p2pkh-schnorr-secp256k1   |    N     |    Y     | Only use with single party [3]
p2sh                      |    Y     |    Y     | -

Abbreviations:

* p2pk = Pay-to-public-key
* p2pkh = Pay-to-public-key-hash
* p2sh = Pay-to-script-hash

Notes:

- [1] Pay to public key addresses are only recommended for use in sharing public
      keys via off-chain protocols such as multi-party signature creation.  Pay
      to public key hash addresses should be used for on-chain transactions to
      avoid revealing the public key until the funds are redeemed.
- [2] Version 0 addresses involving Ed25519 are not recommended due to lack of
      support for safe hierarchical derivation and the requirement for a legacy
      implementation of the curve that has known issues.
- [3] Version 0 addresses involving Schnorr signatures are only recommended for
      use with single parties as extra care must be taken when used with
      multiple parties to avoid potential theft of funds by malicious
      counterparties.

## Note about Standardness vs Consensus

This package is named `stdaddr` to clearly convey that addresses are a
**standardized** construction built on top of the underlying scripting system
that is enforced by the consensus rules.  The specific scripts that the
addresses represent are referred to as standard scripts since they are
recognized forms that most software considers standard by policy.  However, the
consensus rules for regular transactions must support so called non-standard
scripts as well (any script that is not one of the recognized forms).

This distinction between standardness and consensus is extremely important for
developers working on the consensus rules, since consensus must **NOT** use
addresses directly and instead must work with the actual underlying scripts.  An
address is a human-readable representation of an underlying script along with
additional metadata to help prevent misuse and typographical errors.  The
underlying scripts are enforced by consensus, not the other way around!

## Package Usage Primer

The overall design of this package is based on a few key interfaces to provide a
generalized capabilities-based approach to working with the various types of
addresses along with unique types for each supported address type that implement
said interfaces.

Interacting with addresses typically falls into the following categories:

- Instantiating addresses
  - Decoding existing addresses
  - Creating addresses of the types which correspond to desired encumbrances
- Generating the payment script and associated version for use in regular
  transaction outputs
- Determining additional capabilities of an address by type asserting supported
  interfaces

### Address Versions

All software that works with sending funds to addresses must be able to produce
payment scripts that correspond to the same scripting language version the
original creator of the address (aka the recipient) supports as they might
otherwise not be able to actually redeem the funds.

This requirement creates a one-to-one correspondence between a given address and
the underlying scripting language version.  Since the original base58-based
address format does not encode a scripting language version and are implicitly
expected to create version 0 scripts, they are referred to as version 0
addresses.

It is expected that new version addresses will be introduced in the future that
change the address format to improve some of the shortcomings of base58
addresses as well as properly encode a scripting language version.

### Instantiating Addresses

In order to provide a more ergonomic API depending on the specific needs of
callers, this package offers two approaches to instantiating addresses:

1. Methods that accept and instantiate addresses for any supported version.  For
   example, `DecodeAddress` and `NewAddressPubKeyHashEcdsaSecp256k1`.
2. Methods that accept and instantiate addresses for a specific version.  For
   example, `DecodeAddressV0` and `NewAddressPubKeyHashEcdsaSecp256k1V0`.

The first approach is likely suitable for most callers as it is intended to
support all historical address types as well as allow easily specifying a script
version from a dynamic variable.  For example, applications involving historical
data analytics, such as block explorers, should prefer this approach.

However, callers might wish to only support decoding and creating specific
address versions in which case the second approach is more suitable.  Further,
when it comes to creating addresses, since not all adddress types will
necessarily be supported by all scripting language versions, the second approach
makes it clear from the API exactly which types of addresses are supported for a
given version at compile time whereas the first approach requires the caller to
check for errors at run time.

### Generating Payment Scripts and Displaying a Human-Readable Address String

In order to generalize support for the various standard address types, this
package provides an `Address` interface which is the primary interface
implemented by all supported address types.

Use the `PaymentScript` method to obtain the scripting language version
associated with the address along with a script to pay a transaction output to
the address and the `Address` method to get the human-readable string encoding
of the address.

### Address Use in the Staking System

A distinguishing aspect of the staking system in Decred is that it imposes
additional constraints on the types of addresses that can be used versus those
for regular transactions.  This is because scripts in the staking system are
tightly controlled by consensus to only permit very specific scripts, unlike
regular transactions which allow scripts composed of any sequence of
[opcodes](https://devdocs.decred.org/developer-guides/transactions/txscript/opcodes/)
that evaluate to true.

In order to support this difference, this package provides the `StakeAddress`
interface which is only implemented by the address types that are supported by
the staking system.  This allows callers to determine if an address can be used
in the staking system by type asserting the interface and then making use of the
relevant methods provided by the interface to interact with the staking system.
Refer to the code examples to see this in action as well as the API
documentation of the interface methods for further details.

### Converting Public Key to Public Key Hash Addresses

It is often necessary to share public keys in interactive and manual off-chain
protocols, such as multi-party signature creation.  Since raw public keys are
valid for any network, it is generally less error prone and more convenient to
use human-readable public key addresses in this case because they ensure the
correct network is involved as well as help prevent entry errors.

However, whenever dealing with on-chain transactions, public key hash addresses
should typically be used instead to avoid revealing the public key until the
funds are redeemed.

With the aim of supporting easy conversion of a public key address to a public
key hash address for the cases when the caller already has the former available,
this package provides the `AddressPubKeyHasher` interface which is only
implemented by the public key address types.  This allows callers to determine
if an address can be converted to its public key hash variant by type asserting
the interface and then convert it by making use of the `AddressPubKeyHash`
method provided by the interface.

### Hash160 Use in Addresses

The term `Hash160` is used as shorthand to refer to a hash that is created via a
combination of [RIPEMD-160](../../../crypto/ripemd160) and
[BLAKE256](../../../crypto/blake256).  Specifically, it is the result of
`RIPEMD-160(BLAKE256(data))`.

For address types that involve Hash160 hashes, such as version 0 public key hash
and script hash addresses, it can be convenient for callers to extract the
associated hash.

To enable the hash extraction, this package provides the `Hash160er` interface
which is only implemented by the address types that involve Hash160.  This
allows callers to determine if an address involves a Hash160 by type asserting
the interface and then extract it by making use of the `Hash160` method provided
by the interface.

## Installation and Updating

This package is internal and therefore is neither directly installed nor needs
to be manually updated.

## Examples

* [DecodeAddress](https://pkg.go.dev/github.com/decred/dcrd/internal/staging/stdaddr#example-DecodeAddress)

  Demonstrates decoding addresses, generating their payment scripts and
  associated scripting language versions, determining supported capabilities by
  checking if interfaces are implemented, obtaining the associated underlying
  hash160 for addresses that support it, converting public key addresses to
  their public key hash variant, and generating stake-related scripts for
  addresses that can be used in the staking system.

## License

Package stdaddr is licensed under the [copyfree](http://copyfree.org) ISC
License.