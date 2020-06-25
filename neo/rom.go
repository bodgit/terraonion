package neo

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

var errInvalidInterleave = errors.New("can only interleave two or four ROMs")

func interleaveROM(width int64, readers ...io.Reader) (io.Reader, error) {
	eof := 1<<len(readers) - 1

	var order []int
	switch len(readers) {
	case 2:
		order = []int{0, 1}
	case 4:
		order = []int{0, 2, 1, 3}
	default:
		return nil, errInvalidInterleave
	}

	// Use a bufio.Reader to wrap each reader otherwise interleaving real
	// files with a small width is very CPU intensive and slow
	bufferedReaders := make([]*bufio.Reader, len(readers))
	for i := range readers {
		// If readers[i] is already somehow a bufio.Reader it's a no-op
		bufferedReaders[i] = bufio.NewReader(readers[i])
	}

	var tail []int64

	b := new(bytes.Buffer)
loop:
	for {
		for i := range order {
			n, err := io.CopyN(b, bufferedReaders[order[i]], width)
			if err != nil {
				if err != io.EOF {
					return nil, err
				}
				eof &= ^(1 << order[i])
			}

			if n < width {
				b.Write(bytes.Repeat([]byte{0}, int(width-n)))
			}

			if n == 0 || len(tail) > 0 {
				tail = append(tail, n)
			}

			if eof == 0 {
				break loop
			}
		}
	}

	for i := len(tail) - 1; i >= 0; i-- {
		if tail[i] == 0 {
			b.Truncate(b.Len() - int(width))
		} else {
			break
		}
	}

	return b, nil
}
