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

	sendMessage = func(ctx context.Context, b *bot.Bot, params *bot.SendMessageParams) (*models.Message, error) {
		return b.SendMessage(ctx, params)
	}
)

// UserRegistrar ensures users are persisted and tracked when updates arrive.
type UserRegistrar interface {
	EnsureUser(ctx context.Context, userID int64) (bool, error)
}

// GroupRegistrar ensures groups are persisted when the bot encounters them.
type GroupRegistrar interface {
	EnsureGroup(ctx context.Context, chatID int64, title string) (bool, error)
}

type clientOptions struct {
	userRegistrar  UserRegistrar
	groupRegistrar GroupRegistrar
}

// ClientOption configures optional Telegram client dependencies.
type ClientOption func(*clientOptions)

// WithUserRegistrar wires a user registration hook that runs on every update.
func WithUserRegistrar(registrar UserRegistrar) ClientOption {
	return func(opts *clientOptions) {
		opts.userRegistrar = registrar
	}
}

// WithGroupRegistrar wires a group registration hook that runs on group updates.
func WithGroupRegistrar(registrar GroupRegistrar) ClientOption {
	return func(opts *clientOptions) {
		opts.groupRegistrar = registrar
	}
}

// Client wraps the Telegram bot instance and logging dependencies.
type Client struct {
	bot    botRunner
	logger *logrus.Entry
}

// NewClient initializes the Telegram bot with long polling and default handlers.
func NewClient(cfg config.Config, logger *logrus.Entry, opts ...ClientOption) (*Client, error) {
	if strings.TrimSpace(cfg.TelegramToken) == "" {
		return nil, errors.New("telegram token is required")
	}
	if logger == nil {
		logger = logging.Logger()
	}

	clientOpts := clientOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&clientOpts)
		}
	}

	tgBot, err := createBot(cfg.TelegramToken,
		bot.WithAllowedUpdates(defaultAllowedUpdates),
		bot.WithDefaultHandler(defaultHandler(logger, clientOpts.userRegistrar, clientOpts.groupRegistrar, cfg.BotOwnerID)),
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
	chatType   string
	chatTitle  string
}

type registeredHandler struct {
	name    string
	handler bot.HandlerFunc
}

type messageRouter struct {
	logger          *logrus.Entry
	commandHandlers map[string]registeredHandler
	unknownHandler  registeredHandler
	genericHandler  registeredHandler
}

func newMessageRouter(logger *logrus.Entry, botOwnerID int64) *messageRouter {
	return &messageRouter{
		logger: logger,
		commandHandlers: map[string]registeredHandler{
			"start": {
				name:    "command_start",
				handler: startCommandHandler(logger, botOwnerID),
			},
		},
		unknownHandler: registeredHandler{
			name:    "command_unknown",
			handler: commandLoggerHandler(logger, "command_unknown"),
		},
		genericHandler: registeredHandler{
			name:    "generic_message",
			handler: genericLoggerHandler(logger),
		},
	}
}

func (r *messageRouter) route(ctx context.Context, b *bot.Bot, update *models.Update, meta updateMeta) {
	msg := primaryMessage(update)
	if msg == nil {
		return
	}

	normalizedChatType := normalizeChatType(meta.chatType)

	if isCommand(meta.text) {
		cmd := commandName(meta.text)
		target, ok := r.commandHandlers[cmd]
		if !ok {
			target = r.unknownHandler
		}

		r.logRoute(meta, normalizedChatType, target.name, "command", cmd)
		target.handler(ctx, b, update)
		return
	}

	r.logRoute(meta, normalizedChatType, r.genericHandler.name, "message", "")
	r.genericHandler.handler(ctx, b, update)
}

func (r *messageRouter) logRoute(meta updateMeta, chatType, handlerName, route, command string) {
	fields := logging.Fields{
		"event":     "telegram_route",
		"handler":   handlerName,
		"route":     route,
		"chat_type": chatType,
	}

	if command != "" {
		fields["command"] = command
	}
	if meta.userID != 0 {
		fields["user_id"] = meta.userID
	}
	if meta.chatID != 0 {
		fields["chat_id"] = meta.chatID
	}

	r.logger.WithFields(fields).Info("routed update")
}

func defaultHandler(logger *logrus.Entry, userRegistrar UserRegistrar, groupRegistrar GroupRegistrar, botOwnerID int64) bot.HandlerFunc {
	if logger == nil {
		logger = logging.Logger()
	}

	router := newMessageRouter(logger, botOwnerID)

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update == nil {
			return
		}

		if ctx == nil {
			ctx = context.Background()
		}

		meta := extractUpdateMeta(update)

		normalizedChatType := normalizeChatType(meta.chatType)

		if userRegistrar != nil && meta.userID != 0 {
			if _, err := userRegistrar.EnsureUser(ctx, meta.userID); err != nil {
				logger.WithFields(logging.Fields{
					"event":   "user_registration_failed",
					"user_id": meta.userID,
					"chat_id": meta.chatID,
				}).WithError(err).Error("failed to ensure user registration")
			}
		}

		if groupRegistrar != nil && meta.chatID != 0 && normalizedChatType == "group" {
			if _, err := groupRegistrar.EnsureGroup(ctx, meta.chatID, meta.chatTitle); err != nil {
				logger.WithFields(logging.Fields{
					"event":      "group_registration_failed",
					"chat_id":    meta.chatID,
					"chat_title": meta.chatTitle,
				}).WithError(err).Error("failed to ensure group registration")
			}
		}

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
		if meta.chatType != "" {
			fields["chat_type"] = normalizedChatType
		}

		logger.WithFields(fields).Info("telegram update received")

		router.route(ctx, b, update, meta)
	}
}

func extractUpdateMeta(update *models.Update) updateMeta {
	switch {
	case update.Message != nil:
		return updateMeta{
			userID:     userID(update.Message.From),
			chatID:     chatID(&update.Message.Chat),
			text:       strings.TrimSpace(update.Message.Text),
			chatTitle:  chatTitle(&update.Message.Chat),
			chatType:   string(update.Message.Chat.Type),
			updateType: "message",
		}
	case update.EditedMessage != nil:
		return updateMeta{
			userID:     userID(update.EditedMessage.From),
			chatID:     chatID(&update.EditedMessage.Chat),
			text:       strings.TrimSpace(update.EditedMessage.Text),
			chatTitle:  chatTitle(&update.EditedMessage.Chat),
			chatType:   string(update.EditedMessage.Chat.Type),
			updateType: "edited_message",
		}
	case update.CallbackQuery != nil:
		return updateMeta{
			userID:     userID(&update.CallbackQuery.From),
			chatID:     messageChatID(update.CallbackQuery.Message),
			text:       strings.TrimSpace(update.CallbackQuery.Data),
			chatTitle:  messageChatTitle(update.CallbackQuery.Message),
			chatType:   messageChatType(update.CallbackQuery.Message),
			updateType: "callback_query",
		}
	case update.MyChatMember != nil:
		return updateMeta{
			userID:     userID(&update.MyChatMember.From),
			chatID:     chatID(&update.MyChatMember.Chat),
			chatTitle:  chatTitle(&update.MyChatMember.Chat),
			chatType:   string(update.MyChatMember.Chat.Type),
			updateType: "my_chat_member",
		}
	case update.ChatMember != nil:
		return updateMeta{
			userID:     userID(&update.ChatMember.From),
			chatID:     chatID(&update.ChatMember.Chat),
			chatTitle:  chatTitle(&update.ChatMember.Chat),
			chatType:   string(update.ChatMember.Chat.Type),
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

func chatTitle(chat *models.Chat) string {
	if chat == nil {
		return ""
	}

	return strings.TrimSpace(chat.Title)
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

func messageChatType(msg models.MaybeInaccessibleMessage) string {
	switch msg.Type {
	case models.MaybeInaccessibleMessageTypeMessage:
		if msg.Message == nil {
			return ""
		}
		return string(msg.Message.Chat.Type)
	case models.MaybeInaccessibleMessageTypeInaccessibleMessage:
		if msg.InaccessibleMessage == nil {
			return ""
		}
		return string(msg.InaccessibleMessage.Chat.Type)
	default:
		return ""
	}
}

func messageChatTitle(msg models.MaybeInaccessibleMessage) string {
	switch msg.Type {
	case models.MaybeInaccessibleMessageTypeMessage:
		if msg.Message == nil {
			return ""
		}
		return chatTitle(&msg.Message.Chat)
	case models.MaybeInaccessibleMessageTypeInaccessibleMessage:
		if msg.InaccessibleMessage == nil {
			return ""
		}
		return chatTitle(&msg.InaccessibleMessage.Chat)
	default:
		return ""
	}
}

func normalizeChatType(chatType string) string {
	switch chatType {
	case string(models.ChatTypePrivate):
		return "private"
	case string(models.ChatTypeGroup), string(models.ChatTypeSupergroup):
		return "group"
	case "":
		return "unknown"
	default:
		return chatType
	}
}

func isCommand(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "/")
}

func commandName(text string) string {
	clean := strings.TrimSpace(strings.TrimPrefix(text, "/"))
	if clean == "" {
		return ""
	}

	if idx := strings.Index(clean, " "); idx >= 0 {
		clean = clean[:idx]
	}
	if idx := strings.Index(clean, "@"); idx >= 0 {
		clean = clean[:idx]
	}

	return strings.ToLower(clean)
}

func primaryMessage(update *models.Update) *models.Message {
	switch {
	case update == nil:
		return nil
	case update.Message != nil:
		return update.Message
	case update.EditedMessage != nil:
		return update.EditedMessage
	default:
		return nil
	}
}

func logCommandHandled(logger *logrus.Entry, handlerName string, meta updateMeta) {
	fields := logging.Fields{
		"event":     "command_handler",
		"handler":   handlerName,
		"chat_type": normalizeChatType(meta.chatType),
	}

	if meta.userID != 0 {
		fields["user_id"] = meta.userID
	}
	if meta.chatID != 0 {
		fields["chat_id"] = meta.chatID
	}
	if meta.text != "" {
		fields["text"] = meta.text
	}

	logger.WithFields(fields).Info("handled command")
}

func startCommandHandler(logger *logrus.Entry, botOwnerID int64) bot.HandlerFunc {
	if logger == nil {
		logger = logging.Logger()
	}

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if ctx == nil || update == nil {
			return
		}

		meta := extractUpdateMeta(update)
		logCommandHandled(logger, "command_start", meta)

		chatType := normalizeChatType(meta.chatType)

		if chatType != "private" {
			logger.WithFields(logging.Fields{
				"event":     "command_start_ignored",
				"chat_type": chatType,
				"user_id":   meta.userID,
				"chat_id":   meta.chatID,
			}).Info("ignored /start outside private chat")
			return
		}

		if meta.chatID == 0 {
			logger.WithFields(logging.Fields{
				"event":   "command_start_send_failed",
				"user_id": meta.userID,
				"chat_id": meta.chatID,
			}).Error("cannot send start response without chat_id")
			return
		}

		messageText := startMessage(meta.userID, botOwnerID)

		if b == nil {
			logger.WithFields(logging.Fields{
				"event":   "command_start_send_failed",
				"user_id": meta.userID,
				"chat_id": meta.chatID,
			}).Error("cannot send start response without telegram client")
			return
		}

		if _, err := sendMessage(ctx, b, &bot.SendMessageParams{
			ChatID: meta.chatID,
			Text:   messageText,
		}); err != nil {
			logger.WithFields(logging.Fields{
				"event":   "command_start_send_failed",
				"user_id": meta.userID,
				"chat_id": meta.chatID,
			}).WithError(err).Error("failed to send start response")
			return
		}

		logger.WithFields(logging.Fields{
			"event":   "command_start_sent",
			"user_id": meta.userID,
			"chat_id": meta.chatID,
		}).Info("sent start response")
	}
}

func startMessage(userID, botOwnerID int64) string {
	role := "user"
	if userID != 0 && userID == botOwnerID {
		role = "owner"
	}

	lines := []string{
		"Welcome to the Telegram payment bot (base build).",
		"User registration and chat tracking are enabled; payment flows and dashboards will arrive later.",
		fmt.Sprintf("Your role: %s", role),
		"Status: registered",
	}

	return strings.Join(lines, "\n")
}

func commandLoggerHandler(logger *logrus.Entry, handlerName string) bot.HandlerFunc {
	if logger == nil {
		logger = logging.Logger()
	}

	return func(ctx context.Context, _ *bot.Bot, update *models.Update) {
		if ctx == nil || update == nil {
			return
		}

		meta := extractUpdateMeta(update)

		logCommandHandled(logger, handlerName, meta)
	}
}

func genericLoggerHandler(logger *logrus.Entry) bot.HandlerFunc {
	if logger == nil {
		logger = logging.Logger()
	}

	return func(ctx context.Context, _ *bot.Bot, update *models.Update) {
		if ctx == nil || update == nil {
			return
		}

		meta := extractUpdateMeta(update)

		fields := logging.Fields{
			"event":     "generic_handler",
			"handler":   "generic_message",
			"chat_type": normalizeChatType(meta.chatType),
		}

		if meta.userID != 0 {
			fields["user_id"] = meta.userID
		}
		if meta.chatID != 0 {
			fields["chat_id"] = meta.chatID
		}
		if meta.text != "" {
			fields["text"] = meta.text
		}

		logger.WithFields(fields).Info("handled generic message")
	}
}
