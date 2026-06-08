package pty

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/CarriedWorldUniverse/acp-claude-pty/internal/mockclaude"
)

type recordedSidecar struct {
	PromptReturned bool            `json:"prompt_returned"`
	Events         []recordedEvent `json:"events"`
}

type recordedEvent struct {
	Type   string `json:"type"`
	Line   string `json:"line,omitempty"`
	Model  string `json:"model,omitempty"`
	Level  string `json:"level,omitempty"`
	Reason string `json:"reason,omitempty"`
	Path   string `json:"path,omitempty"`
}

func TestParser_Recorded(t *testing.T) {
	dir := filepath.Join("testdata", "sessions")
	if os.Getenv("UPDATE_RECORDED_SESSIONS") == "1" {
		updateRecordedSessions(t, dir)
	}

	logs, err := filepath.Glob(filepath.Join(dir, "*.log"))
	if err != nil {
		t.Fatalf("glob recorded sessions: %v", err)
	}
	if len(logs) == 0 {
		t.Fatalf("no recorded session logs found in %s", dir)
	}
	sort.Strings(logs)

	for _, logPath := range logs {
		name := strings.TrimSuffix(filepath.Base(logPath), ".log")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("read log: %v", err)
			}
			want := readRecordedSidecar(t, strings.TrimSuffix(logPath, ".log")+".events.json")

			for _, chunkSize := range []int{1, 2, 3, 5, 8, 13, 64, len(raw) + 1} {
				t.Run(fmt.Sprintf("chunk_%d", chunkSize), func(t *testing.T) {
					got := replayRecordedBytes(raw, chunkSize)
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("replay mismatch\nchunk size: %d\ngot:  %+v\nwant: %+v", chunkSize, got, want)
					}
				})
			}
		})
	}
}

func updateRecordedSessions(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create recorded session dir: %v", err)
	}
	for name, raw := range recordedSeeds(t) {
		logPath := filepath.Join(dir, name+".log")
		if err := os.WriteFile(logPath, raw, 0o644); err != nil {
			t.Fatalf("write %s: %v", logPath, err)
		}
		sidecar := replayRecordedBytes(raw, 7)
		encoded, err := json.MarshalIndent(sidecar, "", "  ")
		if err != nil {
			t.Fatalf("marshal sidecar for %s: %v", name, err)
		}
		encoded = append(encoded, '\n')
		sidecarPath := filepath.Join(dir, name+".events.json")
		if err := os.WriteFile(sidecarPath, encoded, 0o644); err != nil {
			t.Fatalf("write %s: %v", sidecarPath, err)
		}
	}
}

func recordedSeeds(t *testing.T) map[string][]byte {
	t.Helper()
	return map[string][]byte{
		"cold_start":             bytes.Join([][]byte{mockclaude.IdleTitle("welcome"), []byte("Welcome to Claude Code\n")}, nil),
		"simple_text_turn":       runMockTurn(t, []byte("hello\r"), []byte("Hello from Claude.\n")),
		"tool_use":               runMockTurn(t, []byte("use tool\r"), []byte("I will inspect the file.\n\ntool_use: read_file\ninput: README.md\n")),
		"tool_result":            runMockTurn(t, []byte("tool result\r"), []byte("tool_result: read_file\nstatus: success\nlines: 3\n")),
		"multi_line_code_fenced": runMockTurn(t, []byte("code\r"), []byte("Here is a snippet:\n\n```go\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```\n")),
	}
}

func runMockTurn(t *testing.T, input []byte, response []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	err := mockclaude.Run(bytes.NewReader(input), &out, mockclaude.Script{
		Init:    mockclaude.IdleTitle("welcome"),
		Default: response,
		Cook:    0,
	})
	if err != nil {
		t.Fatalf("run mock claude: %v", err)
	}
	return out.Bytes()
}

func readRecordedSidecar(t *testing.T, path string) recordedSidecar {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	var sidecar recordedSidecar
	if err := json.Unmarshal(raw, &sidecar); err != nil {
		t.Fatalf("unmarshal sidecar: %v", err)
	}
	return sidecar
}

func replayRecordedBytes(raw []byte, chunkSize int) recordedSidecar {
	detector := NewTitlePromptDetector()
	out := make(chan Event, 64)
	effects := newSlashEffectParser(out)
	var lineBuf bytes.Buffer
	var ansi ansiStreamStripper
	sidecar := recordedSidecar{}
	at := time.Unix(0, 0)

	for start := 0; start < len(raw); start += chunkSize {
		end := start + chunkSize
		if end > len(raw) {
			end = len(raw)
		}
		chunk := raw[start:end]
		done := detector.Feed(chunk)
		effects.Feed(at, chunk)
		emitRecordedLines(out, at, &lineBuf, &ansi, chunk)
		if done {
			sidecar.PromptReturned = true
			flushRecordedPartialLine(out, at, &lineBuf)
		}
		drainRecordedEvents(out, &sidecar.Events)
		if done {
			break
		}
	}
	drainRecordedEvents(out, &sidecar.Events)
	return sidecar
}

func emitRecordedLines(out chan<- Event, at time.Time, lineBuf *bytes.Buffer, ansi *ansiStreamStripper, chunk []byte) {
	stripped := ansi.Strip(chunk)
	for _, b := range stripped {
		if b == '\n' {
			out <- NewLineEvent(at, trimCR(lineBuf.String()))
			lineBuf.Reset()
			continue
		}
		lineBuf.WriteByte(b)
	}
}

func flushRecordedPartialLine(out chan<- Event, at time.Time, lineBuf *bytes.Buffer) {
	if lineBuf.Len() == 0 {
		return
	}
	out <- NewLineEvent(at, trimCR(lineBuf.String()))
	lineBuf.Reset()
}

func drainRecordedEvents(in <-chan Event, out *[]recordedEvent) {
	for {
		select {
		case ev := <-in:
			*out = append(*out, normalizeRecordedEvent(ev))
		default:
			return
		}
	}
}

func normalizeRecordedEvent(ev Event) recordedEvent {
	switch e := ev.(type) {
	case LineEvent:
		return recordedEvent{Type: "line", Line: e.Line}
	case CompactStart:
		return recordedEvent{Type: "compact_start"}
	case CompactEnd:
		return recordedEvent{Type: "compact_end"}
	case CompactSummaryAvailable:
		return recordedEvent{Type: "compact_summary_available", Path: e.Path}
	case Cleared:
		return recordedEvent{Type: "cleared"}
	case ModelChanged:
		return recordedEvent{Type: "model_changed", Model: e.Model}
	case EffortChanged:
		return recordedEvent{Type: "effort_changed", Level: e.Level}
	case SessionExiting:
		return recordedEvent{Type: "session_exiting", Reason: e.Reason}
	default:
		panic(fmt.Sprintf("unhandled event type %T", ev))
	}
}

func TestParser_RecordedHarnessMatchesTurnLoop(t *testing.T) {
	raw := runMockTurn(t, []byte("hello\r"), []byte("line one\nline two"))
	chunks := make(chan readChunk, len(raw))
	for start := 0; start < len(raw); start += 5 {
		end := start + 5
		if end > len(raw) {
			end = len(raw)
		}
		chunks <- readChunk{at: time.Unix(0, 0), bytes: raw[start:end]}
	}
	close(chunks)

	out := make(chan Event, 16)
	loop := &turnLoop{
		chunks:        chunks,
		detector:      NewTitlePromptDetector(),
		effects:       newSlashEffectParser(out),
		out:           out,
		promptTimeout: time.Second,
		hangTimeout:   time.Second,
	}
	if err := loop.runTurn(context.Background()); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	close(out)

	var loopEvents []recordedEvent
	for ev := range out {
		loopEvents = append(loopEvents, normalizeRecordedEvent(ev))
	}
	replayed := replayRecordedBytes(raw, 5)
	if !reflect.DeepEqual(loopEvents, replayed.Events) {
		t.Fatalf("harness diverged from turnLoop\ngot:  %+v\nwant: %+v", replayed.Events, loopEvents)
	}
}
