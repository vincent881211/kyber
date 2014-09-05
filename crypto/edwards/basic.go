package edwards

import (
	"math/big"
	"crypto/cipher"
	"dissent/crypto"
)

type basicPoint struct {
	x,y crypto.ModInt
	c *BasicCurve
}

func (P *basicPoint) String() string {
	return P.c.pointString(&P.x,&P.y)
}

// Create a new ModInt representing a coordinate on this curve,
// with a given int64 integer value for constant-initialization convenience.
func (P *basicPoint) coord(v int64) *crypto.ModInt {
	return crypto.NewModInt(v, &P.c.P)
}

func (P *basicPoint) Len() int {
	return (P.y.M.BitLen() + 7 + 1) / 8
}

// Encode an Edwards curve point.
func (P *basicPoint) Encode() []byte {
	return P.c.encodePoint(&P.x, &P.y)
}

// Decode an Edwards curve point.
func (P *basicPoint) Decode(b []byte) error {
	return P.c.decodePoint(b, &P.x, &P.y)
}

// Equality test for two Points on the same curve
func (P *basicPoint) Equal(P2 crypto.Point) bool {
	E2 := P2.(*basicPoint)
	return P.x.Equal(&E2.x) && P.y.Equal(&E2.y)
}

// Set point to be equal to P2.
func (P *basicPoint) Set(P2 crypto.Point) crypto.Point {
	E2 := P2.(*basicPoint)
	P.c = E2.c
	P.x.Set(&E2.x)
	P.y.Set(&E2.y)
	return P
}

// Set to the neutral element, which is (0,1) for twisted Edwards curves.
func (P *basicPoint) Null() crypto.Point {
	P.Set(&P.c.null)
	return P
}

// Set to the standard base point for this curve
func (P *basicPoint) Base(rand cipher.Stream) crypto.Point {
	if rand == nil {
		P.Set(&P.c.base)
	} else {
		P.c.pickBase(rand, P, &P.c.null)
	}
	return P
}

func (P *basicPoint) PickLen() int {
	return P.c.pickLen()
}

func (P *basicPoint) Pick(data []byte,rand cipher.Stream) (crypto.Point, []byte) {
	return P,P.c.pickPoint(data, rand, &P.x, &P.y)
}

// Extract embedded data from a point group element
func (P *basicPoint) Data() ([]byte,error) {
	return P.c.data(&P.x,&P.y)
}

// Add two points using the basic unified addition laws for Edwards curves:
//
//	x' = ((x1*y2 + x2*y1) / (1 + d*x1*x2*y1*y2))
//	y' = ((y1*y2 - a*x1*x2) / (1 - d*x1*x2*y1*y2))
//
func (P *basicPoint) Add(P1,P2 crypto.Point) crypto.Point {
	E1 := P1.(*basicPoint)
	E2 := P2.(*basicPoint)
	x1,y1 := E1.x,E1.y
	x2,y2 := E2.x,E2.y

	var t1,t2,dm,nx,dx,ny,dy crypto.ModInt

	// Reused part of denominator: dm = d*x1*x2*y1*y2
	dm.Mul(&P.c.d,&x1).Mul(&dm,&x2).Mul(&dm,&y1).Mul(&dm,&y2)

	// x' numerator/denominator
	nx.Add(t1.Mul(&x1,&y2),t2.Mul(&x2,&y1))
	dx.Add(&P.c.one,&dm)

	// y' numerator/denominator
	ny.Sub(t1.Mul(&y1,&y2),t2.Mul(&x1,&x2).Mul(&P.c.a,&t2))
	dy.Sub(&P.c.one,&dm)

	// result point
	P.x.Div(&nx,&dx)
	P.y.Div(&ny,&dy)
	return P
}

// Point doubling, which for Edwards curves can be accomplished
// simply by adding a point to itself (no exceptions for equal input points).
func (P *basicPoint) double() crypto.Point {
	return P.Add(P,P)
}

// Subtract points so that their secrets subtract homomorphically
func (P *basicPoint) Sub(A,B crypto.Point) crypto.Point {
	var nB basicPoint
	return P.Add(A,nB.Neg(B))
}

// Find the negative of point A.
// For Edwards curves, the negative of (x,y) is (-x,y).
func (P *basicPoint) Neg(A crypto.Point) crypto.Point {
	E := A.(*basicPoint)
	P.c = E.c
	P.x.Neg(&E.x)
	P.y.Set(&E.y)
	return P
}

// Multiply point p by scalar s using the repeated doubling method.
func (P *basicPoint) Mul(G crypto.Point, s crypto.Secret) crypto.Point {
	v := s.(*crypto.ModInt).V
	if G == nil {
		return P.Base(nil).Mul(P,s)
	}
	T := P
	if G == P {		// Must use temporary in case G == P
		T = &basicPoint{}
	}
	T.Set(&P.c.null)	// Initialize to identity element (0,1)
	for i := v.BitLen()-1; i >= 0; i-- {
		T.double()
		if v.Bit(i) != 0 {
			T.Add(T, G)
		}
	}
	if T != P {
		P.Set(T)
	}
	return P
}


// Basic unoptimized reference implementation of Twisted Edwards curves.
// This reference implementation is mainly intended for testing, debugging,
// and instructional uses, and not for production use.
// The projective coordinates implementation (ProjectiveCurve)
// is just as general and much faster.
//
type BasicCurve struct {
	curve			// generic Edwards curve functionality
	null basicPoint		// Neutral/identity point (0,1)
	base basicPoint		// Standard base point
}

// Create a new Point on this curve.
func (c *BasicCurve) Point() crypto.Point {
	P := new(basicPoint)
	P.c = c
	P.Set(&c.null)
	return P
}

// Initialize the curve with given parameters.
func (c *BasicCurve) Init(p *Param) *BasicCurve {
	c.curve.init(p)

	// Identity element is (0,1)
	c.null.c = c
	c.null.x.Init64(0, &c.P)
	c.null.y.Init64(1, &c.P)

	// Base point B
	c.base.c = c
	c.base.x.Init(&p.BX, &c.P)
	c.base.y.Init(&p.BY, &c.P)

	// Sanity checks
	if !c.onCurve(&c.null.x,&c.null.y) {
		panic("init: null point not on curve!?")
	}
	if !c.onCurve(&c.base.x,&c.base.y) {
		panic("init: base point not on curve!?")
	}

	return c
}

// Compute the parameters for Curve25519 from the Ed25519 specification in:
// High-speed high-security signatures
// http://ed25519.cr.yp.to/ed25519-20110926.pdf
//
// Here we actually compute the standard base point by the specification,
// which requires that the curve already be (mostly) initialized.
func (c *BasicCurve) init25519() *BasicCurve {
	c.Name = "25519"

	// p = 2^255 - 19
	c.P.SetBit(zero, 255, 1)
	c.P.Sub(&c.P, big.NewInt(19))
	//println("p: "+c.P.String())

	// r = 2^252 + 27742317777372353535851937790883648493
	c.R.SetString("27742317777372353535851937790883648493", 10)
	c.R.SetBit(&c.R, 252, 1)
	//println("r: "+c.R.String())

	// s = 8 (cofactor)
	c.S.SetInt64(8)
	c.cofact.Init64(8, &c.P)

	// a = -1
	c.a.Init64(-1, &c.P)
	//println("a: "+c.a.V.String())

	// d = -121665/121666
	c.d.Init64(-121665, &c.P).Div(&c.d,crypto.NewModInt(121666, &c.P))
	//println("d: "+c.d.V.String())

	// Useful ModInt constants for this curve
	c.zero.Init64(0, &c.P)
	c.one.Init64(1, &c.P)

	// Identity element is (0,1)
	c.null.c = c
	c.null.x.Init64(0, &c.P)
	c.null.y.Init64(1, &c.P)
	if !c.onCurve(&c.null.x,&c.null.y) {
		panic("init25519: identity point not on curve!?")
	}

	// Base point B is the unique (x,4/5) such that x is positive
	c.base.c = c
	c.base.y.Init64(4, &c.P).Div(&c.base.y,crypto.NewModInt(5, &c.P))
	ok := c.solveForX(&c.base.x,&c.base.y)
	if !ok {
		panic("init25519: invalid base point!?")
	}
	if c.coordSign(&c.base.x) != 0 {
		c.base.x.Neg(&c.base.x)	// take the positive square root
	}
	//println("B: "+c.base.String())
	if !c.onCurve(&c.base.x,&c.base.y) {
		panic("init25519: base point not on curve!?")
	}

	return c
}

