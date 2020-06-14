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
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/bodgit/rom"
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
	errInvalid      = errors.New("neo: invalid data")
	errTooMuch      = errors.New("neo: too much data")
	errUnmatchedROM = errors.New("neo: unable to match ROM")
	errGameNotFound = errors.New("neo: game not found")
	errROMNotFound  = errors.New("neo: ROM not found")
	errUnsupported  = errors.New("neo: unsupported game")
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

	path = filepath.Clean(path)

	if err := f.readMameROM(path); err != nil {
		if err != errGameNotFound {
			return nil, err
		}

		if err := f.readGenericROM(path); err != nil {
			return nil, err
		}
	}

	// Update the sizes if the read was successful
	for i := 0; i < Areas; i++ {
		f.Size[i] = uint32(len(f.ROM[i]))
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

func (f *File) readMameROM(path string) error {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	g, ok := mameGames[base]
	if !ok {
		return errGameNotFound
	}

	f.Year, f.Genre, f.Screenshot, f.Name, f.Manufacturer = g.year, g.genre, g.screenshot, g.name, g.manufacturer

	r, err := rom.NewReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	rr := []rom.Reader{r}

	// Assume there's never a grandparent ROM
	if g.parent != "" {
		r, err := rom.NewReader(filepath.Join(filepath.Dir(path), g.parent+filepath.Ext(path)))
		if err != nil {
			return err
		}
		defer r.Close()
		rr = append(rr, r)
	}

	readers := make([][]io.Reader, Areas)

	for i := 0; i < Areas; i++ {
		for _, mr := range g.area[i].rom {
			var reader io.ReadCloser
		reader:
			for _, r := range rr {
				for _, file := range r.Files() {
					crc, err := r.Checksum(file, rom.CRC32)
					if err != nil {
						return err
					}
					if bytes.Equal(crc, mr.crc) {
						reader, err = r.Open(file)
						if err != nil {
							return err
						}
						defer reader.Close()
						break reader
					}
				}
			}
			if reader == nil {
				return errROMNotFound
			}
			readers[i] = append(readers[i], reader)
		}
	}

	return g.reader.read(f, g.mameGame, readers)
}

type byROMFilename []string

func (f byROMFilename) Len() int {
	return len(f)
}

func (f byROMFilename) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}

func (f byROMFilename) Less(i, j int) bool {
	re := regexp.MustCompile(`([psmvc])(\d+)`)
	m1 := re.FindStringSubmatch(strings.ToLower(f[i]))
	m2 := re.FindStringSubmatch(strings.ToLower(f[j]))
	if m1 == nil || m2 == nil {
		// If neither file match, just sort on the whole filename
		return f[i] < f[j]
	}
	if m1[1] != m2[1] {
		return m1[1] < m2[1]
	}
	// Shouldn't error as it's guaranteed to only be a string of digits
	n1, _ := strconv.Atoi(m1[2])
	n2, _ := strconv.Atoi(m2[2])
	return n1 < n2
}

func (f *File) readGenericROM(path string) error {
	r, err := rom.NewReader(path)
	if err != nil {
		return err
	}
	defer r.Close()

	files := r.Files()
	sort.Sort(byROMFilename(files))

	g := mameGame{}

	readers := make([][]io.Reader, Areas)

	for _, file := range files {
		area := Areas
		if match := regexp.MustCompile(`([psmc])\d+`).FindStringSubmatch(strings.ToLower(file)); match != nil {
			areaFromString := map[string]int{
				"p": P,
				"s": S,
				"m": M,
				"c": C,
			}
			area = areaFromString[match[1]]
		} else if regexp.MustCompile(`[vV](?:1\d|\d[^\d])`).MatchString(file) {
			area = V1
		} else if regexp.MustCompile(`[vV]2\d`).MatchString(file) {
			area = V2
		}

		if area == Areas {
			return errUnmatchedROM
		}

		size, err := r.Size(file)
		if err != nil {
			return err
		}

		crc, err := r.Checksum(file, rom.CRC32)
		if err != nil {
			return err
		}

		g.area[area].size += size
		g.area[area].rom = append(g.area[area].rom, mameROM{
			filename: file,
			size:     size,
			crc:      crc,
		})

		reader, err := r.Open(file)
		if err != nil {
			return err
		}
		defer reader.Close()
		readers[area] = append(readers[area], reader)
	}

	return errors.New("FIXME")

	// return common{}.read(f, g, readers)
}
