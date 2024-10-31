package reader

import (
	"encoding/binary"
	"io"
)

const (
	StdoutStream = 1
	StderrStream = 2
	headerSize   = 8
)

type adaptiveReader struct {
	reader    io.Reader
	buffer    []byte
	isDocker  bool
	checkMode bool
}

func NewAdaptiveReader(r io.Reader) *adaptiveReader {
	return &adaptiveReader{
		reader:    r,
		checkMode: true,
	}
}

func (ar *adaptiveReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	// Return buffered data first if available
	if len(ar.buffer) > 0 {
		n := copy(p, ar.buffer)
		ar.buffer = ar.buffer[n:]
		return n, nil
	}

	// Initial format detection
	if ar.checkMode {
		header := make([]byte, headerSize)
		n, err := io.ReadFull(ar.reader, header)

		// Handle EOF or partial data during initial read
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			if n == 0 {
				return 0, io.EOF
			}
			ar.checkMode = false
			copied := copy(p, header[:n])
			if copied < n {
				ar.buffer = header[copied:n]
			}
			return copied, nil
		}

		// Handle other errors
		if err != nil {
			return 0, err
		}

		// We have a full header, check if it's Docker format
		if (header[0] == StdoutStream || header[0] == StderrStream) && header[1] == 0 && header[2] == 0 && header[3] == 0 {
			ar.isDocker = true
			size := int(binary.BigEndian.Uint32(header[4:]))
			
			data := make([]byte, size)
			_, err = io.ReadFull(ar.reader, data)
			if err != nil {
				return 0, err
			}

			n = copy(p, data)
			if n < len(data) {
				ar.buffer = data[n:]
			}
			return n, nil
		}

		// Not a Docker header, treat as regular data
		ar.checkMode = false
        
        // Copy header bytes first
        n = copy(p, header)
        if n < len(header) {
            ar.buffer = header[n:]
            return n, nil
        }

        // If we have more space in p, try to read more data
        if len(p) > n {
            m, err := ar.reader.Read(p[n:])
            if err != nil && err != io.EOF {
                return n, err
            }
            return n + m, err
        }
        
        return n, nil
	}

	// Regular read mode - just pass through to underlying reader
	return ar.reader.Read(p)
}