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

func (tc *TurnContext) SetMessageSent()      { tc.messageSent.Store(true) }
func (tc *TurnContext) WasMessageSent() bool { return tc.messageSent.Load() }

func NewTurnContext(parent context.Context) (context.Context, *TurnContext) {
	tc := &TurnContext{}
	return context.WithValue(parent, turnCtxKey{}, tc), tc
}

func GetTurnContext(ctx context.Context) *TurnContext {
	tc, _ := ctx.Value(turnCtxKey{}).(*TurnContext)
	return tc
}

func ContextWithSessionID(ctx context.Context, sid string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, sid)
}

func SessionIDFromContext(ctx context.Context) string {
	sid, _ := ctx.Value(sessionIDKey{}).(string)
	return sid
}

// ProgressInfo carries channel routing info for progress callbacks.
type ProgressInfo struct {
	Channel string
	ChatID  string
}

func ContextWithProgressInfo(ctx context.Context, channel, chatID string) context.Context {
	return context.WithValue(ctx, progressInfoKey{}, &ProgressInfo{Channel: channel, ChatID: chatID})
}

func GetProgressInfo(ctx context.Context) *ProgressInfo {
	pi, _ := ctx.Value(progressInfoKey{}).(*ProgressInfo)
	return pi
}

func ContextWithInputRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, inputRoleKey{}, role)
}

func InputRoleFromContext(ctx context.Context) string {
	role, _ := ctx.Value(inputRoleKey{}).(string)
	return role
}
