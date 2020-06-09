/*
Package neo implements the .neo file format used by the Terraonion NeoSD cartridge.
*/
package neo

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"io/ioutil"
	"strings"
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
	errUnsupportedFormat = errors.New("unsupported format")
	errInvalid           = errors.New("neo: invalid data")
	errTooMuch           = errors.New("neo: too much data")
)

var signature = [3]byte{'N', 'E', 'O'}

type fileHeader struct {
	Signature [3]byte
	Version   byte
}

func (h fileHeader) isValid() bool {
	return bytes.Equal(h.Signature[:], signature[:]) && h.Version == 1
}

func newFileHeader(version byte) fileHeader {
	return fileHeader{
		Signature: signature,
		Version:   version,
	}
}

const (
	nameLength         int = 33
	manufacturerLength int = 17
	offsetNGH          int = 0x108
)

type fileFields struct {
	fileHeader
	Size         [Areas]uint32
	Year         uint32
	Genre        uint32
	Screenshot   uint32
	NGH          uint32
	Name         [nameLength]byte
	Manufacturer [manufacturerLength]byte
	_            [4002]byte
}

// File represents a .neo file. It is simply a header followed by six ROM sections
type File struct {
	fileFields
	Size         [Areas]uint32
	Year         uint32
	Genre        Genre
	Screenshot   uint32
	NGH          uint32
	Name         string
	Manufacturer string
	ROM          [Areas][]byte
}

// NewFile returns a File based on the passed zip file or directory
// containing Neo Geo ROM images. If the last element of the path stripped
// of any .extension matches a game known to MAME then it will use MAME
// logic to decode the ROM images otherwise it falls back to generic logic
// based solely on the filenames
func NewFile(path string) (*File, error) {
	f := new(File)

	if err := f.readMAMEROM(path); err != nil {
		return nil, err
	}

	if len(f.ROM[P]) > offsetNGH+2 {
		f.NGH = uint32(binary.LittleEndian.Uint16(f.ROM[P][offsetNGH:]))
	}

	return f, nil
}

// MarshalBinary encodes the file into binary form and returns the result
func (f *File) MarshalBinary() ([]byte, error) {
	f.fileFields = fileFields{}
	f.fileFields.fileHeader = newFileHeader(1)

	copy(f.fileFields.Size[:], f.Size[:])

	f.fileFields.Year = f.Year
	f.fileFields.Genre = uint32(f.Genre)
	f.fileFields.Screenshot = f.Screenshot
	f.fileFields.NGH = f.NGH

	copy(f.fileFields.Name[:], f.Name)
	copy(f.fileFields.Manufacturer[:], f.Manufacturer)

	w := new(bytes.Buffer)
	// Writes to bytes.Buffer never error
	_ = binary.Write(w, binary.LittleEndian, &f.fileFields)

	for i := 0; i < Areas; i++ {
		_, _ = w.Write(f.ROM[i])
	}

	return w.Bytes(), nil
}

// UnmarshalBinary decodes the file from binary form
func (f *File) UnmarshalBinary(b []byte) error {
	r := bytes.NewReader(b)
	if err := binary.Read(r, binary.LittleEndian, &f.fileFields); err != nil {
		return err
	}

	if !f.isValid() {
		return errInvalid
	}

	copy(f.Size[:], f.fileFields.Size[:])

	f.Year = f.fileFields.Year
	f.Genre = Genre(f.fileFields.Genre)
	f.Screenshot = f.fileFields.Screenshot
	f.NGH = f.fileFields.NGH

	f.Name = strings.TrimRight(string(f.fileFields.Name[:]), "\x00")
	f.Manufacturer = strings.TrimRight(string(f.fileFields.Manufacturer[:]), "\x00")

	for i := 0; i < Areas; i++ {
		if f.Size[i] > 0 {
			f.ROM[i] = make([]byte, f.Size[i])
			if _, err := r.Read(f.ROM[i]); err != nil {
				return err
			}
		}
	}

	// There should be no more data to read
	if n, _ := io.CopyN(ioutil.Discard, r, 1); n != 0 {
		return errTooMuch
	}

	return nil
}
