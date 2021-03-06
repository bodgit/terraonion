package neo

func cxDecrypt(b []byte) []byte {
	rom := make([]byte, len(b))
	for i := 0; i < len(b)/0x40; i++ {
		copy(rom[i*0x40:(i+1)*0x40], b[(i^1)*0x40:])
	}
	return rom
}

func sxDecrypt(b []byte, value int) []byte {
	rom := make([]byte, len(b))
	switch value {
	case 1:
		for i := 0; i < len(rom); i += 0x10 {
			copy(rom[i:i+8], b[i+8:])
			copy(rom[i+8:], b[i:i+8])
		}
	case 2:
		for i := 0; i < len(rom); i++ {
			rom[i] = bitswapByte(b[i], 7, 6, 0, 4, 3, 2, 1, 5)
		}
	default:
		return b
	}
	return rom
}
