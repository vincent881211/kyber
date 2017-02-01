package poly

import (
	"errors"
	"fmt"

	"gopkg.in/dedis/crypto.v0/abstract"
	"gopkg.in/dedis/crypto.v0/config"
)

// This package provides  a dealer-less distributed verifiable secret sharing
// using Pedersen VSS scheme as explained in "Provably Secure Distributed
// Schnorr Signatures and a (t, n) Threshold Scheme for Implicit Certificates"
// This file is responsible for the setup of a shared secret among n peers.
// The output is a global public polynomial (pubPoly) and a secret share for
// each peers.

// PolyInfo describe the information needed to construct (and verify) a matrixShare
type Threshold struct {
	// How many peer do we need to reconstruct a secret
	T int
	// How many peers do we need to verify
	R int
	// How many peers are collaborating into constructing the shared secret ( i.e. MatrixShare is of size NxN)
	N int
}

// Represent the output of a VSS Pedersen scheme : a global public polynomial and a share of its related priv poly
// for a peer
// A SharedSecret is generated by the receiver's func : ProduceSharedSecret
// This SharedSecret is used by the distributed t-n Schnorr algorithm as a
// shared key.
type SharedSecret struct {

	// The shared public polynomial
	Pub *PubPoly

	// The share of the shared secret
	Share *abstract.Scalar

	// The index where the share has been evaluated in the shared private polynomial
	// and the index to use where the share can be checked against the shared
	// public polynomial
	// f(i) = share for peer i
	Index int
}

// Receiver Part : Receiver struct is basically the underlying structure of the general matrix.
// If a peer is a receiver, it will receive all deals and compute all of its share and then he will
// be able to generate the SharedSecret
type Receiver struct {
	// info is just the info about the polynomials we're gonna use
	info Threshold

	// suite [ed25519,nist ...]
	suite abstract.Suite

	// This index is the index used by the dealers to make the share for this receiver
	// For a given receiver, It should be the same for every deals /!!\
	index int

	// the Receiver private / public key combination
	// it may or may not have to be the long term key of the node
	key *config.KeyPair

	// List of Dealers. Be careful : this receiver should have the SAME index in
	// each of the Dealer's deals otherwise we wouldn't know which index to chose
	// from the shared public polynomial
	deals []*Deal
}

// Returns a new Receiver
func NewReceiver(suite abstract.Suite, info Threshold, key *config.KeyPair) *Receiver {
	return new(Receiver).Init(suite, info, key)
}

// Init a new Receiver struct
// info is the info about the structure of the polynomials used
// key is the long-term public key of the receiver
func (r *Receiver) Init(suite abstract.Suite, info Threshold, key *config.KeyPair) *Receiver {
	r.index = -1 // no dealer received yet
	r.info = info
	r.suite = suite
	r.key = key
	r.deals = make([]*Deal, 0, info.N)
	return r
}

// Adddeal adds a deal to the array of deals the receiver already has.
// You must give the index of the receiver so the receiver can generate its
// response for this deal to the dealer,
// i.e. index is generally the index of the receiver in the matrix, and
// is usually fixed.
// It will return a Response to be sent back to the Dealer so he can verify its
// deal
func (r *Receiver) AddDeal(index int, deal *Deal) (*Response, error) {
	if r.index == -1 {
		r.index = index
	}
	if r.index != index {
		return nil, errors.New(fmt.Sprintf("Wrong index received for receiver : %d instead of %d", index, r.index))
	}
	// produce response
	resp, err := deal.ProduceResponse(index, r.key)
	if err == nil {
		r.deals = append(r.deals, deal)
	}
	return resp, err
}

// ProduceSharedSecret will generate the sharedsecret relative to this receiver
// it will throw an error if something is wrong such as not enough Dealers received
// The shared secret can be computed when all deals have been sent and
// basically consists of a
// 1. Public Polynomial which is basically the sums of all Dealers's polynomial
// 2. Share of the global Private Polynomial (which is to never be computed directly), which is
// 		basically SUM of fj(i) for a receiver i
func (r *Receiver) ProduceSharedSecret() (*SharedSecret, error) {
	if len(r.deals) < 1 {
		return nil, errors.New("Receiver has 0 Dealers in its data.Can't produce SharedSecret.")
	}
	pub := new(PubPoly)
	pub.InitNull(r.suite, r.info.T, r.suite.Point().Base())
	share := r.suite.Scalar().Zero()
	for index := range r.deals {
		// Compute secret shares of the shared secret = sum of the respectives shares of peer i
		// For peer i , s = SUM fj(i)
		s := r.deals[index].RevealShare(r.index, r.key)
		//s, e := r.Dealers[index].State.RevealShare(r.index, r.Key)
		share.Add(share, s)

		// Compute shared public polynomial = SUM of indiviual public polynomials
		pub.Add(pub, r.deals[index].PubPoly())
	}

	if val := pub.Check(r.index, share); val == false {
		return nil, errors.New("Receiver's secret share of the shared secret could not be checked against the shared polynomial")
	}

	return &SharedSecret{
		Pub:   pub,
		Share: &share,
		Index: r.index,
	}, nil
}

// MARSHALLING side

// PolyInfo marshalling :
func (p *Threshold) Equal(p2 Threshold) bool {
	return p.N == p2.N && p.R == p2.R && p.T == p2.T
}
