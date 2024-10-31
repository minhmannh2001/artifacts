package reader

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestAdaptiveReader_RegularData(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		readSize int
		want     []byte
		wantErr  error
	}{
		{
			name:     "read all data at once",
			input:    []byte("hello world"),
			readSize: 11,
			want:     []byte("hello world"),
			wantErr:  nil,
		},
		{
			name:     "read data in chunks",
			input:    []byte("hello world"),
			readSize: 5,
			want:     []byte("hello"),
			wantErr:  nil,
		},
		{
			name:     "empty input",
			input:    []byte{},
			readSize: 5,
			want:     nil,
			wantErr:  io.EOF,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &adaptiveReader{
				reader: bytes.NewReader(tt.input),
			}

			got := make([]byte, tt.readSize)
			n, err := reader.Read(got)

			if err != tt.wantErr {
				t.Errorf("Read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if n != len(tt.want) {
					t.Errorf("Read() n = %v, want %v", n, len(tt.want))
				}
				if !bytes.Equal(got[:n], tt.want) {
					t.Errorf("Read() = %v, want %v", got[:n], tt.want)
				}
			}
		})
	}
}

func TestAdaptiveReader_DockerStream(t *testing.T) {
	tests := []struct {
		name     string
		stream   byte
		data     []byte
		readSize int
		want     []byte
		wantErr  error
	}{
		{
			name:     "stdout stream full read",
			stream:   StdoutStream,
			data:     []byte("docker output"),
			readSize: 100,
			want:     []byte("docker output"),
			wantErr:  nil,
		},
		{
			name:     "stderr stream full read",
			stream:   StderrStream,
			data:     []byte("error message"),
			readSize: 100,
			want:     []byte("error message"),
			wantErr:  nil,
		},
		{
			name:     "partial read requiring buffer",
			stream:   StdoutStream,
			data:     []byte("long message"),
			readSize: 5,
			want:     []byte("long "),
			wantErr:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create Docker stream header
			header := make([]byte, 8)
			header[0] = tt.stream
			binary.BigEndian.PutUint32(header[4:], uint32(len(tt.data)))

			// Combine header and data
			input := append(header, tt.data...)
			reader := &adaptiveReader{
				reader: bytes.NewReader(input),
			}

			got := make([]byte, tt.readSize)
			n, err := reader.Read(got)

			if err != tt.wantErr {
				t.Errorf("Read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if n != len(tt.want) {
					t.Errorf("Read() n = %v, want %v", n, len(tt.want))
				}
				if !bytes.Equal(got[:n], tt.want) {
					t.Errorf("Read() = %v, want %v", got[:n], tt.want)
				}
			}
		})
	}
}

func TestAdaptiveReader_BufferedData(t *testing.T) {
	t.Run("read buffered data from previous read", func(t *testing.T) {
		reader := &adaptiveReader{
			reader: bytes.NewReader([]byte("test")),
			buffer: []byte("buffered"),
		}

		got := make([]byte, 4)
		n, err := reader.Read(got)

		if err != nil {
			t.Errorf("Read() unexpected error = %v", err)
		}

		want := []byte("buff")
		if n != len(want) {
			t.Errorf("Read() n = %v, want %v", n, len(want))
		}
		if !bytes.Equal(got[:n], want) {
			t.Errorf("Read() = %v, want %v", got[:n], want)
		}
	})
}

func TestAdaptiveReader_PartialHeader(t *testing.T) {
	t.Run("partial header with EOF", func(t *testing.T) {
		reader := &adaptiveReader{
			reader: bytes.NewReader([]byte{1, 2, 3}), // Incomplete header
		}

		got := make([]byte, 10)
		n, err := reader.Read(got)

		if err != nil {
			t.Errorf("Read() unexpected error = %v", err)
		}

		want := []byte{1, 2, 3}
		if n != len(want) {
			t.Errorf("Read() n = %v, want %v", n, len(want))
		}
		if !bytes.Equal(got[:n], want) {
			t.Errorf("Read() = %v, want %v", got[:n], want)
		}
	})
}

func TestAdaptiveReader_MultipleReads(t *testing.T) {
	// Create Docker stream header
	header := make([]byte, 8)
	header[0] = StdoutStream
	data := []byte("test message")
	binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
	input := append(header, data...)

	reader := &adaptiveReader{
		reader: bytes.NewReader(input),
	}

	// First read
	got1 := make([]byte, 4)
	n1, err1 := reader.Read(got1)
	if err1 != nil {
		t.Fatalf("First Read() error = %v", err1)
	}

	// Second read
	got2 := make([]byte, 100)
	n2, err2 := reader.Read(got2)
	if err2 != nil {
		t.Fatalf("Second Read() error = %v", err2)
	}

	// Verify complete message was read
	complete := append(got1[:n1], got2[:n2]...)
	if !bytes.Equal(complete, data) {
		t.Errorf("Multiple reads = %v, want %v", complete, data)
	}
}