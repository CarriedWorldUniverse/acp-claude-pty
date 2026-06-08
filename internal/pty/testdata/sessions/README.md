# Recorded Parser Sessions

This directory stores byte-for-byte PTY replay fixtures for the prompt
detector/parser path. Each scenario has two files:

- `<scenario>.log`: raw PTY bytes, including OSC title-bar sequences.
- `<scenario>.events.json`: normalized expected parser output. Event
  timestamps are intentionally omitted.

The checked-in seed corpus is generated deterministically from
`internal/mockclaude` so CI can exercise the replay harness before real
Claude drift captures are available.

To refresh mock-derived seeds:

```sh
UPDATE_RECORDED_SESSIONS=1 go test ./internal/pty -run TestParser_Recorded
```

To refresh from a real capture, run `acp-claude-pty` with raw logging enabled
for the session under test, copy the captured PTY byte stream to
`internal/pty/testdata/sessions/<scenario>.log`, then write or update the
matching `<scenario>.events.json` sidecar with the normalized events expected
from that stream. Keep captures small and scenario-focused so failures point
to one detector/parser behavior.
