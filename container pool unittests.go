package containerpool

import (
	"bufio"
	"datafeedctl/internal/app/jobworker/worker/shared"
	"datafeedctl/internal/app/jobworker/worker/tokenstore"
	"encoding/json"
	"strings"
	"testing"
)

// mockWriteCloser implements io.WriteCloser for testing
type mockWriteCloser struct {
	written []byte
}

func (m *mockWriteCloser) Write(p []byte) (n int, err error) {
	m.written = append(m.written, p...)
	return len(p), nil
}

func (m *mockWriteCloser) Close() error {
	return nil
}

func TestDockerContainer_Run(t *testing.T) {
	tests := []struct {
		name          string
		input         shared.DatafeedJob
		tokens        tokenstore.TenantTokens
		containerOuts []string
		want          shared.DatafeedOutput
		wantErr       bool
	}{
		{
			name: "successful execution",
			input: shared.DatafeedJob{
				Name:      "test-job",
				TaskID:    "task-123",
				RequestID: "req-456",
				Context:   `{"script": "test.py", "command": "python", "args": {}}`,
				Args:      map[string]interface{}{},
			},
			tokens: tokenstore.TenantTokens{
				TenantToken:         "test-token",
				TenantDatafeedToken: "datafeed-token",
			},
			containerOuts: []string{
				`{"type": "result", "results": {"data": "test"}, "results_type": "json"}`,
				`{"type": "completed"}`,
			},
			want: shared.DatafeedOutput{
				Name:      "test-job",
				TaskId:    "task-123",
				RequestID: "req-456",
				Payload:   `{"Type":1,"Contents":{"data":"test"},"ContentsFormat":"json"}`,
			},
			wantErr: false,
		},
		{
			name: "error execution",
			input: shared.DatafeedJob{
				Name:      "test-job",
				TaskID:    "task-123",
				RequestID: "req-456",
				Context:   `{"script": "test.py", "command": "python", "args": {}}`,
				Args:      map[string]interface{}{},
			},
			tokens: tokenstore.TenantTokens{
				TenantToken:         "test-token",
				TenantDatafeedToken: "datafeed-token",
			},
			containerOuts: []string{
				`{"type": "error", "error_message": "execution failed"}`,
				`{"type": "completed"}`,
			},
			want: shared.DatafeedOutput{
				Name:      "test-job",
				TaskId:    "task-123",
				RequestID: "req-456",
				Payload:   `{"Type":2,"Contents":"Task failed: execution failed"}`,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock stdin/stdout
			mockStdin := &mockWriteCloser{}
			mockStdoutReader := strings.NewReader(strings.Join(tt.containerOuts, "\n"))
			mockStdout := bufio.NewScanner(mockStdoutReader)

			container := &DockerContainer{
				ID:     "test-container",
				State:  Free,
				Stdin:  mockStdin,
				Stdout: mockStdout,
			}

			got, err := container.Run(tt.input, tt.tokens)
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Compare outputs
			if got.Name != tt.want.Name || got.TaskId != tt.want.TaskId || got.RequestID != tt.want.RequestID {
				t.Errorf("Run() metadata mismatch, got = %v, want %v", got, tt.want)
			}

			// Compare payloads (need to compare as JSON to ignore formatting differences)
			var gotPayload, wantPayload interface{}
			if err := json.Unmarshal([]byte(got.Payload), &gotPayload); err != nil {
				t.Errorf("Failed to unmarshal got payload: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.want.Payload), &wantPayload); err != nil {
				t.Errorf("Failed to unmarshal want payload: %v", err)
			}
			if !deepEqual(gotPayload, wantPayload) {
				t.Errorf("Run() payload mismatch\ngot  = %v\nwant = %v", got.Payload, tt.want.Payload)
			}
		})
	}
}

func TestCheckAlive(t *testing.T) {
	tests := []struct {
		name          string
		containerOut  string
		expectedAlive bool
	}{
		{
			name:          "container is alive",
			containerOut:  `{"type": "check_alive_output"}`,
			expectedAlive: true,
		},
		{
			name:          "container is not alive",
			containerOut:  `{"type": "error"}`,
			expectedAlive: false,
		},
		{
			name:          "invalid json response",
			containerOut:  `invalid json`,
			expectedAlive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStdin := &mockWriteCloser{}
			mockStdoutReader := strings.NewReader(tt.containerOut)
			mockStdout := bufio.NewScanner(mockStdoutReader)

			container := &DockerContainer{
				ID:     "test-container",
				State:  Free,
				Stdin:  mockStdin,
				Stdout: mockStdout,
			}

			alive := container.CheckAlive()
			if alive != tt.expectedAlive {
				t.Errorf("CheckAlive() = %v, want %v", alive, tt.expectedAlive)
			}
		})
	}
}

func TestAddEnvVarsToContext(t *testing.T) {
	tests := []struct {
		name        string
		contextJSON string
		tokens      tokenstore.TenantTokens
		want        string
		wantErr     bool
	}{
		{
			name:        "valid context",
			contextJSON: `{"script": "test.py"}`,
			tokens: tokenstore.TenantTokens{
				TenantToken:         "test-token",
				TenantDatafeedToken: "datafeed-token",
			},
			want:    `{"env_vars":"orenctl_api_key=test-token,datafeed_api_token=datafeed-token","script":"test.py"}`,
			wantErr: false,
		},
		{
			name:        "invalid JSON",
			contextJSON: `invalid json`,
			tokens: tokenstore.TenantTokens{
				TenantToken:         "test-token",
				TenantDatafeedToken: "datafeed-token",
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := addEnvVarsToContext(tt.contextJSON, tt.tokens)
			if (err != nil) != tt.wantErr {
				t.Errorf("addEnvVarsToContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Compare as JSON to ignore formatting differences
				var gotJSON, wantJSON map[string]interface{}
				if err := json.Unmarshal([]byte(got), &gotJSON); err != nil {
					t.Errorf("Failed to unmarshal got JSON: %v", err)
				}
				if err := json.Unmarshal([]byte(tt.want), &wantJSON); err != nil {
					t.Errorf("Failed to unmarshal want JSON: %v", err)
				}
				if !deepEqual(gotJSON, wantJSON) {
					t.Errorf("addEnvVarsToContext() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// Helper function to compare two interfaces deeply
func deepEqual(a, b interface{}) bool {
	aJson, _ := json.Marshal(a)
	bJson, _ := json.Marshal(b)
	return string(aJson) == string(bJson)
}