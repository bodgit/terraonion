package neo

//go:generate go run generate.go

import (
	"bytes"
	"io"
	"io/ioutil"
	"regexp"

	"github.com/bodgit/plumbing"
)

const (
	_                = iota
	oneTwentyEightKB = 128 << (10 * iota)
	oneMB, twoMB     = 1 << (10 * iota), 2 << (10 * iota)
)

type mameROM struct {
	filename string
	size     uint64
	crc      []byte
}

type mameArea struct {
	size uint64
	rom  []mameROM
}

func (a mameArea) padSize() uint64 {
	var pad uint64
	for _, r := range a.rom {
		if r.size > pad {
			pad = r.size
		}
	}
	return pad
}

type mameGame struct {
	parent string
	area   [Areas]mameArea
}

type gameReader interface {
	read(*File, mameGame, [][]io.Reader) error
}

func commonPReader(a mameArea, readers []io.Reader, re *regexp.Regexp) ([]byte, error) {
	var patches []io.Reader
	var roms []mameROM

	i := 0
	for j, x := range a.rom {
		if re != nil && re.MatchString(x.filename) {
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
		var err error
		patch, err = ioutil.ReadAll(io.MultiReader(patches...))
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
	reader := io.MultiReader(readers...)

	if _, err := io.CopyN(ioutil.Discard, reader, int64(len(patch))); err != nil {
		return nil, err
	}

	return ioutil.ReadAll(io.MultiReader(bytes.NewReader(patch), reader))
}

func commonCReader(a mameArea, readers []io.Reader) ([]byte, error) {
	var intermediates []io.Reader

	for i := 0; i < len(readers); i += 2 {
		intermediate, err := interleaveROM(1, readers[i], readers[i+1])
		if err != nil {
			return nil, err
		}

		if i < len(readers)-2 {
			intermediate = plumbing.PaddedReader(intermediate, int64(a.padSize()*2), 0)
		}

		intermediates = append(intermediates, intermediate)
	}

	return ioutil.ReadAll(io.MultiReader(intermediates...))
}

func commonPaddedReader(a mameArea, readers []io.Reader) ([]byte, error) {
	padded := make([]io.Reader, len(readers))

	for i, r := range readers {
		if i < len(readers)-1 {
			r = plumbing.PaddedReader(r, int64(a.padSize()), 0)
		}
		padded[i] = r
	}

	return ioutil.ReadAll(io.MultiReader(padded...))
}

func commonCMC42Reader(f *File, g mameGame, readers [][]io.Reader, xor int) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			if f.ROM[P], err = commonPReader(g.area[P], readers[P], regexp.MustCompile(`\.ep`)); err != nil {
				return err
			}
		case S:
			break
		case C:
			b, err := commonCReader(g.area[C], readers[C])
			if err != nil {
				return err
			}
			f.ROM[C], f.ROM[S] = cmc42Decrypt(b, xor, int(g.area[S].size))
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// common handles the majority of games
type common struct{}

func (common) read(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			if f.ROM[P], err = commonPReader(g.area[P], readers[P], regexp.MustCompile(`\.ep`)); err != nil {
				return err
			}
		case C:
			if f.ROM[C], err = commonCReader(g.area[C], readers[C]); err != nil {
				return err
			}
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// bangbead uses CMC42 encryption
type bangbead struct{}

func (bangbead) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, 0xf8)
}

// dragonsh has a couple of missing ROMs which are replaced with "erased" images of the expected size
type dragonsh struct{}

func (dragonsh) read(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			if f.ROM[P], err = gpilotspPReader(g.area[P], readers[P]); err != nil {
				return err
			}
		case M:
			f.ROM[M] = bytes.Repeat([]byte{0xff}, oneTwentyEightKB)
		case V1:
			f.ROM[V1] = bytes.Repeat([]byte{0xff}, twoMB)
		case C:
			if f.ROM[C], err = commonCReader(g.area[C], readers[C]); err != nil {
				return err
			}
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// fightfeva is standard apart from the patch ROM isn't named following the
// same convention as other patch ROMs; it's named as .sp2 instead of .ep1
type fightfeva struct{}

func (fightfeva) read(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			if f.ROM[P], err = commonPReader(g.area[P], readers[P], regexp.MustCompile(`\.sp`)); err != nil {
				return err
			}
		case C:
			if f.ROM[C], err = commonCReader(g.area[C], readers[C]); err != nil {
				return err
			}
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// ganryu uses CMC42 encryption
type ganryu struct{}

func (ganryu) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, 0x07)
}

func gpilotspPReader(a mameArea, readers []io.Reader) ([]byte, error) {
	var intermediates []io.Reader

	for i := 0; i < len(readers); i += 2 {
		intermediate, err := interleaveROM(1, readers[i+1], readers[i])
		if err != nil {
			return nil, err
		}
		intermediates = append(intermediates, intermediate)
	}

	return ioutil.ReadAll(io.MultiReader(intermediates...))
}

type gpilotsp struct{}

func (gpilotsp) read(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			if f.ROM[P], err = gpilotspPReader(g.area[P], readers[P]); err != nil {
				return err
			}
		case C:
			if f.ROM[C], err = kotm2pCReader(g.area[C], readers[C]); err != nil {
				return err
			}
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// kof95a is standard apart from the regular ROMs being named like patch ROMs
type kof95a struct{}

func (kof95a) read(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			if f.ROM[P], err = commonPReader(g.area[P], readers[P], nil); err != nil {
				return err
			}
		case C:
			if f.ROM[C], err = commonCReader(g.area[C], readers[C]); err != nil {
				return err
			}
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// kof99ka uses CMC42 encryption
type kof99ka struct{}

func (kof99ka) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, 0x00)
}

func kotm2CReader(a mameArea, readers []io.Reader) ([]byte, error) {
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
}

type kotm2 struct{}

func (kotm2) read(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			if f.ROM[P], err = commonPReader(g.area[P], readers[P], regexp.MustCompile(`\.ep`)); err != nil {
				return err
			}
		case C:
			if f.ROM[C], err = kotm2CReader(g.area[C], readers[C]); err != nil {
				return err
			}
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

func kotm2pPReader(a mameArea, readers []io.Reader) ([]byte, error) {
	var intermediates []io.Reader

	for i := 0; i < len(readers); i += 2 {
		intermediate, err := interleaveROM(1, readers[i:i+2]...)
		if err != nil {
			return nil, err
		}
		intermediates = append(intermediates, intermediate)
	}

	return ioutil.ReadAll(io.MultiReader(intermediates...))
}

func kotm2pCReader(a mameArea, readers []io.Reader) ([]byte, error) {
	var intermediates []io.Reader

	for i := 0; i < len(readers); i += 4 {
		intermediate, err := interleaveROM(1, readers[i:i+4]...)
		if err != nil {
			return nil, err
		}
		intermediates = append(intermediates, intermediate)
	}

	return ioutil.ReadAll(io.MultiReader(intermediates...))
}

type kotm2p struct{}

func (kotm2p) read(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			if f.ROM[P], err = kotm2pPReader(g.area[P], readers[P]); err != nil {
				return err
			}
		case C:
			if f.ROM[C], err = kotm2pCReader(g.area[C], readers[C]); err != nil {
				return err
			}
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// mslug3h uses CMC42 encryption
type mslug3h struct{}

func (mslug3h) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, 0xad)
}

// nitd uses CMC42 encryption
type nitd struct{}

func (nitd) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, 0xff)
}

// pbobblenb is standard apart from the ADPCM area has 2 MB of empty space prepended
type pbobblenb struct{}

func (pbobblenb) read(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			if f.ROM[P], err = commonPReader(g.area[P], readers[P], regexp.MustCompile(`\.ep`)); err != nil {
				return err
			}
		case C:
			if f.ROM[C], err = commonCReader(g.area[C], readers[C]); err != nil {
				return err
			}
		case V1:
			b, err := commonPaddedReader(g.area[V1], readers[V1])
			if err != nil {
				return err
			}
			f.ROM[V1] = append(bytes.Repeat([]byte{0}, twoMB), b...)
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// preisle2 uses CMC42 encryption
type preisle2 struct{}

func (preisle2) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, 0x9f)
}

// s1945p uses CMC42 encryption
type s1945p struct{}

func (s1945p) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, 0x05)
}

// sengoku3 uses CMC42 encryption
type sengoku3 struct{}

func (sengoku3) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, 0xfe)
}

func viewpoinCReader(a mameArea, readers []io.Reader) ([]byte, error) {
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
}

type viewpoin struct{}

func (viewpoin) read(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			if f.ROM[P], err = commonPReader(g.area[P], readers[P], regexp.MustCompile(`\.ep`)); err != nil {
				return err
			}
		case C:
			if f.ROM[C], err = viewpoinCReader(g.area[C], readers[C]); err != nil {
				return err
			}
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// zupapa uses CMC42 encryption
type zupapa struct{}

func (zupapa) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, 0xbd)
}
