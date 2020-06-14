package neo

//go:generate go run generate.go

import (
	"bytes"
	"encoding/binary"
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

// CMC42 XOR keys
const (
	bangbeadGfxKey = 0xf8
	ganryuGfxKey   = 0x07
	garouGfxKey    = 0x06
	kof99GfxKey    = 0x00
	mslug3GfxKey   = 0xad
	nitdGfxKey     = 0xff
	preisle2GfxKey = 0x9f
	s1945pGfxKey   = 0x05
	sengoku3GfxKey = 0xfe
	zupapaGfxKey   = 0xbd
)

// CMC50 XOR keys
const (
	kof2000GfxKey  = 0x00
	kof2001GfxKey  = 0x1e
	kof2003GfxKey  = 0x9d
	jockeygpGfxKey = 0xac
	mslug4GfxKey   = 0x31
	mslug5GfxKey   = 0x19
	pnyaaGfxKey    = 0x2e
	rotdGfxKey     = 0x3f
	svcGfxKey      = 0x57
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

func uint16SliceToBytes(rom []uint16) []byte {
	b := make([]byte, len(rom)*2)
	for i, x := range rom {
		binary.LittleEndian.PutUint16(b[i*2:(i+1)*2], x)
	}
	return b
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

func smaPReader(a mameArea, readers []io.Reader) ([]uint16, error) {
	b, err := ioutil.ReadAll(io.MultiReader(append([]io.Reader{bytes.NewBuffer(bytes.Repeat([]byte{0x00}, 0xc0000))}, readers...)...))
	if err != nil {
		return nil, err
	}

	rom := make([]uint16, len(b)/2)
	for i := range rom {
		rom[i] = binary.LittleEndian.Uint16(b[i*2 : (i+1)*2])
	}

	return rom, nil
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
			f.ROM[C] = cmc42GfxDecrypt(b, xor)
			f.ROM[S] = cmcSfixDecrypt(f.ROM[C], int(g.area[S].size))
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

func commonCMC50Reader(f *File, g mameGame, readers [][]io.Reader, xor int) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			if f.ROM[P], err = commonPReader(g.area[P], readers[P], regexp.MustCompile(`\.ep`)); err != nil {
				return err
			}
		case S:
			break
		case M:
			b, err := commonPaddedReader(g.area[M], readers[M])
			if err != nil {
				return err
			}
			f.ROM[M] = cmc50M1Decrypt(b)
		case C:
			b, err := commonCReader(g.area[C], readers[C])
			if err != nil {
				return err
			}
			f.ROM[C] = cmc50GfxDecrypt(b, xor)
			f.ROM[S] = cmcSfixDecrypt(f.ROM[C], int(g.area[S].size))
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

func commonPCM2Reader(f *File, g mameGame, readers [][]io.Reader, xor int, decryptSfix bool, value int) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			if f.ROM[P], err = commonPReader(g.area[P], readers[P], regexp.MustCompile(`\.ep`)); err != nil {
				return err
			}
		case S:
			if decryptSfix {
				break
			}
			if f.ROM[S], err = commonPaddedReader(g.area[S], readers[S]); err != nil {
				return err
			}
		case M:
			b, err := commonPaddedReader(g.area[M], readers[M])
			if err != nil {
				return err
			}
			f.ROM[M] = cmc50M1Decrypt(b)
		case V1:
			b, err := commonPaddedReader(g.area[V1], readers[V1])
			if err != nil {
				return err
			}
			f.ROM[V1] = pcm2Decrypt(b, value)
		case C:
			b, err := commonCReader(g.area[C], readers[C])
			if err != nil {
				return err
			}
			f.ROM[C] = cmc50GfxDecrypt(b, xor)
			if decryptSfix {
				f.ROM[S] = cmcSfixDecrypt(f.ROM[C], int(g.area[S].size))
			}
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

func pvcPReader(a mameArea, readers []io.Reader) ([]byte, error) {
	reader, err := interleaveROM(2, readers[0], readers[1])
	if err != nil {
		return nil, err
	}

	if len(readers) > 2 {
		reader = io.MultiReader(append([]io.Reader{reader}, readers[2:]...)...)
	}

	return ioutil.ReadAll(reader)
}

func commonPVCReader(f *File, g mameGame, readers [][]io.Reader, xor1, xor2 [0x20]byte, bitswap1, bitswap2, bitswap3 []int, xor3, value, xor int) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			b, err := pvcPReader(g.area[P], readers[P])
			if err != nil {
				return err
			}

			// kof2003
			if len(b) > 0x800000 {
				for i := 0; i < 0x100000; i++ {
					b[0x800000+i] ^= b[0x100002|i]
				}
			}

			for i := 0; i < 0x100000; i++ {
				b[i] ^= xor1[i%0x20]
			}

			for i := 0x100000; i < 0x800000; i++ {
				b[i] ^= xor2[i%0x20]
			}

			for i := 0x100000; i < 0x800000; i += 4 {
				v := uint16(b[i+1]) | uint16(b[i+2])<<8
				v = bitswapUint16(v, bitswap1...)
				b[i+1] = byte(v & 0xff)
				b[i+2] = byte(v >> 8)
			}

			buf := make([]byte, len(b))

			copy(buf, b)
			for i := 0; i < 0x100000/0x10000; i++ {
				off := (i & 0xf0) + bitswapInt(i&0x0f, bitswap2...)
				copy(b[i*0x10000:], buf[off*0x10000:(off*0x10000)+0x10000])
			}

			for i := 0x100000; i < len(b); i += 0x100 {
				off := (i & 0xf000ff) + ((i & 0x000f00) ^ xor3) + (bitswapInt((i&0x0ff000)>>12, bitswap3...) << 12)
				copy(b[i:], buf[off:(off+0x100)])
			}

			copy(buf, b)
			copy(b[0x100000:], buf[len(b)-0x100000:])
			copy(b[0x200000:], buf[0x100000:len(b)-0x100000])

			f.ROM[P] = b
		case S:
			break
		case M:
			b, err := commonPaddedReader(g.area[M], readers[M])
			if err != nil {
				return err
			}
			f.ROM[M] = cmc50M1Decrypt(b)
		case V1:
			b, err := commonPaddedReader(g.area[V1], readers[V1])
			if err != nil {
				return err
			}
			f.ROM[V1] = pcm2Swap(b, value)
		case C:
			b, err := commonCReader(g.area[C], readers[C])
			if err != nil {
				return err
			}
			f.ROM[C] = cmc50GfxDecrypt(b, xor)
			f.ROM[S] = cmcSfixDecrypt(f.ROM[C], int(g.area[S].size))
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
	return commonCMC42Reader(f, g, readers, bangbeadGfxKey)
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
	return commonCMC42Reader(f, g, readers, ganryuGfxKey)
}

// garou uses SMA and CMC42 encryption
type garou struct{}

func (garou) read(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			rom, err := smaPReader(g.area[P], readers[P])
			if err != nil {
				return err
			}

			for i := 0; i < 0x800000/2; i++ {
				rom[i+0x080000] = bitswapUint16(rom[i+0x080000], 13, 12, 14, 10, 8, 2, 3, 1, 5, 9, 11, 4, 15, 0, 6, 7)
			}

			for i := 0; i < 0xc0000/2; i++ {
				rom[i] = rom[0x710000/2+bitswapInt(i, 23, 22, 21, 20, 19, 18, 4, 5, 16, 14, 7, 9, 6, 13, 17, 15, 3, 1, 2, 12, 11, 8, 10, 0)]
			}

			for i := 0; i < 0x800000/2; i += 0x8000 / 2 {
				buf := make([]uint16, 0x8000/2)
				copy(buf, rom[i+0x080000:])
				for j := 0; j < 0x8000/2; j++ {
					rom[i+j+0x080000] = buf[bitswapInt(j, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 9, 4, 8, 3, 13, 6, 2, 7, 0, 12, 1, 11, 10, 5)]
				}
			}

			f.ROM[P] = uint16SliceToBytes(rom)
		case S:
			break
		case C:
			b, err := commonCReader(g.area[C], readers[C])
			if err != nil {
				return err
			}
			f.ROM[C] = cmc42GfxDecrypt(b, garouGfxKey)
			f.ROM[S] = cmcSfixDecrypt(f.ROM[C], int(g.area[S].size))
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// garouh uses SMA and CMC42 encryption
type garouh struct{}

func (garouh) read(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			rom, err := smaPReader(g.area[P], readers[P])
			if err != nil {
				return err
			}

			for i := 0; i < 0x800000/2; i++ {
				rom[i+0x080000] = bitswapUint16(rom[i+0x080000], 14, 5, 1, 11, 7, 4, 10, 15, 3, 12, 8, 13, 0, 2, 9, 6)
			}

			for i := 0; i < 0xc0000/2; i++ {
				rom[i] = rom[0x7f8000/2+bitswapInt(i, 23, 22, 21, 20, 19, 18, 5, 16, 11, 2, 6, 7, 17, 3, 12, 8, 14, 4, 0, 9, 1, 10, 15, 13)]
			}

			for i := 0; i < 0x800000/2; i += 0x8000 / 2 {
				buf := make([]uint16, 0x8000/2)
				copy(buf, rom[i+0x080000:])
				for j := 0; j < 0x8000/2; j++ {
					rom[i+j+0x080000] = buf[bitswapInt(j, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 12, 8, 1, 7, 11, 3, 13, 10, 6, 9, 5, 4, 0, 2)]
				}
			}

			f.ROM[P] = uint16SliceToBytes(rom)
		case S:
			break
		case C:
			b, err := commonCReader(g.area[C], readers[C])
			if err != nil {
				return err
			}
			f.ROM[C] = cmc42GfxDecrypt(b, garouGfxKey)
			f.ROM[S] = cmcSfixDecrypt(f.ROM[C], int(g.area[S].size))
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
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

// kof2000 uses SMA and CMC50 encryption
type kof2000 struct{}

func (kof2000) read(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			rom, err := smaPReader(g.area[P], readers[P])
			if err != nil {
				return err
			}

			for i := 0; i < 0x800000/2; i++ {
				rom[i+0x080000] = bitswapUint16(rom[i+0x080000], 12, 8, 11, 3, 15, 14, 7, 0, 10, 13, 6, 5, 9, 2, 1, 4)
			}

			for i := 0; i < 0x63a000/2; i += 0x800 / 2 {
				buf := make([]uint16, 0x800/2)
				copy(buf, rom[i+0x080000:])
				for j := 0; j < 0x800/2; j++ {
					rom[i+j+0x080000] = buf[bitswapInt(j, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 4, 1, 3, 8, 6, 2, 7, 0, 9, 5)]
				}
			}

			for i := 0; i < 0xc0000/2; i++ {
				rom[i] = rom[0x73a000/2+bitswapInt(i, 23, 22, 21, 20, 19, 18, 8, 4, 15, 13, 3, 14, 16, 2, 6, 17, 7, 12, 10, 0, 5, 11, 1, 9)]
			}

			f.ROM[P] = uint16SliceToBytes(rom)
		case S:
			break
		case M:
			b, err := commonPaddedReader(g.area[M], readers[M])
			if err != nil {
				return err
			}
			f.ROM[M] = cmc50M1Decrypt(b)
		case C:
			b, err := commonCReader(g.area[C], readers[C])
			if err != nil {
				return err
			}
			f.ROM[C] = cmc50GfxDecrypt(b, kof2000GfxKey)
			f.ROM[S] = cmcSfixDecrypt(f.ROM[C], int(g.area[S].size))
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// kof2000n uses CMC50 encryption
type kof2000n struct{}

func (kof2000n) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC50Reader(f, g, readers, kof2000GfxKey)
}

// kof2001 uses CMC50 encryption
type kof2001 struct{}

func (kof2001) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC50Reader(f, g, readers, kof2001GfxKey)
}

// kof2003 uses PVC, PCM2 and CMC50 encryption
type kof2003 struct{}

func (kof2003) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonPVCReader(f, g, readers, [0x20]byte{0x3b, 0x6a, 0xf7, 0xb7, 0xe8, 0xa9, 0x20, 0x99, 0x9f, 0x39, 0x34, 0x0c, 0xc3, 0x9a, 0xa5, 0xc8, 0xb8, 0x18, 0xce, 0x56, 0x94, 0x44, 0xe3, 0x7a, 0xf7, 0xdd, 0x42, 0xf0, 0x18, 0x60, 0x92, 0x9f}, [0x20]byte{0x2f, 0x02, 0x60, 0xbb, 0x77, 0x01, 0x30, 0x08, 0xd8, 0x01, 0xa0, 0xdf, 0x37, 0x0a, 0xf0, 0x65, 0x28, 0x03, 0xd0, 0x23, 0xd3, 0x03, 0x70, 0x42, 0xbb, 0x06, 0xf0, 0x28, 0xba, 0x0f, 0xf0, 0x7a}, []int{15, 14, 13, 12, 5, 4, 7, 6, 9, 8, 11, 10, 3, 2, 1, 0}, []int{7, 6, 5, 4, 0, 1, 2, 3}, []int{4, 5, 6, 7, 1, 0, 3, 2}, 0x00800, 5, kof2003GfxKey)
}

// kof2003h uses PVC, PCM2 and CMC50 encryption
type kof2003h struct{}

func (kof2003h) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonPVCReader(f, g, readers, [0x20]byte{0xc2, 0x4b, 0x74, 0xfd, 0x0b, 0x34, 0xeb, 0xd7, 0x10, 0x6d, 0xf9, 0xce, 0x5d, 0xd5, 0x61, 0x29, 0xf5, 0xbe, 0x0d, 0x82, 0x72, 0x45, 0x0f, 0x24, 0xb3, 0x34, 0x1b, 0x99, 0xea, 0x09, 0xf3, 0x03}, [0x20]byte{0x2b, 0x09, 0xd0, 0x7f, 0x51, 0x0b, 0x10, 0x4c, 0x5b, 0x07, 0x70, 0x9d, 0x3e, 0x0b, 0xb0, 0xb6, 0x54, 0x09, 0xe0, 0xcc, 0x3d, 0x0d, 0x80, 0x99, 0x87, 0x03, 0x90, 0x82, 0xfe, 0x04, 0x20, 0x18}, []int{15, 14, 13, 12, 10, 11, 8, 9, 6, 7, 4, 5, 3, 2, 1, 0}, []int{7, 6, 5, 4, 1, 0, 3, 2}, []int{6, 7, 4, 5, 0, 1, 2, 3}, 0x00400, 5, kof2003GfxKey)
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

// kof99 uses SMA and CMC42 encryption
type kof99 struct{}

func (kof99) read(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			rom, err := smaPReader(g.area[P], readers[P])
			if err != nil {
				return err
			}

			for i := 0; i < 0x800000/2; i++ {
				rom[i+0x080000] = bitswapUint16(rom[i+0x080000], 13, 7, 3, 0, 9, 4, 5, 6, 1, 12, 8, 14, 10, 11, 2, 15)
			}

			for i := 0; i < 0x600000/2; i += 0x800 / 2 {
				buf := make([]uint16, 0x800/2)
				copy(buf, rom[i+0x080000:])
				for j := 0; j < 0x800/2; j++ {
					rom[i+j+0x080000] = buf[bitswapInt(j, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 6, 2, 4, 9, 8, 3, 1, 7, 0, 5)]
				}
			}

			for i := 0; i < 0xc0000/2; i++ {
				rom[i] = rom[0x700000/2+bitswapInt(i, 23, 22, 21, 20, 19, 18, 11, 6, 14, 17, 16, 5, 8, 10, 12, 0, 4, 3, 2, 7, 9, 15, 13, 1)]
			}

			f.ROM[P] = uint16SliceToBytes(rom)
		case S:
			break
		case C:
			b, err := commonCReader(g.area[C], readers[C])
			if err != nil {
				return err
			}
			f.ROM[C] = cmc42GfxDecrypt(b, kof99GfxKey)
			f.ROM[S] = cmcSfixDecrypt(f.ROM[C], int(g.area[S].size))
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
	return commonCMC42Reader(f, g, readers, kof99GfxKey)
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

// jockeygp uses CMC50 encryption
type jockeygp struct{}

func (jockeygp) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC50Reader(f, g, readers, jockeygpGfxKey)
}

// mslug3 uses SMA and CMC42 encryption
type mslug3 struct{}

func (mslug3) read(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			rom, err := smaPReader(g.area[P], readers[P])
			if err != nil {
				return err
			}

			for i := 0; i < 0x800000/2; i++ {
				rom[i+0x080000] = bitswapUint16(rom[i+0x080000], 4, 11, 14, 3, 1, 13, 0, 7, 2, 8, 12, 15, 10, 9, 5, 6)
			}

			for i := 0; i < 0xc0000/2; i++ {
				rom[i] = rom[0x5d0000/2+bitswapInt(i, 23, 22, 21, 20, 19, 18, 15, 2, 1, 13, 3, 0, 9, 6, 16, 4, 11, 5, 7, 12, 17, 14, 10, 8)]
			}

			for i := 0; i < 0x800000/2; i += 0x10000 / 2 {
				buf := make([]uint16, 0x10000/2)
				copy(buf, rom[i+0x080000:])
				for j := 0; j < 0x10000/2; j++ {
					rom[i+j+0x080000] = buf[bitswapInt(j, 23, 22, 21, 20, 19, 18, 17, 16, 15, 2, 11, 0, 14, 6, 4, 13, 8, 9, 3, 10, 7, 5, 12, 1)]
				}
			}

			f.ROM[P] = uint16SliceToBytes(rom)
		case S:
			break
		case C:
			b, err := commonCReader(g.area[C], readers[C])
			if err != nil {
				return err
			}
			f.ROM[C] = cmc42GfxDecrypt(b, mslug3GfxKey)
			f.ROM[S] = cmcSfixDecrypt(f.ROM[C], int(g.area[S].size))
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// mslug3a uses SMA and CMC42 encryption
type mslug3a struct{}

func (mslug3a) read(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			rom, err := smaPReader(g.area[P], readers[P])
			if err != nil {
				return err
			}

			for i := 0; i < 0x800000/2; i++ {
				rom[i+0x080000] = bitswapUint16(rom[i+0x080000], 2, 11, 12, 14, 9, 3, 1, 4, 13, 7, 6, 8, 10, 15, 0, 5)
			}

			for i := 0; i < 0xc0000/2; i++ {
				rom[i] = rom[0x5d0000/2+bitswapInt(i, 23, 22, 21, 20, 19, 18, 1, 16, 14, 7, 17, 5, 8, 4, 15, 6, 3, 2, 0, 13, 10, 12, 9, 11)]
			}

			for i := 0; i < 0x800000/2; i += 0x10000 / 2 {
				buf := make([]uint16, 0x10000/2)
				copy(buf, rom[i+0x080000:])
				for j := 0; j < 0x10000/2; j++ {
					rom[i+j+0x080000] = buf[bitswapInt(j, 23, 22, 21, 20, 19, 18, 17, 16, 15, 12, 0, 11, 3, 4, 13, 6, 8, 14, 7, 5, 2, 10, 9, 1)]
				}
			}

			f.ROM[P] = uint16SliceToBytes(rom)
		case S:
			break
		case C:
			b, err := commonCReader(g.area[C], readers[C])
			if err != nil {
				return err
			}
			f.ROM[C] = cmc42GfxDecrypt(b, mslug3GfxKey)
			f.ROM[S] = cmcSfixDecrypt(f.ROM[C], int(g.area[S].size))
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
	return commonCMC42Reader(f, g, readers, mslug3GfxKey)
}

// ms4plus uses PCM2 and CMC50 encryption but doesn't decrypt the S area
type ms4plus struct{}

func (ms4plus) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonPCM2Reader(f, g, readers, mslug4GfxKey, false, 8)
}

// mslug4 uses PCM2 and CMC50 encryption
type mslug4 struct{}

func (mslug4) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonPCM2Reader(f, g, readers, mslug4GfxKey, true, 8)
}

// mslug5 uses PVC, PCM2 and CMC50 encryption
type mslug5 struct{}

func (mslug5) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonPVCReader(f, g, readers, [0x20]byte{0xc2, 0x4b, 0x74, 0xfd, 0x0b, 0x34, 0xeb, 0xd7, 0x10, 0x6d, 0xf9, 0xce, 0x5d, 0xd5, 0x61, 0x29, 0xf5, 0xbe, 0x0d, 0x82, 0x72, 0x45, 0x0f, 0x24, 0xb3, 0x34, 0x1b, 0x99, 0xea, 0x09, 0xf3, 0x03}, [0x20]byte{0x36, 0x09, 0xb0, 0x64, 0x95, 0x0f, 0x90, 0x42, 0x6e, 0x0f, 0x30, 0xf6, 0xe5, 0x08, 0x30, 0x64, 0x08, 0x04, 0x00, 0x2f, 0x72, 0x09, 0xa0, 0x13, 0xc9, 0x0b, 0xa0, 0x3e, 0xc2, 0x00, 0x40, 0x2b}, []int{15, 14, 13, 12, 10, 11, 8, 9, 6, 7, 4, 5, 3, 2, 1, 0}, []int{7, 6, 5, 4, 1, 0, 3, 2}, []int{5, 4, 7, 6, 1, 0, 3, 2}, 0x00700, 2, mslug5GfxKey)
}

// nitd uses CMC42 encryption
type nitd struct{}

func (nitd) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, nitdGfxKey)
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

// pnyaa uses PCM2 and CMC50 encryption
type pnyaa struct{}

func (pnyaa) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonPCM2Reader(f, g, readers, pnyaaGfxKey, true, 4)
}

// preisle2 uses CMC42 encryption
type preisle2 struct{}

func (preisle2) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, preisle2GfxKey)
}

// rotd uses PCM2 and CMC50 encryption
type rotd struct{}

func (rotd) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonPCM2Reader(f, g, readers, rotdGfxKey, true, 16)
}

// s1945p uses CMC42 encryption
type s1945p struct{}

func (s1945p) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, s1945pGfxKey)
}

// sengoku3 uses CMC42 encryption
type sengoku3 struct{}

func (sengoku3) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, sengoku3GfxKey)
}

// svc uses PVC, PCM2 and CMC50 encryption
type svc struct{}

func (svc) read(f *File, g mameGame, readers [][]io.Reader) error {
	return commonPVCReader(f, g, readers, [0x20]byte{0x3b, 0x6a, 0xf7, 0xb7, 0xe8, 0xa9, 0x20, 0x99, 0x9f, 0x39, 0x34, 0x0c, 0xc3, 0x9a, 0xa5, 0xc8, 0xb8, 0x18, 0xce, 0x56, 0x94, 0x44, 0xe3, 0x7a, 0xf7, 0xdd, 0x42, 0xf0, 0x18, 0x60, 0x92, 0x9f}, [0x20]byte{0x69, 0x0b, 0x60, 0xd6, 0x4f, 0x01, 0x40, 0x1a, 0x9f, 0x0b, 0xf0, 0x75, 0x58, 0x0e, 0x60, 0xb4, 0x14, 0x04, 0x20, 0xe4, 0xb9, 0x0d, 0x10, 0x89, 0xeb, 0x07, 0x30, 0x90, 0x50, 0x0e, 0x20, 0x26}, []int{15, 14, 13, 12, 10, 11, 8, 9, 6, 7, 4, 5, 3, 2, 1, 0}, []int{7, 6, 5, 4, 2, 3, 0, 1}, []int{4, 5, 6, 7, 1, 0, 3, 2}, 0x00a00, 3, svcGfxKey)
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
	return commonCMC42Reader(f, g, readers, zupapaGfxKey)
}
