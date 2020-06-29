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
	kof2000GfxKey   = 0x00
	kof2001GfxKey   = 0x1e
	kof2002GfxKey   = 0xec
	kof2003GfxKey   = 0x9d
	jockeygpGfxKey  = 0xac
	matrimGfxKey    = 0x6a
	mslug4GfxKey    = 0x31
	mslug5GfxKey    = 0x19
	pnyaaGfxKey     = 0x2e
	rotdGfxKey      = 0x3f
	samsho5GfxKey   = 0x0f
	samsho5spGfxKey = 0x0d
	svcGfxKey       = 0x57
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

type gameReader func(*File, mameGame, [][]io.Reader) error

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

func k2k2PReader(a mameArea, readers []io.Reader, blocks []int) ([]byte, error) {
	b, err := commonPReader(a, readers, regexp.MustCompile(`\.ep`))
	if err != nil {
		return nil, err
	}

	offset := 0x100000
	// samsho5
	if len(blocks) > 8 {
		offset = 0
	}

	dst := make([]byte, 0x80000*len(blocks))
	copy(dst, b[offset:])

	for i, x := range blocks {
		copy(b[offset+i*0x80000:], dst[x:x+0x80000])
	}

	return b, nil
}

func commonK2K2Reader(f *File, g mameGame, readers [][]io.Reader, xor int, decryptSfix bool, value int, blocks []int) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			b, err := k2k2PReader(g.area[P], readers[P], blocks)
			if err != nil {
				return err
			}
			f.ROM[P] = b
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
			f.ROM[V1] = pcm2Swap(b, value)
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

// unsupported explicitly errors
func unsupported(f *File, g mameGame, readers [][]io.Reader) error {
	return errUnsupported
}

// common handles the majority of games
func common(f *File, g mameGame, readers [][]io.Reader) error {
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
func bangbead(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, bangbeadGfxKey)
}

// dragonsh has a couple of missing ROMs which are replaced with "erased" images of the expected size
func dragonsh(f *File, g mameGame, readers [][]io.Reader) error {
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
func fightfeva(f *File, g mameGame, readers [][]io.Reader) error {
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
func ganryu(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, ganryuGfxKey)
}

// garou uses SMA and CMC42 encryption
func garou(f *File, g mameGame, readers [][]io.Reader) error {
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

// garoubl uses its own S and C encryption
func garoubl(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			if f.ROM[P], err = commonPReader(g.area[P], readers[P], regexp.MustCompile(`\.ep`)); err != nil {
				return err
			}
		case S:
			b, err := commonPaddedReader(g.area[S], readers[S])
			if err != nil {
				return err
			}
			f.ROM[S] = sxDecrypt(b, 2)
		case C:
			b, err := commonCReader(g.area[C], readers[C])
			if err != nil {
				return err
			}
			f.ROM[C] = cxDecrypt(b)
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// garouh uses SMA and CMC42 encryption
func garouh(f *File, g mameGame, readers [][]io.Reader) error {
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

func gpilotsp(f *File, g mameGame, readers [][]io.Reader) error {
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

// kf2k2pls uses PCM2, CMC50 encryption and its own P encryption
func kf2k2pls(f *File, g mameGame, readers [][]io.Reader) error {
	return commonK2K2Reader(f, g, readers, kof2002GfxKey, false, 0, []int{0x100000, 0x280000, 0x300000, 0x180000, 0x000000, 0x380000, 0x200000, 0x080000})
}

// kof2000 uses SMA and CMC50 encryption
func kof2000(f *File, g mameGame, readers [][]io.Reader) error {
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
func kof2000n(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC50Reader(f, g, readers, kof2000GfxKey)
}

// kof2001 uses CMC50 encryption
func kof2001(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC50Reader(f, g, readers, kof2001GfxKey)
}

// kof2002 uses PCM2, CMC50 encryption and its own P encryption
func kof2002(f *File, g mameGame, readers [][]io.Reader) error {
	return commonK2K2Reader(f, g, readers, kof2002GfxKey, true, 0, []int{0x100000, 0x280000, 0x300000, 0x180000, 0x000000, 0x380000, 0x200000, 0x080000})
}

// kof2003 uses PVC, PCM2 and CMC50 encryption
func kof2003(f *File, g mameGame, readers [][]io.Reader) error {
	return commonPVCReader(f, g, readers, [0x20]byte{0x3b, 0x6a, 0xf7, 0xb7, 0xe8, 0xa9, 0x20, 0x99, 0x9f, 0x39, 0x34, 0x0c, 0xc3, 0x9a, 0xa5, 0xc8, 0xb8, 0x18, 0xce, 0x56, 0x94, 0x44, 0xe3, 0x7a, 0xf7, 0xdd, 0x42, 0xf0, 0x18, 0x60, 0x92, 0x9f}, [0x20]byte{0x2f, 0x02, 0x60, 0xbb, 0x77, 0x01, 0x30, 0x08, 0xd8, 0x01, 0xa0, 0xdf, 0x37, 0x0a, 0xf0, 0x65, 0x28, 0x03, 0xd0, 0x23, 0xd3, 0x03, 0x70, 0x42, 0xbb, 0x06, 0xf0, 0x28, 0xba, 0x0f, 0xf0, 0x7a}, []int{15, 14, 13, 12, 5, 4, 7, 6, 9, 8, 11, 10, 3, 2, 1, 0}, []int{7, 6, 5, 4, 0, 1, 2, 3}, []int{4, 5, 6, 7, 1, 0, 3, 2}, 0x00800, 5, kof2003GfxKey)
}

// kof2003h uses PVC, PCM2 and CMC50 encryption
func kof2003h(f *File, g mameGame, readers [][]io.Reader) error {
	return commonPVCReader(f, g, readers, [0x20]byte{0xc2, 0x4b, 0x74, 0xfd, 0x0b, 0x34, 0xeb, 0xd7, 0x10, 0x6d, 0xf9, 0xce, 0x5d, 0xd5, 0x61, 0x29, 0xf5, 0xbe, 0x0d, 0x82, 0x72, 0x45, 0x0f, 0x24, 0xb3, 0x34, 0x1b, 0x99, 0xea, 0x09, 0xf3, 0x03}, [0x20]byte{0x2b, 0x09, 0xd0, 0x7f, 0x51, 0x0b, 0x10, 0x4c, 0x5b, 0x07, 0x70, 0x9d, 0x3e, 0x0b, 0xb0, 0xb6, 0x54, 0x09, 0xe0, 0xcc, 0x3d, 0x0d, 0x80, 0x99, 0x87, 0x03, 0x90, 0x82, 0xfe, 0x04, 0x20, 0x18}, []int{15, 14, 13, 12, 10, 11, 8, 9, 6, 7, 4, 5, 3, 2, 1, 0}, []int{7, 6, 5, 4, 1, 0, 3, 2}, []int{6, 7, 4, 5, 0, 1, 2, 3}, 0x00400, 5, kof2003GfxKey)
}

// kof95a is standard apart from the regular ROMs being named like patch ROMs
func kof95a(f *File, g mameGame, readers [][]io.Reader) error {
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

// kof97oro uses its own P, S and C encryption
func kof97oro(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			b, err := commonPReader(g.area[P], readers[P], regexp.MustCompile(`\.ep`))
			if err != nil {
				return err
			}
			f.ROM[P] = make([]byte, len(b))
			copy(f.ROM[P][:0x100000], b[:0x100000])
			copy(f.ROM[P][0x200000:0x300000], b[0x100000:0x200000])
			copy(f.ROM[P][0x100000:0x200000], b[0x200000:0x300000])
			copy(f.ROM[P][0x400000:0x500000], b[0x300000:0x400000])
			copy(f.ROM[P][0x300000:0x400000], b[0x400000:0x500000])
			copy(b, f.ROM[P])
			for i := 0; i < len(b)/2; i++ {
				copy(f.ROM[P][i*2:(i+1)*2], b[(i^0x7ffef)*2:])
			}
		case S:
			b, err := commonPaddedReader(g.area[S], readers[S])
			if err != nil {
				return err
			}
			f.ROM[S] = sxDecrypt(b, 1)
		case C:
			intermediates := []io.Reader{}
			for i := 0; i < len(readers); i += 2 {
				intermediate, err := interleaveROM(1, readers[C][i], readers[C][i+1])
				if err != nil {
					return err
				}
				intermediates = append(intermediates, intermediate)
			}

			b, err := ioutil.ReadAll(io.MultiReader(intermediates...))
			if err != nil {
				return err
			}

			f.ROM[C] = cxDecrypt(b)
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// kof98 has its own P encryption
func kof98(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			b, err := ioutil.ReadAll(io.MultiReader(readers[P]...))
			if err != nil {
				return err
			}

			sec := []int{0x000000, 0x100000, 0x000004, 0x100004, 0x10000a, 0x00000a, 0x10000e, 0x00000e}
			pos := []int{0x000, 0x004, 0x00a, 0x00e}

			dst := make([]byte, 0x200000)
			copy(dst, b)
			for i := 0x800; i < 0x100000; i += 0x200 {
				for j := 0; j < 0x100; j += 0x10 {
					for k := 0; k < 16; k += 2 {
						copy(b[i+j+k:i+j+k+2], dst[i+j+sec[k/2]+0x100:])
						copy(b[i+j+k+0x100:i+j+k+0x102], dst[i+j+sec[k/2]:])
					}
					if i >= 0x080000 && i < 0x0c0000 {
						for k := 0; k < 4; k++ {
							copy(b[i+j+pos[k]:i+j+pos[k]+2], dst[i+j+pos[k]:])
							copy(b[i+j+pos[k]+0x100:i+j+pos[k]+0x102], dst[i+j+pos[k]+0x100:])
						}
					} else if i >= 0x0c0000 {
						for k := 0; k < 4; k++ {
							copy(b[i+j+pos[k]:i+j+pos[k]+2], dst[i+j+pos[k]+0x100:])
							copy(b[i+j+pos[k]+0x100:i+j+pos[k]+0x102], dst[i+j+pos[k]:])
						}
					}
				}
				copy(b[i+0x000000:], dst[i+0x000000:i+0x000000+2])
				copy(b[i+0x000002:], dst[i+0x100000:i+0x100000+2])
				copy(b[i+0x000100:], dst[i+0x000100:i+0x000100+2])
				copy(b[i+0x000102:], dst[i+0x100100:i+0x100100+2])
			}
			copy(b[0x100000:0x500000], b[0x200000:])

			f.ROM[P] = b
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
func kof99(f *File, g mameGame, readers [][]io.Reader) error {
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
func kof99ka(f *File, g mameGame, readers [][]io.Reader) error {
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

func kotm2(f *File, g mameGame, readers [][]io.Reader) error {
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

func kotm2p(f *File, g mameGame, readers [][]io.Reader) error {
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
func jockeygp(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC50Reader(f, g, readers, jockeygpGfxKey)
}

// lans2004 uses its own P, S, V1, and C encryption
func lans2004(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			b, err := ioutil.ReadAll(io.MultiReader(readers[P]...))
			if err != nil {
				return err
			}

			sec := []int{0x3, 0x8, 0x7, 0xc, 0x1, 0xa, 0x6, 0xd}
			dst := make([]byte, 0x600000)
			for i := 0; i < 8; i++ {
				copy(dst[i*0x20000:(i+1)*0x20000], b[sec[i]*0x20000:])
			}
			copy(dst[0x0bbb00:0x0bd210], b[0x045b00:])
			copy(dst[0x02fff0:0x030000], b[0x1a92be:])
			copy(dst[0x100000:0x500000], b[0x200000:])

			rom := make([]uint16, len(dst)/2)
			for i := range rom {
				rom[i] = binary.LittleEndian.Uint16(dst[i*2 : (i+1)*2])
			}

			for i := 0xbbb00 / 2; i < 0xbe000/2; i++ {
				if (rom[i]&0xffbf == 0x4eb9 || rom[i]&0xffbf == 0x43b9) && rom[i+1] == 0x0000 {
					rom[i+1] = 0x000b
					rom[i+2] += 0x6000
				}
			}

			rom[0x2d15c/2] = 0x000b
			rom[0x2d15e/2] = 0xbb00
			rom[0x2d1e4/2] = 0x6002
			rom[0x2ea7e/2] = 0x6002
			rom[0xbbcd0/2] = 0x6002
			rom[0xbbdf2/2] = 0x6002
			rom[0xbbe42/2] = 0x6002

			f.ROM[P] = uint16SliceToBytes(rom)
		case S:
			b, err := commonPaddedReader(g.area[S], readers[S])
			if err != nil {
				return err
			}
			f.ROM[S] = sxDecrypt(b, 1)
		case V1:
			if f.ROM[V1], err = commonPaddedReader(g.area[V1], readers[V1]); err != nil {
				return err
			}
			for i := 0; i < 0xa00000; i++ {
				f.ROM[V1][i] = bitswapByte(f.ROM[V1][i], 0, 1, 5, 4, 3, 2, 6, 7)
			}
		case C:
			b, err := commonCReader(g.area[C], readers[C])
			if err != nil {
				return err
			}
			f.ROM[C] = cxDecrypt(b)
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// matrim uses PCM2, CMC50 encryption and its own P encryption
func matrim(f *File, g mameGame, readers [][]io.Reader) error {
	return commonK2K2Reader(f, g, readers, matrimGfxKey, true, 1, []int{0x100000, 0x280000, 0x300000, 0x180000, 0x000000, 0x380000, 0x200000, 0x080000})
}

func matrimblBitswapByte(i int) int {
	return i ^ (int(bitswapByte(byte(i&0x3), 4, 3, 1, 2, 0, 7, 6, 5)) << 8)
}

// matrimbl uses partial CMC encryption and its own P, M & C encryption
func matrimbl(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			if f.ROM[P], err = k2k2PReader(g.area[P], readers[P], []int{0x100000, 0x280000, 0x300000, 0x180000, 0x000000, 0x380000, 0x200000, 0x080000}); err != nil {
				return err
			}
		case S:
			break
		case M:
			b, err := commonPaddedReader(g.area[M], readers[M])
			if err != nil {
				return err
			}

			rom := make([]byte, 0x30000)
			copy(rom, b)
			copy(rom[0x10000:], b)

			buf := make([]byte, 0x20000)
			copy(buf, rom[0x10000:])
			for i, j := 0, 0; i < 0x20000; i++ {
				if i&0x10000 != 0 {
					if i&0x800 != 0 {
						j = matrimblBitswapByte(i) ^ 0x10000
					} else {
						j = matrimblBitswapByte(i ^ 0x01)
					}
				} else {
					if i&0x800 != 0 {
						j = matrimblBitswapByte(i^0x01) ^ 0x10000
					} else {
						j = matrimblBitswapByte(i)
					}
				}
				rom[0x10000+j] = buf[i]
			}
			copy(rom[:0x10000], rom[0x10000:])
			// XXX Not sure why I have to byteswap this?
			// This makes things match the NeoBuilder output, but it's not obvious from the MAME sources
			for i := 0x10000; i < 0x20000; i += 2 {
				rom[i+0], rom[i+1] = rom[i+1], rom[i+0]
			}
			f.ROM[M] = rom[:0x20000]
		case V1:
			if f.ROM[V1], err = commonPaddedReader(g.area[V1], readers[V1]); err != nil {
				return err
			}
			// XXX Not sure why I have to byteswap this?
			// This makes things match the NeoBuilder output, but it's not obvious from the MAME sources
			for i := 0x0400000; i < 0x0800000; i += 2 {
				f.ROM[V1][i+0], f.ROM[V1][i+1] = f.ROM[V1][i+1], f.ROM[V1][i+0]
			}
			for i := 0x0c00000; i < 0x1000000; i += 2 {
				f.ROM[V1][i+0], f.ROM[V1][i+1] = f.ROM[V1][i+1], f.ROM[V1][i+0]
			}
		case C:
			b, err := commonCReader(g.area[C], readers[C])
			if err != nil {
				return err
			}
			f.ROM[S] = cmcSfixDecrypt(b, int(g.area[S].size))
			f.ROM[C] = cthdDecrypt(b)
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// mslug3 uses SMA and CMC42 encryption
func mslug3(f *File, g mameGame, readers [][]io.Reader) error {
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
func mslug3a(f *File, g mameGame, readers [][]io.Reader) error {
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

// mslug3b6 uses CMC42 encryption and its own S area encryption
func mslug3b6(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			// Only read 1 MB of the first ROM
			if f.ROM[P], err = ioutil.ReadAll(io.MultiReader(io.LimitReader(readers[P][0], 0x100000), readers[P][1])); err != nil {
				return err
			}
		case S:
			b, err := commonPaddedReader(g.area[S], readers[S])
			if err != nil {
				return err
			}
			f.ROM[S] = sxDecrypt(b, 2)
		case C:
			b, err := commonCReader(g.area[C], readers[C])
			if err != nil {
				return err
			}
			f.ROM[C] = cmc42GfxDecrypt(b, mslug3GfxKey)
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// mslug3h uses CMC42 encryption
func mslug3h(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, mslug3GfxKey)
}

// ms4plus uses PCM2 and CMC50 encryption but doesn't decrypt the S area
func ms4plus(f *File, g mameGame, readers [][]io.Reader) error {
	return commonPCM2Reader(f, g, readers, mslug4GfxKey, false, 8)
}

// ms5plus uses PCM2 and CMC50 encryption and its own S area encryption
func ms5plus(f *File, g mameGame, readers [][]io.Reader) error {
	for i := 0; i < Areas; i++ {
		var err error
		switch i {
		case P:
			if f.ROM[P], err = commonPReader(g.area[P], readers[P], regexp.MustCompile(`\.ep`)); err != nil {
				return err
			}
		case S:
			b, err := commonPaddedReader(g.area[S], readers[S])
			if err != nil {
				return err
			}
			f.ROM[S] = sxDecrypt(b, 1)
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
			f.ROM[V1] = pcm2Swap(b, 2)
		case C:
			b, err := commonCReader(g.area[C], readers[C])
			if err != nil {
				return err
			}
			f.ROM[C] = cmc50GfxDecrypt(b, mslug5GfxKey)
		default:
			if f.ROM[i], err = commonPaddedReader(g.area[i], readers[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// mslug4 uses PCM2 and CMC50 encryption
func mslug4(f *File, g mameGame, readers [][]io.Reader) error {
	return commonPCM2Reader(f, g, readers, mslug4GfxKey, true, 8)
}

// mslug5 uses PVC, PCM2 and CMC50 encryption
func mslug5(f *File, g mameGame, readers [][]io.Reader) error {
	return commonPVCReader(f, g, readers, [0x20]byte{0xc2, 0x4b, 0x74, 0xfd, 0x0b, 0x34, 0xeb, 0xd7, 0x10, 0x6d, 0xf9, 0xce, 0x5d, 0xd5, 0x61, 0x29, 0xf5, 0xbe, 0x0d, 0x82, 0x72, 0x45, 0x0f, 0x24, 0xb3, 0x34, 0x1b, 0x99, 0xea, 0x09, 0xf3, 0x03}, [0x20]byte{0x36, 0x09, 0xb0, 0x64, 0x95, 0x0f, 0x90, 0x42, 0x6e, 0x0f, 0x30, 0xf6, 0xe5, 0x08, 0x30, 0x64, 0x08, 0x04, 0x00, 0x2f, 0x72, 0x09, 0xa0, 0x13, 0xc9, 0x0b, 0xa0, 0x3e, 0xc2, 0x00, 0x40, 0x2b}, []int{15, 14, 13, 12, 10, 11, 8, 9, 6, 7, 4, 5, 3, 2, 1, 0}, []int{7, 6, 5, 4, 1, 0, 3, 2}, []int{5, 4, 7, 6, 1, 0, 3, 2}, 0x00700, 2, mslug5GfxKey)
}

// nitd uses CMC42 encryption
func nitd(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, nitdGfxKey)
}

// pbobblenb is standard apart from the ADPCM area has 2 MB of empty space prepended
func pbobblenb(f *File, g mameGame, readers [][]io.Reader) error {
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
func pnyaa(f *File, g mameGame, readers [][]io.Reader) error {
	return commonPCM2Reader(f, g, readers, pnyaaGfxKey, true, 4)
}

// preisle2 uses CMC42 encryption
func preisle2(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, preisle2GfxKey)
}

// rotd uses PCM2 and CMC50 encryption
func rotd(f *File, g mameGame, readers [][]io.Reader) error {
	return commonPCM2Reader(f, g, readers, rotdGfxKey, true, 16)
}

// s1945p uses CMC42 encryption
func s1945p(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, s1945pGfxKey)
}

// samsh5sp uses PCM2, CMC50 encryption and its own P encryption
func samsh5sp(f *File, g mameGame, readers [][]io.Reader) error {
	return commonK2K2Reader(f, g, readers, samsho5spGfxKey, true, 6, []int{0x000000, 0x080000, 0x500000, 0x480000, 0x600000, 0x580000, 0x700000, 0x280000, 0x100000, 0x680000, 0x400000, 0x780000, 0x200000, 0x380000, 0x300000, 0x180000})
}

// samsho5 uses PCM2, CMC50 encryption and its own P encryption
func samsho5(f *File, g mameGame, readers [][]io.Reader) error {
	return commonK2K2Reader(f, g, readers, samsho5GfxKey, true, 4, []int{0x000000, 0x080000, 0x700000, 0x680000, 0x500000, 0x180000, 0x200000, 0x480000, 0x300000, 0x780000, 0x600000, 0x280000, 0x100000, 0x580000, 0x400000, 0x380000})
}

// sengoku3 uses CMC42 encryption
func sengoku3(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, sengoku3GfxKey)
}

// svc uses PVC, PCM2 and CMC50 encryption
func svc(f *File, g mameGame, readers [][]io.Reader) error {
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

func viewpoin(f *File, g mameGame, readers [][]io.Reader) error {
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
func zupapa(f *File, g mameGame, readers [][]io.Reader) error {
	return commonCMC42Reader(f, g, readers, zupapaGfxKey)
}
