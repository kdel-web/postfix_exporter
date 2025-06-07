package main

import (
	"context"
	"io"

	"github.com/nxadm/tail"
)

// A FileLogSource can read lines from a file.
type FileLogSource struct {
	tailer *tail.Tail
}

// NewFileLogSource creates a new log source, tailing the given file.
func NewFileLogSource(ctx context.Context, path string) (*FileLogSource, error) {
	tailer, err := tail.TailFile(path, tail.Config{
		ReOpen:    true,                               // reopen the file if it's rotated
		MustExist: true,                               // fail immediately if the file is missing or has incorrect permissions
		Follow:    true,                               // run in follow mode
		Location:  &tail.SeekInfo{Whence: io.SeekEnd}, // seek to end of file
		Logger:    tail.DiscardingLogger,
	})
	if err != nil {
		return nil, err
	}
	return &FileLogSource{tailer}, nil
}

func (s *FileLogSource) Close() error {
	defer s.tailer.Cleanup()
	go func() {
		// Stop() waits for the tailer goroutine to shut down, but it
		// can be blocking on sending on the Lines channel...
		for range s.tailer.Lines {
		}
	}()
	return s.tailer.Stop()
}

func (s *FileLogSource) Path() string {
	return s.tailer.Filename
}

func (s *FileLogSource) Read(ctx context.Context) (string, error) {
	select {
	case line, ok := <-s.tailer.Lines:
		if !ok {
			return "", io.EOF
		}
		return line.Text, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// A LogSource is an interface to read log lines.
type LogSource interface {
	// Path returns a representation of the log location.
	Path() string

	// Read returns the next log line. Returns `io.EOF` at the end of
	// the log.
	Read(context.Context) (string, error)
}
