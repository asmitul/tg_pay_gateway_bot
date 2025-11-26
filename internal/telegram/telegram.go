// Package telegram hosts the Telegram client, routing, and handlers.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/sirupsen/logrus"

	"tg_pay_gateway_bot/internal/config"
	"tg_pay_gateway_bot/internal/logging"
)

type botRunner interface {
	Start(ctx context.Context)
}

var (
	defaultAllowedUpdates = bot.AllowedUpdates{
		"message",
		"edited_message",
		"callback_query",
		"my_chat_member",
		"chat_member",
	}

	createBot = func(token string, options ...bot.Option) (botRunner, error) {
		return bot.New(token, options...)
	}
)

// Client wraps the Telegram bot instance and logging dependencies.
type Client struct {
	bot    botRunner
	logger *logrus.Entry
}

// NewClient initializes the Telegram bot with long polling and default handlers.
func NewClient(cfg config.Config, logger *logrus.Entry) (*Client, error) {
	if strings.TrimSpace(cfg.TelegramToken) == "" {
		return nil, errors.New("telegram token is required")
	}
	if logger == nil {
		logger = logging.Logger()
	}

	tgBot, err := createBot(cfg.TelegramToken,
		bot.WithAllowedUpdates(defaultAllowedUpdates),
		bot.WithDefaultHandler(defaultHandler(logger)),
		bot.WithErrorsHandler(errorHandler(logger)),
	)
	if err != nil {
		return nil, fmt.Errorf("init telegram bot client: %w", err)
	}

	return &Client{
		bot:    tgBot,
		logger: logger,
	}, nil
}

// Start begins receiving updates via long polling until the context is canceled.
func (c *Client) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	c.logger.WithFields(logging.Fields{
		"event":           "telegram_listen",
		"allowed_updates": defaultAllowedUpdates,
	}).Info("starting telegram long polling")

	c.bot.Start(ctx)

	c.logger.WithField("event", "telegram_stopped").Info("telegram polling stopped")
}

type updateMeta struct {
	userID     int64
	chatID     int64
	text       string
	updateType string
}

func defaultHandler(logger *logrus.Entry) bot.HandlerFunc {
	if logger == nil {
		logger = logging.Logger()
	}

	return func(ctx context.Context, _ *bot.Bot, update *models.Update) {
		if update == nil {
			return
		}

		meta := extractUpdateMeta(update)

		fields := logging.Fields{
			"event":       "telegram_update",
			"update_type": meta.updateType,
		}

		if meta.text != "" {
			fields["text"] = meta.text
		}
		if meta.userID != 0 {
			fields["user_id"] = meta.userID
		}
		if meta.chatID != 0 {
			fields["chat_id"] = meta.chatID
		}

		logger.WithFields(fields).Info("telegram update received")
	}
}

func extractUpdateMeta(update *models.Update) updateMeta {
	switch {
	case update.Message != nil:
		return updateMeta{
			userID:     userID(update.Message.From),
			chatID:     chatID(&update.Message.Chat),
			text:       strings.TrimSpace(update.Message.Text),
			updateType: "message",
		}
	case update.EditedMessage != nil:
		return updateMeta{
			userID:     userID(update.EditedMessage.From),
			chatID:     chatID(&update.EditedMessage.Chat),
			text:       strings.TrimSpace(update.EditedMessage.Text),
			updateType: "edited_message",
		}
	case update.CallbackQuery != nil:
		return updateMeta{
			userID:     userID(&update.CallbackQuery.From),
			chatID:     messageChatID(update.CallbackQuery.Message),
			text:       strings.TrimSpace(update.CallbackQuery.Data),
			updateType: "callback_query",
		}
	case update.MyChatMember != nil:
		return updateMeta{
			userID:     userID(&update.MyChatMember.From),
			chatID:     chatID(&update.MyChatMember.Chat),
			updateType: "my_chat_member",
		}
	case update.ChatMember != nil:
		return updateMeta{
			userID:     userID(&update.ChatMember.From),
			chatID:     chatID(&update.ChatMember.Chat),
			updateType: "chat_member",
		}
	default:
		return updateMeta{updateType: "unknown"}
	}
}

func errorHandler(logger *logrus.Entry) bot.ErrorsHandler {
	if logger == nil {
		logger = logging.Logger()
	}

	return func(err error) {
		if err == nil {
			return
		}

		logger.WithField("event", "telegram_error").WithError(err).Error("telegram polling error")
	}
}

func userID(user *models.User) int64 {
	if user == nil {
		return 0
	}

	return user.ID
}

func chatID(chat *models.Chat) int64 {
	if chat == nil {
		return 0
	}

	return chat.ID
}

func messageChatID(msg models.MaybeInaccessibleMessage) int64 {
	switch msg.Type {
	case models.MaybeInaccessibleMessageTypeMessage:
		if msg.Message == nil {
			return 0
		}
		return chatID(&msg.Message.Chat)
	case models.MaybeInaccessibleMessageTypeInaccessibleMessage:
		if msg.InaccessibleMessage == nil {
			return 0
		}
		return chatID(&msg.InaccessibleMessage.Chat)
	default:
		return 0
	}
}
