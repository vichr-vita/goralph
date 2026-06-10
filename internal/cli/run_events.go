package cli

import (
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

type runEvent struct {
	Type      string          `json:"type"`
	RunID     int64           `json:"run_id,omitempty"`
	TaskID    int64           `json:"task_id,omitempty"`
	Mode      string          `json:"mode,omitempty"`
	Status    string          `json:"status,omitempty"`
	Message   string          `json:"message,omitempty"`
	Runs      int             `json:"runs,omitempty"`
	Stream    string          `json:"stream,omitempty"`
	Data      string          `json:"data,omitempty"`
	Progress  *progressOutput `json:"progress,omitempty"`
	Run       *runOutput      `json:"run,omitempty"`
	Error     string          `json:"error,omitempty"`
	Timestamp string          `json:"timestamp"`
}

type runEventWriter struct {
	mu      sync.Mutex
	encoder *json.Encoder
	err     error
}

func newRunEventWriter(cmd *cobra.Command) *runEventWriter {
	if !jsonOutputFromContext(cmd.Context()) {
		return nil
	}
	return &runEventWriter{encoder: json.NewEncoder(cmd.OutOrStdout())}
}

func (w *runEventWriter) Emit(event runEvent) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return w.err
	}
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := w.encoder.Encode(event); err != nil {
		w.err = err
		return err
	}
	return nil
}

func (w *runEventWriter) Err() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.err
}

func (w *runEventWriter) OutputWriter(stream string) io.Writer {
	return runEventOutputWriter{events: w, stream: stream}
}

type runEventOutputWriter struct {
	events *runEventWriter
	stream string
}

func (w runEventOutputWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := w.events.Emit(runEvent{Type: "runner_output", Stream: w.stream, Data: string(p)}); err != nil {
		return 0, err
	}
	return len(p), nil
}
