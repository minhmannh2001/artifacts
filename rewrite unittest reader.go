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
			name:     "read regular data smaller than header size",
			input:    []byte("hello"),
			readSize: 10,
			want:     []byte("hello"),
			wantErr:  nil,
		},
		{
			name:     "read regular data larger than header size",
			input:    []byte("hello world this is a test"),
			readSize: 50,
			want:     []byte("hello world this is a test"),
			wantErr:  nil,
		},
		{
			name:     "read data in small chunks",
			input:    []byte("hello"),
			readSize: 2,
			want:     []byte("he"),
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
			reader := NewAdaptiveReader(bytes.NewReader(tt.input))
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
					t.Errorf("Read() = %q, want %q", got[:n], tt.want)
				}
			}
		})
	}
}

func TestAdaptiveReader_PartialHeader(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		readSize int
		want     []byte
		wantErr  error
	}{
		{
			name:     "partial data less than header size",
			input:    []byte{1, 2, 3},
			readSize: 10,
			want:     []byte{1, 2, 3},
			wantErr:  nil,
		},
		{
			name:     "partial header exactly header size but invalid",
			input:    []byte{3, 3, 3, 3, 0, 0, 0, 5},
			readSize: 10,
			want:     []byte{3, 3, 3, 3, 0, 0, 0, 5},
			wantErr:  nil,
		},
		{
			name:     "read partial header in chunks",
			input:    []byte{1, 2, 3, 4},
			readSize: 2,
			want:     []byte{1, 2},
			wantErr:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := NewAdaptiveReader(bytes.NewReader(tt.input))
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
			name:     "valid docker stdout stream",
			stream:   StdoutStream,
			data:     []byte("hello"),
			readSize: 10,
			want:     []byte("hello"),
			wantErr:  nil,
		},
		{
			name:     "valid docker stderr stream",
			stream:   StderrStream,
			data:     []byte("error"),
			readSize: 10,
			want:     []byte("error"),
			wantErr:  nil,
		},
		{
			name:     "docker stream with small read buffer",
			stream:   StdoutStream,
			data:     []byte("hello world"),
			readSize: 5,
			want:     []byte("hello"),
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
			reader := NewAdaptiveReader(bytes.NewReader(input))

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
					t.Errorf("Read() = %q, want %q", got[:n], tt.want)
				}
			}
		})
	}
}

func TestAdaptiveReader_MultipleReads(t *testing.T) {
	tests := []struct {
		name      string
		input     []byte
		readSizes []int
		wants     [][]byte
		wantErrs  []error
	}{
		{
			name:      "multiple reads of regular data",
			input:     []byte("hello world"),
			readSizes: []int{5, 4, 2},
			wants:     [][]byte{[]byte("hello"), []byte(" wor"), []byte("ld")},
			wantErrs:  []error{nil, nil, nil},
		},
		{
			name:      "multiple reads with docker stream",
			input: func() []byte {
				header := make([]byte, 8)
				header[0] = StdoutStream
				data := []byte("hello world")
				binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
				return append(header, data...)
			}(),
			readSizes: []int{5, 6},
			wants:     [][]byte{[]byte("hello"), []byte(" world")},
			wantErrs:  []error{nil, nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := NewAdaptiveReader(bytes.NewReader(tt.input))
			
			for i, size := range tt.readSizes {
				got := make([]byte, size)
				n, err := reader.Read(got)

				if err != tt.wantErrs[i] {
					t.Errorf("Read %d error = %v, wantErr %v", i, err, tt.wantErrs[i])
					return
				}

				if err == nil {
					if n != len(tt.wants[i]) {
						t.Errorf("Read %d n = %v, want %v", i, n, len(tt.wants[i]))
					}
					if !bytes.Equal(got[:n], tt.wants[i]) {
						t.Errorf("Read %d = %q, want %q", i, got[:n], tt.wants[i])
					}
				}
			}
		})
	}
}