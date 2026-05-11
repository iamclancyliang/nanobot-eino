package app

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	langfuse "github.com/cloudwego/eino-ext/callbacks/langfuse"
	"github.com/cloudwego/eino/schema"
	"github.com/wall/nanobot-eino/pkg/apperr"
	"github.com/wall/nanobot-eino/pkg/bus"
	"github.com/wall/nanobot-eino/pkg/tools"
)

const defaultAgentErrorReply = apperr.DefaultPublicMessage

var logApp = slog.With("module", "app")

type ChatStreamer interface {
	ChatStream(ctx context.Context, sessionID, input string) (*schema.StreamReader[*schema.Message], error)
}

// sessionQueue holds the per-session message channel for a RunInboundLoop worker.
type sessionQueue struct {
	ch chan *bus.InboundMessage
}

// RunInboundLoop processes messages from the bus using per-session worker goroutines.
// Each unique session key gets its own worker, so messages for the same session are
// processed sequentially while messages for different sessions run in parallel.
// wg is incremented once per new session worker; callers may call wg.Wait() to block
// until all in-flight processing is complete.
func RunInboundLoop(
	ctx context.Context,
	messageBus *bus.MessageBus,
	bot ChatStreamer,
	wg *sync.WaitGroup,
) {
	var sessions sync.Map // sessionKey -> *sessionQueue

	for msg := range messageBus.ConsumeInbound(ctx) {
		key := msg.SessionKey()
		sq, loaded := sessions.LoadOrStore(key, &sessionQueue{
			ch: make(chan *bus.InboundMessage, 32),
		})
		q := sq.(*sessionQueue)

		if !loaded {
			// New session: start a dedicated worker goroutine.
			wg.Add(1)
			go func(q *sessionQueue) {
				defer wg.Done()
				for m := range q.ch {
					processMessage(ctx, messageBus, bot, m)
				}
			}(q)
		}

		select {
		case q.ch <- msg:
		case <-ctx.Done():
			return
		}
	}

	// Close all session channels so workers drain remaining messages and exit.
	sessions.Range(func(_, v any) bool {
		close(v.(*sessionQueue).ch)
		return true
	})
}

// processMessage handles a single inbound message: routes it, calls the bot,
// and publishes the response to the outbound channel.
func processMessage(
	ctx context.Context,
	messageBus *bus.MessageBus,
	bot ChatStreamer,
	m *bus.InboundMessage,
) {
	sessionID := m.SessionKey()
	targetChannel := m.Channel
	targetChatID := m.ChatID
	if m.Channel == "system" {
		targetChannel, targetChatID = DecodeSystemRoute(m.ChatID)
	}
	logApp.Info("Processing message",
		"content_preview", previewContent(m.Content),
		"channel", targetChannel,
		"chat_id", targetChatID,
	)

	ctx = langfuse.SetTrace(ctx,
		langfuse.WithSessionID(sessionID),
		langfuse.WithUserID(m.SenderID),
		langfuse.WithName("chat"),
		langfuse.WithMetadata(map[string]string{
			"channel": targetChannel,
			"chat_id": targetChatID,
		}),
	)

	turnCtx, turnFlag := tools.NewTurnContext(ctx)
	turnCtx = tools.ContextWithProgressInfo(turnCtx, targetChannel, targetChatID)
	if m.Channel == "system" && m.SenderID == "subagent" {
		turnCtx = tools.ContextWithInputRole(turnCtx, "assistant")
	}

	reader, err := bot.ChatStream(turnCtx, sessionID, m.Content)
	err = apperr.Normalize("agent.ChatStream", err)
	if err != nil && apperr.Retryable(err) {
		logApp.Warn("Agent transient error, retrying in 2s", "error", err)
		time.Sleep(2 * time.Second)
		reader, err = bot.ChatStream(turnCtx, sessionID, m.Content)
		err = apperr.Normalize("agent.ChatStream", err)
	}
	if err != nil {
		logApp.Error("Agent error", "error", err, "session", sessionID)
		messageBus.PublishOutbound(ctx, &bus.OutboundMessage{
			Channel:  targetChannel,
			ChatID:   targetChatID,
			Content:  apperr.PublicMessage(err),
			Metadata: m.Metadata,
		})
		return
	}
	defer reader.Close()

	var fullResponse string
	streamFailed := false
	var streamErr error
	for {
		chunk, recvErr := reader.Recv()
		if recvErr != nil {
			if recvErr != io.EOF {
				streamFailed = true
				streamErr = apperr.Normalize("agent.StreamRecv", recvErr)
				logApp.Error("Stream recv error", "error", recvErr, "session", sessionID)
			}
			break
		}
		fullResponse += chunk.Content
	}

	if streamFailed && fullResponse == "" && !turnFlag.WasMessageSent() {
		messageBus.PublishOutbound(ctx, &bus.OutboundMessage{
			Channel:  targetChannel,
			ChatID:   targetChatID,
			Content:  apperr.PublicMessage(streamErr),
			Metadata: m.Metadata,
		})
		return
	}
	if fullResponse == "" || turnFlag.WasMessageSent() {
		return
	}
	replyTo := bus.ExtractReplyTo(m.Metadata)
	messageBus.PublishOutbound(ctx, &bus.OutboundMessage{
		Channel:  targetChannel,
		ChatID:   targetChatID,
		Content:  fullResponse,
		ReplyTo:  replyTo,
		Metadata: m.Metadata,
	})
}

func previewContent(content string) string {
	runes := []rune(content)
	if len(runes) > 80 {
		return string(runes[:80])
	}
	return content
}

func DecodeSystemRoute(chatID string) (channel string, targetChatID string) {
	if strings.Contains(chatID, ":") {
		parts := strings.SplitN(chatID, ":", 2)
		return parts[0], parts[1]
	}
	return "cli", chatID
}
