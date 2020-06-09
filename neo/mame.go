package neo

//go:generate go run generate.go

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/bodgit/plumbing"
	"github.com/gabriel-vasile/mimetype"
)

const (
	_                = iota
	oneTwentyEightKB = 128 << (10 * iota)
	oneMB, twoMB     = 1 << (10 * iota), 2 << (10 * iota)
)

var (
	errGameNotFound = errors.New("game not found")
	errBadROM       = errors.New("ROM doesn't match MAME data")
	errROMNotFound  = errors.New("ROM not found")
)

type mameROM struct {
	filename string
	size     uint64
	crc      [crc32.Size]uint8
	sha1     [sha1.Size]uint8
}

type mameGame struct {
	parent string
	roms   [Areas][]mameROM
}

type romOpener interface {
	open(mameGame, int) ([]io.Reader, error)
}

type romReader interface {
	read(mameGame, int, romOpener) ([]byte, error)
}

func crc(c uint32) []byte {
	return []byte{byte(0xff & (c >> 24)), byte(0xff & (c >> 16)), byte(0xff & (c >> 8)), byte(c)}
}

type zipReader struct {
	path string
}

func (zr zipReader) open(g mameGame, r int) ([]io.Reader, error) {
	game, err := zip.OpenReader(zr.path)
	if err != nil {
		return nil, err
	}
	defer game.Close()
	zips := []*zip.ReadCloser{game}

	if g.parent != "" {
		parent, err := zip.OpenReader(filepath.Join(filepath.Dir(zr.path), g.parent+filepath.Ext(zr.path)))
		if err != nil {
			return nil, err
		}
		defer parent.Close()
		zips = append(zips, parent)
	}

	var readers []io.Reader
	for _, rom := range g.roms[r] {
		var r io.Reader
	zips:
		for _, z := range zips {
			for _, f := range z.File {
				if f.Name == rom.filename || bytes.Equal(crc(f.CRC32), rom.crc[:]) {
					if f.UncompressedSize64 != rom.size || !bytes.Equal(crc(f.CRC32), rom.crc[:]) {
						return nil, errBadROM
					}

					t, err := f.Open()
					if err != nil {
						return nil, err
					}
					defer t.Close()

					b := new(bytes.Buffer)

					if _, err := io.Copy(b, t); err != nil {
						return nil, err
					}

					r = b

					break zips
				}
			}
		}

		if r == nil {
			return nil, errROMNotFound
		}

		readers = append(readers, r)
	}

	return readers, nil
}

type directoryReader struct {
	path string
}

func (dr directoryReader) open(g mameGame, r int) ([]io.Reader, error) {
	dirs := []string{dr.path}

	if g.parent != "" {
		dirs = append(dirs, filepath.Join(filepath.Dir(dr.path), g.parent))
	}

	var readers []io.Reader
	for _, rom := range g.roms[r] {
		var r io.Reader
	dirs:
		for _, dir := range dirs {
			f, err := os.Open(filepath.Join(dir, rom.filename))
			if err != nil {
				if os.IsNotExist(err) {
					continue dirs
				}
				return nil, err
			}
			defer f.Close()

			b := new(bytes.Buffer)

			h := sha1.New()
			n, err := io.Copy(io.MultiWriter(h, b), f)
			if err != nil {
				return nil, err
			}

			if uint64(n) != rom.size || !bytes.Equal(h.Sum(nil), rom.sha1[:]) {
				return nil, errBadROM
			}

			r = b

			break dirs
		}

		if r == nil {
			return nil, errROMNotFound
		}

		readers = append(readers, r)
	}

	return readers, nil
}

// common handles the majority of games
type common struct{}

func (common) read(g mameGame, r int, o romOpener) ([]byte, error) {
	readers, err := o.open(g, r)
	if err != nil {
		return nil, err
	}

	var pad uint64
	for _, x := range g.roms[r] {
		if x.size > pad {
			pad = x.size
		}
	}

	switch r {
	case P:
		var patches []io.Reader
		var roms []mameROM
		i := 0
		for j, x := range g.roms[r] {
			if strings.Contains(filepath.Ext(x.filename), ".ep") {
				patches = append(patches, readers[j])
			} else {
				readers[i] = readers[j]
				roms = append(roms, x)
				i++
			}
		}
		readers = readers[:i]

		var patch []byte
		if len(patches) > 0 {
			patch, err = ioutil.ReadAll(concatenateROM(patches...))
			if err != nil {
				return nil, err
			}
		}

		if roms[0].size == twoMB {
			b, tmp := new(bytes.Buffer), new(bytes.Buffer)
			if _, err := io.CopyN(tmp, readers[0], oneMB); err != nil {
				return nil, err
			}
			if _, err := io.Copy(b, readers[0]); err != nil {
				return nil, err
			}
			if _, err := io.Copy(b, tmp); err != nil {
				return nil, err
			}
			readers[0] = b
		}
		reader := concatenateROM(readers...)

		if _, err := io.CopyN(ioutil.Discard, reader, int64(len(patch))); err != nil {
			return nil, err
		}

		return ioutil.ReadAll(concatenateROM(bytes.NewReader(patch), reader))
	case C:
		var intermediates []io.Reader

		for i := 0; i < len(readers); i += 2 {
			intermediate, err := interleaveROM(1, readers[i], readers[i+1])
			if err != nil {
				return nil, err
			}

			if i < len(readers)-2 {
				intermediate = plumbing.PaddedReader(intermediate, int64(pad*2), 0)
			}

			intermediates = append(intermediates, intermediate)
		}

		return ioutil.ReadAll(concatenateROM(intermediates...))
	default:
		var padded []io.Reader

		for i, r := range readers {
			if i < len(readers)-1 {
				r = plumbing.PaddedReader(r, int64(pad), 0)
			}
			padded = append(padded, r)
		}

		return ioutil.ReadAll(concatenateROM(padded...))
	}
}

// dragonsh has a couple of missing ROMs which are replaced with "erased" images of the expected size
type dragonsh struct{}

func (dragonsh) read(g mameGame, r int, o romOpener) ([]byte, error) {
	switch r {
	case P:
		return gpilotsp{}.read(g, r, o)
	case M:
		return bytes.Repeat([]byte{0xff}, oneTwentyEightKB), nil
	case V1:
		return bytes.Repeat([]byte{0xff}, twoMB), nil
	default:
		return common{}.read(g, r, o)
	}
}

// fightfeva is standard apart from the patch ROM isn't named following the
// same convention as other patch ROMs; it's named as .sp2 instead of .ep1
type fightfeva struct{}

func (fightfeva) read(g mameGame, r int, o romOpener) ([]byte, error) {
	readers, err := o.open(g, r)
	if err != nil {
		return nil, err
	}

	switch r {
	case P:
		if _, err := io.CopyN(ioutil.Discard, readers[0], int64(g.roms[r][1].size)); err != nil {
			return nil, err
		}

		return ioutil.ReadAll(concatenateROM(readers[1], readers[0]))
	default:
		return common{}.read(g, r, o)
	}
}

type gpilotsp struct{}

func (gpilotsp) read(g mameGame, r int, o romOpener) ([]byte, error) {
	readers, err := o.open(g, r)
	if err != nil {
		return nil, err
	}

	switch r {
	case P:
		var intermediates []io.Reader

		for i := 0; i < len(readers); i += 2 {
			intermediate, err := interleaveROM(1, readers[i+1], readers[i])
			if err != nil {
				return nil, err
			}
			intermediates = append(intermediates, intermediate)
		}

		return ioutil.ReadAll(concatenateROM(intermediates...))
	default:
		return kotm2p{}.read(g, r, o)
	}
}

// kof95a is standard apart from the regular ROMs being named like patch ROMs
type kof95a struct{}

func (kof95a) read(g mameGame, r int, o romOpener) ([]byte, error) {
	readers, err := o.open(g, r)
	if err != nil {
		return nil, err
	}

	switch r {
	case P:
		return ioutil.ReadAll(concatenateROM(readers...))
	default:
		return common{}.read(g, r, o)
	}
}

type kotm2 struct{}

func (kotm2) read(g mameGame, r int, o romOpener) ([]byte, error) {
	readers, err := o.open(g, r)
	if err != nil {
		return nil, err
	}

	switch r {
	case C:
		var intermediates []io.Reader

		for i := 0; i < len(readers); i += 2 {
			intermediate, err := interleaveROM(1, readers[i:i+2]...)
			if err != nil {
				return nil, err
			}
			intermediates = append(intermediates, intermediate)
		}

		i, err := interleaveROM(twoMB, intermediates...)
		if err != nil {
			return nil, err
		}

		return ioutil.ReadAll(i)
	default:
		return common{}.read(g, r, o)
	}
}

type kotm2p struct{}

func (kotm2p) read(g mameGame, r int, o romOpener) ([]byte, error) {
	readers, err := o.open(g, r)
	if err != nil {
		return nil, err
	}

	switch r {
	case P:
		var intermediates []io.Reader

		for i := 0; i < len(readers); i += 2 {
			intermediate, err := interleaveROM(1, readers[i:i+2]...)
			if err != nil {
				return nil, err
			}
			intermediates = append(intermediates, intermediate)
		}

		return ioutil.ReadAll(concatenateROM(intermediates...))
	case C:
		var intermediates []io.Reader

		for i := 0; i < len(readers); i += 4 {
			intermediate, err := interleaveROM(1, readers[i:i+4]...)
			if err != nil {
				return nil, err
			}
			intermediates = append(intermediates, intermediate)
		}

		return ioutil.ReadAll(concatenateROM(intermediates...))
	default:
		return common{}.read(g, r, o)
	}
}

// pbobblenb is standard apart from the ADPCM area has 2 MB of empty space prepended
type pbobblenb struct{}

func (pbobblenb) read(g mameGame, r int, o romOpener) ([]byte, error) {
	switch r {
	case V1:
		b, err := common{}.read(g, r, o)
		if err != nil {
			return nil, err
		}

		return append(bytes.Repeat([]byte{0}, twoMB), b...), nil
	default:
		return common{}.read(g, r, o)
	}
}

type viewpoin struct{}

func (viewpoin) read(g mameGame, r int, o romOpener) ([]byte, error) {
	readers, err := o.open(g, r)
	if err != nil {
		return nil, err
	}

	switch r {
	case C:
		var intermediates []io.Reader

		for i := 0; i < len(readers); i += 2 {
			intermediate, err := interleaveROM(1, readers[i:i+2]...)
			if err != nil {
				return nil, err
			}
			intermediates = append(intermediates, intermediate, bytes.NewReader(bytes.Repeat([]byte{0}, twoMB)))
		}

		i, err := interleaveROM(twoMB, intermediates...)
		if err != nil {
			return nil, err
		}

		return ioutil.ReadAll(i)
	default:
		return common{}.read(g, r, o)
	}
}

func (f *File) readMAMEROM(path string) error {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	g, ok := mameGames[base]
	if !ok {
		return errGameNotFound
	}

	f.version, f.Year, f.Genre, f.Screenshot = newVersion(), g.year, g.genre, g.screenshot

	copy(f.Name[:], g.name)
	copy(f.Manufacturer[:], g.manufacturer)

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	var o romOpener

	switch {
	case info.IsDir():
		o = directoryReader{path}
	default:
		mime, err := mimetype.DetectFile(path)
		if err != nil {
			return err
		}

		switch mime.Extension() {
		case ".zip":
			o = zipReader{path}
		default:
			return errUnsupportedFormat
		}
	}

	for i := 0; i < Areas; i++ {
		if f.ROM[i], err = g.reader.read(g.mameGame, i, o); err != nil {
			return err
		}
		f.Size[i] = uint32(len(f.ROM[i]))
	}

	return nil
}
