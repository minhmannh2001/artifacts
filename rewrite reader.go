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

	// Return buffered data first
	if len(ar.buffer) > 0 {
		n := copy(p, ar.buffer)
		ar.buffer = ar.buffer[n:]
		return n, nil
	}

	// Initial format detection
	if ar.checkMode {
		header := make([]byte, headerSize)
		n, err := io.ReadFull(ar.reader, header)
		
		// Handle EOF during initial read
		if err == io.EOF {
			if n == 0 {
				return 0, io.EOF
			}
			ar.checkMode = false
			return copy(p, header[:n]), nil
		}

		// Handle partial header
		if err == io.ErrUnexpectedEOF {
			ar.checkMode = false
			return copy(p, header[:n]), nil
		}

		// Handle other errors
		if err != nil {
			return 0, err
		}

		// Check for valid Docker header
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

		// Not a Docker header, switch to regular mode and handle header as regular data
		ar.checkMode = false
		n = copy(p, header)
		if n < len(header) {
			ar.buffer = header[n:]
			return n, nil
		}

		// If we copied all header bytes and have room for more, continue reading
		remaining := p[n:]
		if len(remaining) > 0 {
			m, err := ar.reader.Read(remaining)
			return n + m, err
		}
		return n, nil
	}

	// Regular read mode
	return ar.reader.Read(p)
}