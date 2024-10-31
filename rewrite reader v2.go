package reader

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

type adaptiveReader struct {
	reader io.Reader
	buffer []byte
}

func (ar *adaptiveReader) Read(p []byte) (int, error) {
	if len(ar.buffer) > 0 {
		n := copy(p, ar.buffer)
		ar.buffer = ar.buffer[n:]
		return n, nil
	}

	header := make([]byte, 8)
	_, err := io.ReadFull(ar.reader, header)
	if err != nil {
		return 0, err
	}

	// Only handle Docker stream headers
	if header[0] != 1 && header[0] != 2 {
		return 0, io.ErrUnexpectedEOF
	}

	size := int(binary.BigEndian.Uint32(header[4:]))
	data := make([]byte, size)
	_, err = io.ReadFull(ar.reader, data)
	if err != nil {
		return 0, err
	}

	n := copy(p, data)
	if n < len(data) {
		ar.buffer = data[n:]
	}
	return n, nil
}

func TestAdaptiveReader(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		readSize    int
		wantData    []byte
		wantErr     error
		description string
	}{
		{
			name: "standard docker stream",
			input: append([]byte{
				1,                // stream type
				0, 0, 0,         // padding
				0, 0, 0, 5,      // size (5 bytes)
				'h', 'e', 'l', 'l', 'o', // actual data
			}),
			readSize:    10,
			wantData:    []byte("hello"),
			wantErr:     nil,
			description: "Should correctly read a standard Docker stream packet",
		},
		{
			name: "stderr docker stream",
			input: append([]byte{
				2,                // stream type (stderr)
				0, 0, 0,         // padding
				0, 0, 0, 4,      // size (4 bytes)
				't', 'e', 's', 't', // actual data
			}),
			readSize:    10,
			wantData:    []byte("test"),
			wantErr:     nil,
			description: "Should correctly read a stderr Docker stream packet",
		},
		{
			name: "small buffer requiring multiple reads",
			input: append([]byte{
				1,                // stream type
				0, 0, 0,         // padding
				0, 0, 0, 6,      // size (6 bytes)
				'b', 'u', 'f', 'f', 'e', 'r', // actual data
			}),
			readSize:    2,
			wantData:    []byte("bu"),
			wantErr:     nil,
			description: "Should handle small buffer requiring multiple reads",
		},
		{
			name: "invalid stream type",
			input: append([]byte{
				3,                // invalid stream type
				0, 0, 0,         // padding
				0, 0, 0, 4,      // size
				't', 'e', 's', 't', // data
			}),
			readSize:    10,
			wantData:    nil,
			wantErr:     io.ErrUnexpectedEOF,
			description: "Should return error for invalid stream type",
		},
		{
			name:        "empty input",
			input:       []byte{},
			readSize:    10,
			wantData:    nil,
			wantErr:     io.EOF,
			description: "Should handle empty input correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create reader with test input
			ar := &adaptiveReader{
				reader: bytes.NewReader(tt.input),
			}

			// Read from the adaptive reader
			buf := make([]byte, tt.readSize)
			n, err := ar.Read(buf)

			// Check error
			if err != tt.wantErr {
				t.Errorf("Read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// If we expect an error, don't check the data
			if tt.wantErr != nil {
				return
			}

			// Check data
			if n != len(tt.wantData) {
				t.Errorf("Read() n = %v, want %v", n, len(tt.wantData))
			}
			if !bytes.Equal(buf[:n], tt.wantData) {
				t.Errorf("Read() = %v, want %v", buf[:n], tt.wantData)
			}
		})
	}
}

func TestMultipleReads(t *testing.T) {
	// Create a Docker stream with 10 bytes of data
	input := append([]byte{
		1,    // stream type
		0, 0, 0, // padding
		0, 0, 0, 10, // size (10 bytes)
	}, []byte("helloworld")...)

	ar := &adaptiveReader{
		reader: bytes.NewReader(input),
	}

	// First read - 4 bytes
	buf1 := make([]byte, 4)
	n1, err := ar.Read(buf1)
	if err != nil {
		t.Fatalf("First read failed: %v", err)
	}
	if n1 != 4 || string(buf1) != "hell" {
		t.Errorf("First read: got %v, want 'hell'", string(buf1))
	}

	// Second read - 4 bytes
	buf2 := make([]byte, 4)
	n2, err := ar.Read(buf2)
	if err != nil {
		t.Fatalf("Second read failed: %v", err)
	}
	if n2 != 4 || string(buf2) != "owor" {
		t.Errorf("Second read: got %v, want 'owor'", string(buf2))
	}

	// Third read - remaining 2 bytes
	buf3 := make([]byte, 4)
	n3, err := ar.Read(buf3)
	if err != nil {
		t.Fatalf("Third read failed: %v", err)
	}
	if n3 != 2 || string(buf3[:n3]) != "ld" {
		t.Errorf("Third read: got %v, want 'ld'", string(buf3[:n3]))
	}

	// Fourth read - should return EOF
	_, err = ar.Read(buf1)
	if err != io.EOF {
		t.Errorf("Fourth read: got error %v, want EOF", err)
	}
}