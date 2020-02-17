package neo

import (
	"bytes"
	"fmt"
)

const (
	// NameLength is the maximum length of the game name, it should always be zero-padded to this size
	NameLength int = 33
	// ManufacturerLength is the maximum length of the game manufacturer, it should always be zero-padded to this size
	ManufacturerLength int = 17
)

// Header represents the first 4096 bytes of a .neo file
type Header struct {
	version
	Size         [Areas]uint32
	Year         uint32
	Genre        Genre
	Screenshot   uint32
	NGH          uint32
	Name         [NameLength]uint8
	Manufacturer [ManufacturerLength]uint8
	_            [4002]uint8
}

func (h Header) isValid() bool {
	return h.version.isValid()
}

func (h Header) String() string {
	return fmt.Sprintf("%s, %s, %d, %s", bytes.Trim(h.Name[:], "\000"), bytes.Trim(h.Manufacturer[:], "\000"), h.Year, h.Genre)
}
