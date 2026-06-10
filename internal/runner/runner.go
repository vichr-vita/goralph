package runner

import (
	"context"
	"time"
)

// Request describes one prompt execution request.
type Request struct {
	Prompt      string
	WorkDir     string
	Env         []string
	Interactive bool
}

// Metadata describes one runner execution.
type Metadata struct {
	RunnerName    string
	RunnerVersion string
	RunnerModel   string
	SessionID     string
	SessionPath   string
	Command       string
	Args          []string
	PID           int
	Host          string
	StartedAt     time.Time
	FinishedAt    time.Time
	ExitCode      int
	ExitSignal    string
	ExitError     string
}

// Result contains captured runner output and metadata.
type Result struct {
	Metadata Metadata
	Stdout   string
	Stderr   string
}

// Runner executes prompts and returns run metadata.
type Runner interface {
	Run(context.Context, Request) (Result, error)
}
