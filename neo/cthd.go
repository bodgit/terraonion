package neo

const tileSize = 128

func cthdDecrypt(rom []byte) []byte {
	b := make([]byte, len(rom))
	copy(b, rom)

	buf := make([]byte, 16*tileSize)

	for i := 0; i < 1024; i += 8 {
		for _, x := range []struct {
			start     int
			bit3shift int
			bit2shift int
			bit1shift int
			bit0shift int
		}{
			{
				i*512 + 512*0,
				0, 3, 2, 1,
			},
			{
				i*512 + 512*1,
				1, 0, 3, 2,
			},
			{
				i*512 + 512*2,
				2, 1, 0, 3,
			},
			// skip 3 & 4
			{
				i*512 + 512*5,
				0, 1, 2, 3,
			},
			{
				i*512 + 512*6,
				0, 1, 2, 3,
			},
			{
				i*512 + 512*7,
				0, 2, 3, 1,
			},
		} {
			realrom := x.start * tileSize

			for j := 0; j < 32; j++ {
				for k := 0; k < 16; k++ {
					offset := (k&1>>0)<<x.bit0shift | (k&2>>1)<<x.bit1shift | (k&4>>2)<<x.bit2shift | (k&8>>3)<<x.bit3shift
					copy(buf[k*tileSize:k*tileSize+tileSize], rom[realrom+offset*tileSize:])
				}
				copy(b[realrom:], buf)
				realrom += 16 * tileSize
			}
		}
	}

	return b
}
