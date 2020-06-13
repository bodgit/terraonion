package neo

import "encoding/binary"

func pcm2Decrypt(b []byte, value int) []byte {
	rom := make([]uint16, len(b)/2)
	for i := range rom {
		rom[i] = binary.LittleEndian.Uint16(b[i*2 : (i+1)*2])
	}

	buf := make([]uint16, value/2)

	for i := 0; i < len(rom); i += value / 2 {
		copy(buf, rom[i:])
		for j := 0; j < value/2; j++ {
			rom[i+j] = buf[j^(value/4)]
		}
	}

	return uint16SliceToBytes(rom)
}
