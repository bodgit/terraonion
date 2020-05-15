/*
Package crc32 implements the 32-bit cyclic redundancy check, or CRC-32,
checksum as implemented by the Terraonion firmware.

It uses the standard CRC-32 normal polynomial but computes the CRC in a
slightly different way.
*/
package crc32

import (
	"hash"
	crc "hash/crc32"
)

func makeTable(poly uint32) *crc.Table {
	t := new(crc.Table)
	for i := 0; i < 256; i++ {
		crc := uint32(i << 24)
		for j := 0; j < 8; j++ {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ poly
			} else {
				crc <<= 1
			}
			t[i] = crc
		}
	}
	return t
}

const polynomial = 0x04c11db7

var table = makeTable(polynomial)

type digest struct {
	crc uint32
	tab *crc.Table
}

// New creates a new hash.Hash32 computing the CRC-32 checksum. Its Sum
// method will lay the value out in big-endian byte order.
func New() hash.Hash32 {
	return &digest{0, table}
}

func (d *digest) Size() int { return crc.Size }

func (d *digest) BlockSize() int { return 1 }

func (d *digest) Reset() { d.crc = 0 }

func update(crc uint32, tab *crc.Table, p []byte) uint32 {
	for i := range p {
		crc = crc<<8 ^ tab[((crc>>24)^uint32(p[i^3]))&0xff]
	}
	return crc
}

// Update returns the result of adding the bytes in p to the crc.
func Update(crc uint32, p []byte) uint32 {
	return update(crc, table, p)
}

func (d *digest) Write(p []byte) (n int, err error) {
	d.crc = update(d.crc, d.tab, p)
	return len(p), nil
}

func (d *digest) Sum32() uint32 { return d.crc }

func (d *digest) Sum(in []byte) []byte {
	s := d.Sum32()
	return append(in, byte(s>>24), byte(s>>16), byte(s>>8), byte(s))
}

// Checksum returns the CRC-32 checksum of data.
func Checksum(data []byte) uint32 { return Update(0, data) }
