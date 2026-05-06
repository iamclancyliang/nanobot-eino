package tools

import (
	"context"
	"sync/atomic"
)

type turnCtxKey struct{}
type sessionIDKey struct{}
type progressInfoKey struct{}
type inputRoleKey struct{}

// TurnContext tracks per-turn mutable state across the agent loop.
// It is stored in context and shared between the agent and tools.
type TurnContext struct {
	messageSent atomic.Bool
}

// SetMessageSent records that the current turn has already produced a
// user-visible message.
func (tc *TurnContext) SetMessageSent() { tc.messageSent.Store(true) }

// WasMessageSent reports whether SetMessageSent has been called this turn.
func (tc *TurnContext) WasMessageSent() bool { return tc.messageSent.Load() }

// NewTurnContext attaches a fresh TurnContext to parent and returns both.
func NewTurnContext(parent context.Context) (context.Context, *TurnContext) {
	tc := &TurnContext{}
	return context.WithValue(parent, turnCtxKey{}, tc), tc
}

// GetTurnContext retrieves the TurnContext attached by NewTurnContext, or
// nil when absent.
func GetTurnContext(ctx context.Context) *TurnContext {
	tc, _ := ctx.Value(turnCtxKey{}).(*TurnContext)
	return tc
}

// ContextWithSessionID attaches the session id to ctx.
func ContextWithSessionID(ctx context.Context, sid string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, sid)
}

// SessionIDFromContext returns the session id stored on ctx, or "" when
// absent.
func SessionIDFromContext(ctx context.Context) string {
	sid, _ := ctx.Value(sessionIDKey{}).(string)
	return sid
}

// ProgressInfo carries channel routing info for progress callbacks.
type ProgressInfo struct {
	Channel string
	ChatID  string
}

// ContextWithProgressInfo attaches channel/chat routing for progress
// callbacks.
func ContextWithProgressInfo(ctx context.Context, channel, chatID string) context.Context {
	return context.WithValue(ctx, progressInfoKey{}, &ProgressInfo{Channel: channel, ChatID: chatID})
}

// GetProgressInfo returns the ProgressInfo attached to ctx, or nil.
func GetProgressInfo(ctx context.Context) *ProgressInfo {
	pi, _ := ctx.Value(progressInfoKey{}).(*ProgressInfo)
	return pi
}

// ContextWithInputRole tags the conversation role (e.g. "user", "system")
// of the input that triggered the current turn.
func ContextWithInputRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, inputRoleKey{}, role)
}

// InputRoleFromContext returns the input role attached to ctx, or "".
func InputRoleFromContext(ctx context.Context) string {
	role, _ := ctx.Value(inputRoleKey{}).(string)
	return role
}
