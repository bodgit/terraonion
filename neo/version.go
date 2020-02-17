package neo

import "bytes"

type version struct {
	Signature [4]uint8
}

var signature = [4]uint8{'N', 'E', 'O', 1}

func (v version) isValid() bool {
	return bytes.Equal(v.Signature[:], signature[:])
}

func newVersion() version {
	return version{signature}
}
