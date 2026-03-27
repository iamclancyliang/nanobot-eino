package tools

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// SendMessagePayload holds the data for one outbound message delivery.
type SendMessagePayload struct {
	Channel string
	ChatID  string
	Content string
	Media   []string
}

// SendMessageFunc is a callback invoked by the message tool to deliver outbound messages.
// It replaces the direct *bus.MessageBus dependency, decoupling pkg/tools from pkg/bus.
// Callers (e.g. pkg/app) supply a closure that wraps the real bus publish call.
// A nil value disables the message tool.
type SendMessageFunc func(ctx context.Context, payload SendMessagePayload)

// MessageArgs is the JSON input schema for the message tool.
type MessageArgs struct {
	Content string   `json:"content" jsonschema:"description=The message content to send"`
	Channel string   `json:"channel,omitempty" jsonschema:"description=Optional: target channel (telegram, discord, etc.)"`
	ChatID  string   `json:"chat_id,omitempty" jsonschema:"description=Optional: target chat/user ID"`
	Media   []string `json:"media,omitempty" jsonschema:"description=Optional: list of file paths to attach (images, audio, documents)"`
}

// NewMessageTool creates a tool that delivers messages via the provided callback.
// defaultChannel and defaultChatID are used when the LLM omits those fields.
func NewMessageTool(onMessage SendMessageFunc, defaultChannel, defaultChatID string) tool.InvokableTool {
	t, _ := utils.InferTool("message", "Send a message to the user. Use this when you want to communicate something.",
		func(ctx context.Context, args *MessageArgs) (string, error) {
			channel := args.Channel
			if channel == "" {
				channel = defaultChannel
			}
			chatID := args.ChatID
			if chatID == "" {
				chatID = defaultChatID
			}

			if channel == "" || chatID == "" {
				return "Error: No target channel/chat specified", nil
			}

			onMessage(ctx, SendMessagePayload{
				Channel: channel,
				ChatID:  chatID,
				Content: args.Content,
				Media:   args.Media,
			})

			if tc := GetTurnContext(ctx); tc != nil {
				tc.SetMessageSent()
			}

			mediaInfo := ""
			if len(args.Media) > 0 {
				mediaInfo = fmt.Sprintf(" with %d attachments", len(args.Media))
			}
			return fmt.Sprintf("Message sent to %s:%s%s", channel, chatID, mediaInfo), nil
		})
	return t
}
