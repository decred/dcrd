package dcrec

type SignatureType int

const (
	// ECTypeSecp256k1 is the ECDSA type for secp256k1 elliptical curves.
	ECTypeSecp256k1 SignatureType = 0

	// ECTypeEdwards is the ECDSA type for edwards elliptical curves.
	ECTypeEdwards = 1

	// ECTypeSecSchnorr is the ECDSA type for schnorr elliptical curves.
	ECTypeSecSchnorr = 2
)
