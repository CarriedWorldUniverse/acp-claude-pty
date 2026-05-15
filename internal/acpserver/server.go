// Package acpserver exposes a *pty.Driver over the Agent Client Protocol
// using github.com/coder/acp-go-sdk.
//
// The server wraps a single persistent claude REPL — per the lifecycle
// lockdown (NEX-83), one acp-claude-pty process holds one claude session
// globally, with restart available as an explicit caller-driven signal.
// NewSession therefore returns the same sessionID across calls; the
// underlying state lives in the driver.
package acpserver

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/CarriedWorldUniverse/acp-claude-pty/internal/pty"
	acp "github.com/coder/acp-go-sdk"
)

// fixedSessionID is the single session the driver represents.
//
// Multiple ACP sessions over one driver don't map cleanly to a single
// persistent REPL; clients that want isolation should restart the driver
// or run a second acp-claude-pty process.
const fixedSessionID acp.SessionId = "claude-pty-session"

// Server implements acp.Agent over a *pty.Driver.
type Server struct {
	drv  *pty.Driver
	conn *acp.AgentSideConnection

	mu      sync.Mutex
	started bool
}

// New returns a Server that drives drv. Callers wire stdin/stdout to it
// via Serve.
func New(drv *pty.Driver) *Server {
	return &Server{drv: drv}
}

// Serve runs the ACP agent loop until peerInput returns EOF or peerOutput
// signals disconnect. It is the typical entrypoint from main(): pass
// os.Stdin / os.Stdout. Serve blocks.
//
// Serve calls drv.Start if it has not already been started, and drv.Stop
// when the peer disconnects.
func (s *Server) Serve(ctx context.Context, peerInput io.Writer, peerOutput io.Reader) error {
	if err := s.drv.Start(ctx); err != nil {
		return fmt.Errorf("acpserver: driver start: %w", err)
	}
	defer func() { _ = s.drv.Stop(context.Background()) }()

	s.conn = acp.NewAgentSideConnection(s, peerInput, peerOutput)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.conn.Done():
		return nil
	}
}

// --- Agent surface ---

func (s *Server) Initialize(ctx context.Context, _ acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		ProtocolVersion: acp.ProtocolVersionNumber,
		AgentCapabilities: acp.AgentCapabilities{
			LoadSession: false,
		},
	}, nil
}

func (s *Server) Authenticate(ctx context.Context, _ acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (s *Server) NewSession(ctx context.Context, _ acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{SessionId: fixedSessionID}, nil
}

func (s *Server) Cancel(ctx context.Context, _ acp.CancelNotification) error {
	// Cancellation is wired by the SDK via context propagation into Prompt.
	// No additional driver-side action is required here; the in-flight
	// turnLoop will observe the context cancel and return ErrPromptTimeout.
	return nil
}

func (s *Server) SetSessionMode(ctx context.Context, _ acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}

func (s *Server) ListSessions(ctx context.Context, _ acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
}

func (s *Server) ResumeSession(ctx context.Context, _ acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, nil
}

func (s *Server) CloseSession(ctx context.Context, _ acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	return acp.CloseSessionResponse{}, nil
}

func (s *Server) SetSessionConfigOption(ctx context.Context, _ acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}

// Prompt maps an ACP prompt to one Driver.Send invocation. Text content
// blocks are concatenated with single spaces; other block kinds are not
// yet supported (TODO once plumb's tool-use turn capture lands and the
// parser-tier work covers resource/image/audio).
func (s *Server) Prompt(ctx context.Context, p acp.PromptRequest) (acp.PromptResponse, error) {
	input := textFromBlocks(p.Prompt)

	turn, err := s.drv.Send(ctx, input)
	if err != nil {
		return acp.PromptResponse{}, err
	}

	// Stream events as agent_message_chunks in wire-order.
	for ev := range turn.Events {
		s.dispatchEvent(ctx, p.SessionId, ev)
	}

	if turnErr := turn.Err(); turnErr != nil {
		return acp.PromptResponse{StopReason: stopReasonFor(turnErr)}, nil
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

// dispatchEvent maps a driver Event to an ACP session_update notification.
func (s *Server) dispatchEvent(ctx context.Context, sessionID acp.SessionId, ev pty.Event) {
	if s.conn == nil {
		return
	}
	switch e := ev.(type) {
	case pty.LineEvent:
		if e.Line == "" {
			return
		}
		_ = s.conn.SessionUpdate(ctx, acp.SessionNotification{
			SessionId: sessionID,
			Update:    acp.UpdateAgentMessageText(e.Line + "\n"),
		})
	case pty.CompactStart, pty.CompactEnd, pty.CompactSummaryAvailable, pty.Cleared,
		pty.ModelChanged, pty.EffortChanged, pty.SessionExiting:
		// Tagged effect events carry no ACP-standard mapping yet; surface
		// them as agent_message_chunks tagged with the type so callers
		// inspecting the stream don't lose ordering. A richer mapping
		// (e.g. as resource_links or custom Meta keys) is a follow-up
		// once plumb's slash-command output fixtures land.
		_ = s.conn.SessionUpdate(ctx, acp.SessionNotification{
			SessionId: sessionID,
			Update:    acp.UpdateAgentMessageText(formatEffect(e) + "\n"),
		})
	}
}

func formatEffect(ev pty.Event) string {
	switch e := ev.(type) {
	case pty.CompactStart:
		return "[compact-start]"
	case pty.CompactEnd:
		return "[compact-end]"
	case pty.CompactSummaryAvailable:
		return "[compact-summary-available] " + e.Path
	case pty.Cleared:
		return "[cleared]"
	case pty.ModelChanged:
		return "[model-changed] " + e.Model
	case pty.EffortChanged:
		return "[effort-changed] " + e.Level
	case pty.SessionExiting:
		return "[session-exiting] " + e.Reason
	default:
		return ""
	}
}

func textFromBlocks(blocks []acp.ContentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Text != nil {
			parts = append(parts, b.Text.Text)
		}
	}
	return strings.Join(parts, " ")
}

// stopReasonFor maps a driver error to an ACP StopReason.
func stopReasonFor(err error) acp.StopReason {
	de, ok := pty.AsDriverError(err)
	if !ok {
		return acp.StopReasonEndTurn
	}
	switch de.Kind {
	case pty.ErrPromptTimeout, pty.ErrHang:
		return acp.StopReasonCancelled
	case pty.ErrGracefulEOF, pty.ErrAbortedBySIGTERM:
		return acp.StopReasonEndTurn
	default:
		return acp.StopReasonCancelled
	}
}
