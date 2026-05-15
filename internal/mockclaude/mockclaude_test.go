package mockclaude

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_DefaultEchoesInputs(t *testing.T) {
	in := strings.NewReader("hello\rworld\r")
	var out bytes.Buffer
	if err := Run(in, &out, Script{Init: []byte("init")}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := out.String()
	if !strings.HasPrefix(got, "init") {
		t.Errorf("output missing init: %q", got)
	}
	if !strings.Contains(got, "echo: hello") || !strings.Contains(got, "echo: world") {
		t.Errorf("expected echoed inputs in output: %q", got)
	}
}

func TestRun_EmitsBusyThenIdleTitlePerTurn(t *testing.T) {
	in := strings.NewReader("x\r")
	var out bytes.Buffer
	if err := Run(in, &out, Script{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := out.Bytes()

	idleIdx := bytes.Index(got, IdleTitle("done"))
	busyIdx := bytes.Index(got, BusyTitle("…"))
	if busyIdx < 0 {
		t.Fatal("missing busy title in output")
	}
	if idleIdx < 0 {
		t.Fatal("missing idle title in output")
	}
	if busyIdx >= idleIdx {
		t.Errorf("busy must precede idle: busy=%d idle=%d", busyIdx, idleIdx)
	}
}

func TestRun_MatchesByInputPrefix(t *testing.T) {
	in := strings.NewReader("/model claude-opus-4-7\r")
	var out bytes.Buffer
	script := Script{
		Matches: []Match{
			{InputPrefix: "/model ", Response: []byte("Model set to claude-opus-4-7\n")},
		},
	}
	if err := Run(in, &out, script); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "Model set to claude-opus-4-7") {
		t.Errorf("expected match response: %q", out.String())
	}
}

func TestRun_ExitSIGTERM_EmitsResumeHint(t *testing.T) {
	in := strings.NewReader("")
	var out bytes.Buffer
	if err := Run(in, &out, Script{Exit: ExitSIGTERM}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "Resume this session with:") {
		t.Errorf("expected resume hint: %q", out.String())
	}
	if !strings.Contains(out.String(), "claude --resume ") {
		t.Errorf("expected resume invocation: %q", out.String())
	}
}

func TestRun_ExitModelInvalid_OnFirstTurn(t *testing.T) {
	in := strings.NewReader("hi\rfollowup\r")
	var out bytes.Buffer
	if err := Run(in, &out, Script{Exit: ExitModelInvalid}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "is not available") {
		t.Errorf("expected model-invalid line on first turn: %q", got)
	}
	if !strings.Contains(got, "echo: followup") {
		t.Errorf("expected follow-up turn to proceed normally: %q", got)
	}
}

// Regression: real claude-code's TUI accepts only '\r' as Enter (plumb's
// probe-8: 3/3 wedge on '\n', 3/3 land on '\r' for /compact). The mock
// must mirror that or tests pass against a forgiving fake while the
// real binary wedges. A bare '\n' followed by EOF leaves the input
// uncommitted (no echo, no turn).
func TestRun_BareLF_DoesNotCommitTurn(t *testing.T) {
	in := strings.NewReader("hello\n")
	var out bytes.Buffer
	if err := Run(in, &out, Script{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(out.String(), "echo: hello") {
		t.Errorf("'\\n' must not commit a turn; got: %q", out.String())
	}
}

// '\r\n' is tolerated — the CR commits, the trailing LF is stripped from
// the next read so it doesn't show up as a spurious empty turn.
func TestRun_CRLF_CommitsOnceWithoutSpuriousTurn(t *testing.T) {
	in := strings.NewReader("hello\r\n")
	var out bytes.Buffer
	if err := Run(in, &out, Script{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "echo: hello") {
		t.Errorf("'\\r\\n' must commit the 'hello' turn: %q", got)
	}
	if n := strings.Count(got, "echo:"); n != 1 {
		t.Errorf("'\\r\\n' must commit exactly one turn, got %d: %q", n, got)
	}
}

func TestTitleHelpers_Shape(t *testing.T) {
	busy := BusyTitle("x")
	idle := IdleTitle("y")

	if !bytes.HasPrefix(busy, []byte("\x1b]0;")) || !bytes.HasSuffix(busy, []byte("\x07")) {
		t.Errorf("BusyTitle malformed OSC: %q", busy)
	}
	if !bytes.HasPrefix(idle, []byte("\x1b]0;")) || !bytes.HasSuffix(idle, []byte("\x07")) {
		t.Errorf("IdleTitle malformed OSC: %q", idle)
	}
	if !bytes.Contains(busy, []byte("⠂")) {
		t.Error("BusyTitle missing braille glyph")
	}
	if !bytes.Contains(idle, []byte("✳")) {
		t.Error("IdleTitle missing ✳ glyph")
	}
}
