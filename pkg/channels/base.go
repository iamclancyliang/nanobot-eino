package channels

import (
	"context"
	"fmt"
	"log/slog"
)

var logChannels = slog.With("module", "channels")

// Message is the channel-level representation of an incoming user message.
type Message struct {
	SessionID string
	Content   string
	Channel   string
}

// Handler processes a Message and returns the reply content.
type Handler func(ctx context.Context, msg Message) (string, error)

// Channel is the lifecycle interface implemented by every channel adapter
// (Feishu, web, CLI, ...).
type Channel interface {
	Start(ctx context.Context, handler Handler) error
	Stop(ctx context.Context) error
}

// IsSenderAllowed checks if senderID is permitted by the allowFrom list.
// This mirrors nanobot's BaseChannel.is_allowed():
//   - Empty list → deny all (with warning log)
//   - ["*"]      → allow everyone
//   - ["id1"]    → only allow exact matches
func IsSenderAllowed(channelName, senderID string, allowFrom []string) bool {
	if len(allowFrom) == 0 {
		logChannels.Warn("allowFrom is empty, all access denied", "channel", channelName)
		return false
	}
	for _, s := range allowFrom {
		if s == "*" {
			return true
		}
		if s == senderID {
			return true
		}
	}
	logChannels.Warn("Access denied for sender", "channel", channelName, "sender", senderID)
	return false
}

// ValidateAllowFrom checks that allowFrom is not empty for a configured channel.
// This mirrors nanobot's ChannelManager._validate_allow_from() which exits on
// empty allowFrom to prevent silent denial of all messages.
func ValidateAllowFrom(channelName string, allowFrom []string) error {
	if len(allowFrom) == 0 {
		return fmt.Errorf(
			"%q has empty allowFrom (denies all). "+
				"Set [\"*\"] to allow everyone, or add specific user IDs",
			channelName,
		)
	}
	return nil
}
