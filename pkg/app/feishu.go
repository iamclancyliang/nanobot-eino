package app

import (
	"context"

	"github.com/wall/nanobot-eino/pkg/bus"
	"github.com/wall/nanobot-eino/pkg/channels"
	"github.com/wall/nanobot-eino/pkg/config"
)

func BuildFeishuConfig(cfg *config.Config) channels.FeishuConfig {
	return channels.FeishuConfig{
		AppID:             cfg.Channels.Feishu.AppID,
		AppSecret:         cfg.Channels.Feishu.AppSecret,
		VerificationToken: cfg.Channels.Feishu.VerificationToken,
		EncryptKey:        cfg.Channels.Feishu.EncryptKey,
		AllowFrom:         cfg.Channels.Feishu.AllowFrom,
		GroupPolicy:       cfg.Channels.Feishu.GroupPolicy,
	}
}

func StartFeishuChannel(
	ctx context.Context,
	feishuCfg channels.FeishuConfig,
	messageBus *bus.MessageBus,
	onNotConfigured func(),
) *channels.FeishuChannel {
	if feishuCfg.AppID == "" {
		if onNotConfigured != nil {
			onNotConfigured()
		}
		return nil
	}

	if err := channels.ValidateAllowFrom("feishu", feishuCfg.AllowFrom); err != nil {
		logApp.Warn("Feishu channel disabled", "error", err)
		return nil
	}

	feishuChannel := channels.NewFeishuChannel(feishuCfg, messageBus)
	if err := feishuChannel.Start(ctx); err != nil {
		logApp.Error("Failed to start Feishu channel", "error", err)
	}
	go feishuChannel.ListenOutbound(ctx)
	return feishuChannel
}
