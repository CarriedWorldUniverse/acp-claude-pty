package pty

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorKindString(t *testing.T) {
	cases := map[ErrorKind]string{
		ErrCrash:            "crash",
		ErrHang:             "hang",
		ErrPromptTimeout:    "prompt-timeout",
		ErrGracefulEOF:      "graceful-eof",
		ErrAbortedBySIGTERM: "aborted-by-sigterm",
		ErrKilled:           "killed",
		ErrModelInvalid:     "model-invalid",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("ErrorKind(%d).String() = %q, want %q", int(k), got, want)
		}
	}
}

func TestDriverErrorWrapping(t *testing.T) {
	cause := errors.New("read /dev/ptmx: i/o timeout")
	de := &DriverError{Kind: ErrHang, Detail: "no output for 30s", Cause: cause}

	wrapped := fmt.Errorf("send turn 3: %w", de)

	got, ok := AsDriverError(wrapped)
	if !ok {
		t.Fatal("AsDriverError did not unwrap")
	}
	if got.Kind != ErrHang {
		t.Errorf("Kind = %v, want %v", got.Kind, ErrHang)
	}
	if !errors.Is(wrapped, cause) {
		t.Errorf("errors.Is did not find cause through chain")
	}
}
