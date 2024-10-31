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
	isDocker  bool // Indicates if we've detected Docker format
	checkMode bool // True when we're still determining the format
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

	// If we have buffered data, return it first
	if len(ar.buffer) > 0 {
		n := copy(p, ar.buffer)
		ar.buffer = ar.buffer[n:]
		return n, nil
	}

	// If we're in check mode, try to determine the format
	if ar.checkMode {
		header := make([]byte, headerSize)
		n, err := io.ReadFull(ar.reader, header)
		
		// Handle EOF or partial read during format detection
		if err == io.EOF {
			if n == 0 {
				return 0, io.EOF
			}
			ar.checkMode = false
			return copy(p, header[:n]), nil
		}
		
		// Handle other errors during format detection
		if err != nil && err != io.ErrUnexpectedEOF {
			return 0, err
		}

		// If we got a partial header, treat as regular data
		if err == io.ErrUnexpectedEOF {
			ar.checkMode = false
			return copy(p, header[:n]), nil
		}

		// Check if this looks like a Docker stream header
		if (header[0] == StdoutStream || header[0] == StderrStream) && header[1] == 0 && header[2] == 0 && header[3] == 0 {
			ar.isDocker = true
			size := int(binary.BigEndian.Uint32(header[4:]))
			
			// Read the actual data
			data := make([]byte, size)
			_, err = io.ReadFull(ar.reader, data)
			if err != nil {
				return 0, err
			}

			// Copy what we can to p, buffer the rest
			n = copy(p, data)
			if n < len(data) {
				ar.buffer = data[n:]
			}
			return n, nil
		}

        // Not a Docker header, treat as regular data
        ar.checkMode = false
        n = copy(p, header)
        if n < len(header) {
            ar.buffer = header[n:]
        }
        return n, nil
	}

	// Regular read mode
	n, err := ar.reader.Read(p)
	if err != nil && err != io.EOF {
		return n, err
	}
	return n, err
}