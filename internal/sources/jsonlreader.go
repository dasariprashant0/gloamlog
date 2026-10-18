package sources

import (
	"bytes"
	"io"
	"os"
)

// JSONLReader streams a (possibly huge) JSON-lines file one line at a time without holding the file in memory. Lines that sit
// inside one chunk are handed over without a copy, a small carry buffer joins lines that cross a chunk boundary, and lines
// longer than MaxLine (images, file dumps) are skipped without being buffered. A last line with no newline is left unread: it
// may still be being written (EndOffset says where to resume).
type JSONLReader struct {
	Path      string
	MaxLine   int // 0 = 8 MiB
	ChunkSize int // 0 = 4 MiB
}

// LineOutcome says where reading stopped and how many over-long lines were skipped.
type LineOutcome struct {
	EndOffset   int64
	SkippedLong int
}

// ForEachLine calls body for every complete, non-empty line in [offset, min(size at open, offset+limit)); limit < 0 = to the end.
// The slice is only valid during the call: copy it if you keep it. body returns false to stop reading.
func (r JSONLReader) ForEachLine(offset, limit int64, body func(line []byte) bool) (LineOutcome, error) {
	maxLine, chunkSize := r.MaxLine, r.ChunkSize
	if maxLine <= 0 {
		maxLine = 8 << 20
	}
	if chunkSize <= 0 {
		chunkSize = 4 << 20
	}
	f, err := os.Open(r.Path)
	if err != nil {
		return LineOutcome{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return LineOutcome{}, err
	}
	size := st.Size()
	if limit >= 0 && offset+limit < size {
		size = offset + limit
	}
	if offset >= size {
		return LineOutcome{EndOffset: min(offset, size)}, nil
	}

	lineStart, bufBase := offset, offset
	var carry []byte
	skipping, stopped, skipped := false, false, 0
	buf := make([]byte, chunkSize)
	for bufBase < size && !stopped {
		want := int(min(int64(chunkSize), size-bufBase))
		n, err := f.ReadAt(buf[:want], bufBase)
		if n == 0 {
			if err != nil && err != io.EOF {
				return LineOutcome{}, err
			}
			break
		}
		chunk := buf[:n]
		i := 0
		for i < n {
			nlRel := bytes.IndexByte(chunk[i:], '\n')
			if nlRel < 0 { // no newline in the rest of this chunk
				if !skipping {
					carry = append(carry, chunk[i:]...)
					if len(carry) > maxLine {
						skipping = true
						carry = carry[:0]
					}
				}
				break
			}
			nl := i + nlRel
			piece := chunk[i:nl]
			switch {
			case skipping:
				skipping = false
				skipped++
			case len(carry) == 0: // the whole line is inside this chunk: no copy
				if len(piece) > maxLine {
					skipped++
				} else if len(piece) > 0 && !body(piece) {
					stopped = true
				}
			default:
				carry = append(carry, piece...)
				if len(carry) > maxLine {
					skipped++
				} else if !body(carry) {
					stopped = true
				}
			}
			carry = carry[:0]
			lineStart = bufBase + int64(nl) + 1
			i = nl + 1
			if stopped {
				break
			}
		}
		bufBase += int64(n)
	}
	return LineOutcome{EndOffset: lineStart, SkippedLong: skipped}, nil
}
