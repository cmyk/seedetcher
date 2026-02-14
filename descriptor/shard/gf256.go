package shard

import (
	"crypto/rand"
	"errors"
	"fmt"
)

var expTable [512]byte
var logTable [256]byte

func init() {
	// Build GF(256) tables with irreducible polynomial x^8 + x^4 + x^3 + x + 1 (0x11d).
	x := byte(1)
	for i := 0; i < 255; i++ {
		expTable[i] = x
		logTable[x] = byte(i)
		x = xtime(x)
	}
	for i := 255; i < len(expTable); i++ {
		expTable[i] = expTable[i-255]
	}
}

func xtime(x byte) byte {
	if x&0x80 != 0 {
		return (x << 1) ^ 0x1d
	}
	return x << 1
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return expTable[int(logTable[a])+int(logTable[b])]
}

func gfInv(a byte) byte {
	if a == 0 {
		panic("gf inverse of zero")
	}
	return expTable[255-int(logTable[a])]
}

func splitBytes(secret []byte, n, t int) ([][]byte, error) {
	if len(secret) == 0 {
		return nil, errors.New("empty secret")
	}
	if n < minThreshold || n > maxShareCount {
		return nil, fmt.Errorf("invalid n: %d", n)
	}
	if t < minThreshold || t > n {
		return nil, fmt.Errorf("invalid t: %d", t)
	}

	shares := make([][]byte, n)
	for i := range shares {
		shares[i] = make([]byte, len(secret))
	}

	coeff := make([]byte, t)
	for bIdx, s := range secret {
		coeff[0] = s
		if t > 1 {
			if _, err := rand.Read(coeff[1:t]); err != nil {
				return nil, err
			}
		}
		for i := 1; i <= n; i++ {
			x := byte(i)
			shares[i-1][bIdx] = evalPoly(coeff, x)
		}
	}
	return shares, nil
}

func combineBytes(parts [][]byte, x []byte) ([]byte, error) {
	if len(parts) == 0 {
		return nil, errors.New("no parts")
	}
	if len(parts) != len(x) {
		return nil, errors.New("parts/x length mismatch")
	}
	l := len(parts[0])
	for i := range parts {
		if len(parts[i]) != l {
			return nil, errors.New("inconsistent part length")
		}
		if x[i] == 0 {
			return nil, errors.New("invalid x=0")
		}
		for j := i + 1; j < len(x); j++ {
			if x[i] == x[j] {
				return nil, errors.New("duplicate x coordinate")
			}
		}
	}

	out := make([]byte, l)
	for b := 0; b < l; b++ {
		var acc byte
		for i := 0; i < len(parts); i++ {
			li := lagrangeAtZero(x, i)
			acc ^= gfMul(parts[i][b], li)
		}
		out[b] = acc
	}
	return out, nil
}

func evalPoly(coeff []byte, x byte) byte {
	// Horner's method.
	var y byte
	for i := len(coeff) - 1; i >= 0; i-- {
		y = gfMul(y, x) ^ coeff[i]
	}
	return y
}

func lagrangeAtZero(x []byte, i int) byte {
	// l_i(0) = Π_{j!=i} x_j / (x_j - x_i). In GF(2^8), subtraction is XOR.
	num := byte(1)
	den := byte(1)
	xi := x[i]
	for j := 0; j < len(x); j++ {
		if j == i {
			continue
		}
		xj := x[j]
		num = gfMul(num, xj)
		den = gfMul(den, xj^xi)
	}
	return gfMul(num, gfInv(den))
}
