// Package telegram hosts the Telegram client, routing, and handlers.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/sirupsen/logrus"

	"tg_pay_gateway_bot/internal/config"
	"tg_pay_gateway_bot/internal/domain"
	"tg_pay_gateway_bot/internal/logging"
)

type botRunner interface {
	Start(ctx context.Context)
}

const (
	pingMongoTimeout    = 2 * time.Second
	statusLookupTimeout = 2 * time.Second
	statusCountTimeout  = 2 * time.Second
)

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

// MongoChecker allows health checks against MongoDB.
type MongoChecker interface {
	Ping(ctx context.Context) error
}

// UserFetcher retrieves users for permission checks.
type UserFetcher interface {
	GetByID(ctx context.Context, userID int64) (domain.User, error)
}

// StatsProvider exposes simple collection counts for diagnostics.
type StatsProvider interface {
	CountUsers(ctx context.Context) (int64, error)
	CountGroups(ctx context.Context) (int64, error)
}

type commandDiagnostics struct {
	appEnv        string
	processStart  time.Time
	mongoChecker  MongoChecker
	userFetcher   UserFetcher
	statsProvider StatsProvider
}

type clientOptions struct {
	userRegistrar  UserRegistrar
	groupRegistrar GroupRegistrar
	mongoChecker   MongoChecker
	processStart   time.Time
	userFetcher    UserFetcher
	statsProvider  StatsProvider
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

// WithMongoChecker supplies a Mongo health checker for diagnostics.
func WithMongoChecker(checker MongoChecker) ClientOption {
	return func(opts *clientOptions) {
		opts.mongoChecker = checker
	}
}

// WithProcessStart injects the process start time for uptime calculations.
func WithProcessStart(start time.Time) ClientOption {
	return func(opts *clientOptions) {
		opts.processStart = start
	}
}

// WithUserFetcher supplies a user reader for permission checks.
func WithUserFetcher(fetcher UserFetcher) ClientOption {
	return func(opts *clientOptions) {
		opts.userFetcher = fetcher
	}
}

// WithStatsProvider supplies a diagnostics provider for live collection counts.
func WithStatsProvider(provider StatsProvider) ClientOption {
	return func(opts *clientOptions) {
		opts.statsProvider = provider
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

	diag := normalizeDiagnostics(commandDiagnostics{
		appEnv:        cfg.AppEnv,
		processStart:  clientOpts.processStart,
		mongoChecker:  clientOpts.mongoChecker,
		userFetcher:   clientOpts.userFetcher,
		statsProvider: clientOpts.statsProvider,
	})

	tgBot, err := createBot(cfg.TelegramToken,
		bot.WithAllowedUpdates(defaultAllowedUpdates),
		bot.WithDefaultHandler(defaultHandler(logger, clientOpts.userRegistrar, clientOpts.groupRegistrar, cfg.BotOwnerID, diag)),
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

func normalizeDiagnostics(diag commandDiagnostics) commandDiagnostics {
	if strings.TrimSpace(diag.appEnv) == "" {
		diag.appEnv = config.DefaultAppEnv
	}
	if diag.processStart.IsZero() {
		diag.processStart = time.Now()
	}

	return diag
}

func newMessageRouter(logger *logrus.Entry, botOwnerID int64, diag commandDiagnostics) *messageRouter {
	return &messageRouter{
		logger: logger,
		commandHandlers: map[string]registeredHandler{
			"start": {
				name:    "command_start",
				handler: startCommandHandler(logger, botOwnerID),
			},
			"ping": {
				name:    "command_ping",
				handler: pingCommandHandler(logger, diag),
			},
			"status": {
				name:    "command_status",
				handler: statusCommandHandler(logger, botOwnerID, diag),
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

func defaultHandler(logger *logrus.Entry, userRegistrar UserRegistrar, groupRegistrar GroupRegistrar, botOwnerID int64, diag commandDiagnostics) bot.HandlerFunc {
	if logger == nil {
		logger = logging.Logger()
	}

	diag = normalizeDiagnostics(diag)
	router := newMessageRouter(logger, botOwnerID, diag)

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

func pingCommandHandler(logger *logrus.Entry, diag commandDiagnostics) bot.HandlerFunc {
	if logger == nil {
		logger = logging.Logger()
	}
	diag = normalizeDiagnostics(diag)

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if ctx == nil || update == nil {
			return
		}

		meta := extractUpdateMeta(update)
		logCommandHandled(logger, "command_ping", meta)

		if meta.chatID == 0 {
			logger.WithFields(logging.Fields{
				"event":     "command_ping_send_failed",
				"user_id":   meta.userID,
				"chat_id":   meta.chatID,
				"chat_type": normalizeChatType(meta.chatType),
			}).Error("cannot send ping response without chat_id")
			return
		}

		mongoStatus := "error"
		if diag.mongoChecker != nil {
			mongoCtx, cancel := context.WithTimeout(ctx, pingMongoTimeout)
			defer cancel()

			if err := diag.mongoChecker.Ping(mongoCtx); err != nil {
				logger.WithFields(logging.Fields{
					"event":     "command_ping_mongo_error",
					"user_id":   meta.userID,
					"chat_id":   meta.chatID,
					"chat_type": normalizeChatType(meta.chatType),
				}).WithError(err).Error("mongo ping failed during /ping")
			} else {
				mongoStatus = "ok"
			}
		}

		messageText := pingMessage(diag.appEnv, time.Since(diag.processStart), mongoStatus)

		if b == nil {
			logger.WithFields(logging.Fields{
				"event":     "command_ping_send_failed",
				"user_id":   meta.userID,
				"chat_id":   meta.chatID,
				"chat_type": normalizeChatType(meta.chatType),
				"mongo":     mongoStatus,
			}).Error("cannot send ping response without telegram client")
			return
		}

		if _, err := sendMessage(ctx, b, &bot.SendMessageParams{
			ChatID: meta.chatID,
			Text:   messageText,
		}); err != nil {
			logger.WithFields(logging.Fields{
				"event":     "command_ping_send_failed",
				"user_id":   meta.userID,
				"chat_id":   meta.chatID,
				"chat_type": normalizeChatType(meta.chatType),
				"mongo":     mongoStatus,
			}).WithError(err).Error("failed to send ping response")
			return
		}

		logger.WithFields(logging.Fields{
			"event":     "command_ping_sent",
			"user_id":   meta.userID,
			"chat_id":   meta.chatID,
			"chat_type": normalizeChatType(meta.chatType),
			"mongo":     mongoStatus,
		}).Info("sent ping response")
	}
}

func pingMessage(appEnv string, uptime time.Duration, mongoStatus string) string {
	env := strings.TrimSpace(appEnv)
	if env == "" {
		env = config.DefaultAppEnv
	}

	if uptime < 0 {
		uptime = 0
	}
	uptime = uptime.Truncate(time.Second)

	mongo := strings.TrimSpace(mongoStatus)
	if mongo == "" {
		mongo = "error"
	}

	lines := []string{
		"pong",
		fmt.Sprintf("env: %s", env),
		fmt.Sprintf("uptime: %s", uptime),
		fmt.Sprintf("mongo: %s", mongo),
	}

	return strings.Join(lines, "\n")
}

type statusCounts struct {
	users  string
	groups string
}

func statusCommandHandler(logger *logrus.Entry, botOwnerID int64, diag commandDiagnostics) bot.HandlerFunc {
	if logger == nil {
		logger = logging.Logger()
	}
	diag = normalizeDiagnostics(diag)

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if ctx == nil || update == nil {
			return
		}

		meta := extractUpdateMeta(update)
		logCommandHandled(logger, "command_status", meta)

		if meta.chatID == 0 {
			logger.WithFields(logging.Fields{
				"event":     "command_status_send_failed",
				"user_id":   meta.userID,
				"chat_type": normalizeChatType(meta.chatType),
			}).Error("cannot send status response without chat_id")
			return
		}

		role := ""
		authorized := false

		if meta.userID == 0 {
			logger.WithFields(logging.Fields{
				"event":     "command_status_denied",
				"reason":    "missing_user_id",
				"chat_id":   meta.chatID,
				"chat_type": normalizeChatType(meta.chatType),
			}).Warn("status command denied due to missing user_id")
		} else if diag.userFetcher == nil {
			logger.WithFields(logging.Fields{
				"event":     "command_status_user_lookup_missing",
				"user_id":   meta.userID,
				"chat_id":   meta.chatID,
				"chat_type": normalizeChatType(meta.chatType),
			}).Error("status command missing user fetcher")
		} else {
			authCtx, cancel := context.WithTimeout(ctx, statusLookupTimeout)
			user, err := diag.userFetcher.GetByID(authCtx, meta.userID)
			cancel()

			if err != nil {
				logger.WithFields(logging.Fields{
					"event":     "command_status_user_lookup_failed",
					"user_id":   meta.userID,
					"chat_id":   meta.chatID,
					"chat_type": normalizeChatType(meta.chatType),
				}).WithError(err).Error("failed to load user for status command")
			} else {
				role = strings.TrimSpace(user.Role)
				if meta.userID == botOwnerID && domain.RolePriority(role) >= domain.RolePriorityOwner {
					authorized = true
				}
			}
		}

		if !authorized {
			if b == nil {
				logger.WithFields(logging.Fields{
					"event":     "command_status_send_failed",
					"user_id":   meta.userID,
					"chat_id":   meta.chatID,
					"chat_type": normalizeChatType(meta.chatType),
					"role":      role,
				}).Error("cannot send permission denied response without telegram client")
				return
			}

			if _, err := sendMessage(ctx, b, &bot.SendMessageParams{
				ChatID: meta.chatID,
				Text:   "permission denied",
			}); err != nil {
				logger.WithFields(logging.Fields{
					"event":     "command_status_send_failed",
					"user_id":   meta.userID,
					"chat_id":   meta.chatID,
					"chat_type": normalizeChatType(meta.chatType),
					"role":      role,
				}).WithError(err).Error("failed to send permission denied response")
				return
			}

			logger.WithFields(logging.Fields{
				"event":     "command_status_denied",
				"user_id":   meta.userID,
				"chat_id":   meta.chatID,
				"chat_type": normalizeChatType(meta.chatType),
				"role":      role,
			}).Info("status command denied")
			return
		}

		counts := statusCounts{
			users:  "error",
			groups: "error",
		}

		if diag.statsProvider == nil {
			logger.WithFields(logging.Fields{
				"event":     "command_status_stats_missing",
				"user_id":   meta.userID,
				"chat_id":   meta.chatID,
				"chat_type": normalizeChatType(meta.chatType),
				"role":      role,
			}).Error("status command missing stats provider")
		} else {
			statsCtx, cancel := context.WithTimeout(ctx, statusCountTimeout)
			userCount, userErr := diag.statsProvider.CountUsers(statsCtx)
			groupCount, groupErr := diag.statsProvider.CountGroups(statsCtx)
			cancel()

			if userErr != nil {
				logger.WithFields(logging.Fields{
					"event":     "command_status_user_count_error",
					"user_id":   meta.userID,
					"chat_id":   meta.chatID,
					"chat_type": normalizeChatType(meta.chatType),
					"role":      role,
				}).WithError(userErr).Error("failed to count users for /status")
			} else {
				counts.users = strconv.FormatInt(userCount, 10)
			}

			if groupErr != nil {
				logger.WithFields(logging.Fields{
					"event":     "command_status_group_count_error",
					"user_id":   meta.userID,
					"chat_id":   meta.chatID,
					"chat_type": normalizeChatType(meta.chatType),
					"role":      role,
				}).WithError(groupErr).Error("failed to count groups for /status")
			} else {
				counts.groups = strconv.FormatInt(groupCount, 10)
			}
		}

		messageText := statusMessage(diag.appEnv, counts)

		if b == nil {
			logger.WithFields(logging.Fields{
				"event":     "command_status_send_failed",
				"user_id":   meta.userID,
				"chat_id":   meta.chatID,
				"chat_type": normalizeChatType(meta.chatType),
				"role":      role,
				"users":     counts.users,
				"groups":    counts.groups,
			}).Error("cannot send status response without telegram client")
			return
		}

		if _, err := sendMessage(ctx, b, &bot.SendMessageParams{
			ChatID: meta.chatID,
			Text:   messageText,
		}); err != nil {
			logger.WithFields(logging.Fields{
				"event":     "command_status_send_failed",
				"user_id":   meta.userID,
				"chat_id":   meta.chatID,
				"chat_type": normalizeChatType(meta.chatType),
				"role":      role,
				"users":     counts.users,
				"groups":    counts.groups,
			}).WithError(err).Error("failed to send status response")
			return
		}

		logger.WithFields(logging.Fields{
			"event":     "command_status_sent",
			"user_id":   meta.userID,
			"chat_id":   meta.chatID,
			"chat_type": normalizeChatType(meta.chatType),
			"role":      role,
			"users":     counts.users,
			"groups":    counts.groups,
		}).Info("sent status response")
	}
}

func statusMessage(appEnv string, counts statusCounts) string {
	env := strings.TrimSpace(appEnv)
	if env == "" {
		env = config.DefaultAppEnv
	}

	userCount := strings.TrimSpace(counts.users)
	if userCount == "" {
		userCount = "error"
	}

	groupCount := strings.TrimSpace(counts.groups)
	if groupCount == "" {
		groupCount = "error"
	}

	lines := []string{
		"bot_status: running",
		fmt.Sprintf("env: %s", env),
		fmt.Sprintf("connected_chats: %s", groupCount),
		fmt.Sprintf("registered_users: %s", userCount),
	}

	return strings.Join(lines, "\n")
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
