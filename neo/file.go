/*
Package neo implements the .neo file format used by the Terraonion NeoSD cartridge.
*/
package neo

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const (
	// Extension is the conventional file extension used
	Extension = ".neo"
)

// These constants map to the ROM sections
const (
	P int = iota
	S
	M
	V1
	V2
	C
	Areas
)

var (
	errInvalidFile       = errors.New("invalid file")
	errUnsupportedFormat = errors.New("unsupported format")
)

// File represents a .neo file. It is simply a header followed by six ROM sections
type File struct {
	Header
	ROM [Areas][]byte
}

// New returns a File based on the passed zip file or directory containing Neo Geo ROM images
func New(path string) (*File, error) {
	f := new(File)

	if err := f.readMAMEROM(path); err != nil {
		return nil, err
	}

	return f, nil
}

func (f *File) isValid() bool {
	return f.Header.isValid()
}

func (f *File) String() string {
	return f.Header.String()
}

// MarshalBinary encodes the file into binary form and returns the result
func (f *File) MarshalBinary() ([]byte, error) {
	w := new(bytes.Buffer)

	if err := binary.Write(w, binary.LittleEndian, &f.Header); err != nil {
		return nil, err
	}

	for i := 0; i < Areas; i++ {
		if _, err := w.Write(f.ROM[i]); err != nil {
			return nil, err
		}
	}

	return w.Bytes(), nil
}

// UnmarshalBinary decodes the file from binary form
func (f *File) UnmarshalBinary(b []byte) error {
	r := bytes.NewReader(b)

	if err := binary.Read(r, binary.LittleEndian, &f.Header); err != nil {
		return err
	}

	if !f.isValid() {
		return errInvalidFile
	}

	for i := 0; i < Areas; i++ {
		if f.Size[i] > 0 {
			f.ROM[i] = make([]byte, f.Size[i])
			if _, err := r.Read(f.ROM[i]); err != nil {
				return err
			}
		}
	}

	return nil
}
