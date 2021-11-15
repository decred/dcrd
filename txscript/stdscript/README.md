stdscript
=========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/txscript/v4/stdscript)

## Overview

This package provides facilities for identifying and extracting data from
[transaction scripts](https://devdocs.decred.org/developer-guides/transactions/txscript/overview/)
that are considered standard by the default policy of most nodes.

### Understanding Standardness Versus Consensus

Under the hood, all regular transactions make use of transaction scripts with
consensus-enforced semantics determined by an associated scripting language
version in order to create smart contracts that impose constraints required to
redeem funds.

The consensus rules consist of executing these transaction scripts in a virtual
machine and further dictate that the result of the execution must evaluate to
true.  In particular, this means that scripts may be composed of any sequence of
supported
[opcodes](https://devdocs.decred.org/developer-guides/transactions/txscript/opcodes/)
so long as they evaluate to true.

Recall that the full transaction script that is executed is the concatenation of
two parts:

1. The output script of a transaction (also known as the public key script)
2. The input script of a transaction spending the aforementioned output (also
   known as the signature script)

Standard transaction scripts are **public key scripts** which are only composed
of a well-defined sequence of opcodes that are widely used to achieve a specific
goal such as requiring signatures to spend funds or anchoring data into the
chain.  In other words, standard scripts are a restricted subset of all possible
public key scripts such that they conform to well-defined templates of a
recognized form.

In addition, in order to improve overall efficiency of the network and limit
potential denial of service attacks, the default **policy** of most nodes is to
only accept and relay scripts which are considered standard, however, that in no
way implies a transaction with non-standard scripts (any script that is not one
of the recognized forms) is invalid per the consensus rules.

This distinction between standardness and consensus is extremely important for
developers working on the consensus rules since what is considered a standard
script can change at any time, while script execution for a given scripting
language version must remain valid forever!  To be explicit, the consensus rules
must **NOT** reference or interact with code dealing with standard scripts
directly in any way as that would lead to breaking perfectly valid scripts that
may no longer be considered standard by policy.

This package is named `stdscript` to clearly convey that these scripts are
particular recognized forms that most software considers standard by policy and
therefore, as previously stated, must **NOT** be referenced by consensus code.

### Pitfall with Special Script Types Enforced by Consensus

One important point of potential confusion to be cognizant of is that some of
the script types, such as pay-to-script-hash, as well as all of the
stake-related scripts, additionally have special consensus enforcement which
means the consensus rules must also recognize them.  In other words, they are
both consensus scripts as well as recognized standard scripts.

This is particularly relevant to the Decred staking system since a
distinguishing aspect of it is that its scripts are tightly controlled by
consensus to only permit very specific scripts, unlike regular transactions
which allow scripts composed of any sequence of opcodes that evaluate to true.

Consequently, it might be tempting to make use of the code in this package for
identifying them for the purposes of enforcement since it is conveniently
available, however, as previously stated, developers must **NOT** reference any
code in this package from code that enforces consensus rules as this package
will change with policy over time and therefore would lead to breaking the
consensus rules.

### Recognized Version 0 Standard Scripts

The following table lists the `version 0` script types this package recognizes
along with whether the type only applies to the staking system, and the type of
the raw data that can be extracted:

Version 0 Script Type     | Stake? | Data Type
--------------------------|--------|--------------------
p2pk-ecdsa-secp256k1      |    N   | `[]byte`
p2pk-ed25519              |    N   | `[]byte`
p2pk-schnorr-secp256k1    |    N   | `[]byte`
p2pkh-ecdsa-secp256k1     |    N   | `[]byte`
p2pkh-ed25519             |    N   | `[]byte`
p2pkh-schnorr-secp256k1   |    N   | `[]byte`
p2sh                      |    N   | `[]byte`
ecdsa-multisig            |    N   | `MultiSigDetailsV0`
nulldata                  |    N   | `[]byte`
stake submission p2pkh    |    Y   | `[]byte`
stake submission p2sh     |    Y   | `[]byte`
stake generation p2pkh    |    Y   | `[]byte`
stake generation p2sh     |    Y   | `[]byte`
stake revocation p2pkh    |    Y   | `[]byte`
stake revocation p2sh     |    Y   | `[]byte`
stake change p2pkh        |    Y   | `[]byte`
stake change p2sh         |    Y   | `[]byte`
treasury add              |    Y   | -
treasury generation p2pkh |    Y   | `[]byte`
treasury generation p2sh  |    Y   | `[]byte`

Abbreviations:

* p2pk = Pay-to-public-key
* p2pkh = Pay-to-public-key-hash
* p2sh = Pay-to-script-hash

## Package Usage Primer

Interacting with existing standard scripts typically falls into the following
categories:

- Analyzing them to determine their type
- Extracting data

### Determining Script Type

In order to provide a more ergonomic and efficient API depending on the specific
needs of callers, this package offers three approaches to determining script
types:

1. Determining the script type as a `ScriptType` enum.  For example,
   `DetermineScriptType` and `DetermineScriptTypeV0`.
2. Determining if a script is a specific type directly.  For example,
   `IsPubKeyHashScript`, `IsPubKeyHashScriptV0`, and `IsScriptHashScript`.
3. Determining if a script is a specific type by checking if the relevant
   version-specific data was extracted.  For example, `ExtractPubKeyHashV0` and
   `ExtractScriptHashV0`.

The first approach is most suitable for callers that want to work with and
display information about a wide variety of script types, such as block
explorers.

The second approach is better suited to callers that are looking for a very
specific script type as it is more efficient to check for a single type instead
of checking all types to produce an enum as in the first approach.

The third approach is aimed more at callers that need access to the relevant
data from the script in addition to merely determining the type.  Unless noted
otherwise in the documentation for a specific method, the data extraction
methods only return data if the provided script is actually of the associated
type.  For example, when a wallet needs to determine if a transaction script is
a pay-to-pubkey-hash script that involves a particular public key hash, instead
of calling `IsPubKeyHashScriptV0` (or worse calling `DetermineScriptType` and
checking the returned enum type) followed by `ExtractPubKeyHashV0`, it is more
efficient to simply call `ExtractPubKeyHashV0` and check if the data returned is
not `nil` followed by using said data.

It is also worth noting that the first two approaches offer API methods that
accept the scripting language version as a parameter as well as their
version-specific variants that make it clear from the API exactly which script
types are supported for a given version at compile time.  The third method, on
the other hand, only offers version-specific methods.  This is discussed further
in the [Extracting Data](#extracting-data) section.

### Extracting Data

Callers that work with standard scripts often need to obtain the type and
version-specific data.  For example, a caller will likely need to be able to
determine the number of required signatures and individual public keys from a
version 0 ECDSA multisignature script for further processing.

This package provides several version-specific data extraction methods, such as
`ExtractPubKeyHashV0` and `ExtractMultiSigScriptDetailsV0`, for this purpose.

It should be noted that unlike the script type determination methods, this
package does not offer version-agnostic variants of the data extraction methods
that accept a dynamic version.  This is a deliberate design choice that was made
because the associated data is highly dependent on both the specific type of
script and the scripting language version, so any version-agnostic methods would
need to return interfaces that callers would have to type assert based on the
version thereby defeating the original intention of using a version-agnostic
method to begin with.

### Provably Pruneable Scripts

A provably pruneable script is a public key script that is of a specific form
that is provably unspendable and therefore is safe to prune from the set of
unspent transaction outputs.  They are primarily useful for anchoring
commitments into the blockchain and are the preferred method to achieve that
goal.

This package provides the version-specific `ProvablyPruneableScriptV0` method
for this purpose.

Note that no version-agnostic variant of the method that accepts a dynamic
version is provided since the exact details of what is considered standard is
likely to change between scripting language versions, so callers will
necessarily have to ensure appropriate data is provided based on the version.

### Additional Convenience Methods

As mentioned in the overview, standardness only applies to public key scripts.
However, despite perhaps not fitting well with the name of the package, some
additional convenience methods are provided for detecting and extracting data
from common redeem scripts (the target script of a standard pay-to-script-hash
script) as follows:

- Version 0 ECDSA multisignature redeem scripts
- Version 0 atomic swap redeem scripts

## Installation and Updating

This package is part of the `github.com/decred/dcrd/txscript/v4` module.  Use
the standard go tooling for working with modules to incorporate it.

## Examples

* [DetermineScriptType](https://pkg.go.dev/github.com/decred/dcrd/txscript/v4/stdscript#example-DeterminScriptType)

  Demonstrates determining the type of a script for a given scripting language
  version.

* [ExtractPubKeyHashV0](https://pkg.go.dev/github.com/decred/dcrd/txscript/v4/stdscript#example-ExtractPubKeyHashV0)

  Demonstrates extracting a public key hash from a standard pay-to-pubkey-hash
  script for scripting language version 0.

* [ExtractScriptHashV0](https://pkg.go.dev/github.com/decred/dcrd/txscript/v4/stdscript#example-ExtractScriptHashV0)

  Demonstrates extracting a script hash from a standard pay-to-script-hash
  script for scripting language version 0.

## License

Package stdscript is licensed under the [copyfree](http://copyfree.org) ISC
License.
