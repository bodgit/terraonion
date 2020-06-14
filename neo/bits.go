package neo

// XXX Is there a nicer way to write these?

func bitswapByte(n byte, bits ...int) (result byte) {
	for _, b := range bits {
		result <<= 1
		if n&(1<<b) > 0 {
			result |= 1
		}
	}
	return
}

func bitswapInt(n int, bits ...int) (result int) {
	for _, b := range bits {
		result <<= 1
		if n&(1<<b) > 0 {
			result |= 1
		}
	}
	return
}

func bitswapUint16(n uint16, bits ...int) (result uint16) {
	for _, b := range bits {
		result <<= 1
		if n&(1<<b) > 0 {
			result |= 1
		}
	}
	return
}
