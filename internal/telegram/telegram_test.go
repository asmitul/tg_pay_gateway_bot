package telegram

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"

	"tg_pay_gateway_bot/internal/config"
	"tg_pay_gateway_bot/internal/domain"
)

type fakeBot struct {
	startedWith context.Context
}

func (f *fakeBot) Start(ctx context.Context) {
	f.startedWith = ctx
}

func TestNewClientCreatesBot(t *testing.T) {
	origCreateBot := createBot
	defer func() { createBot = origCreateBot }()

	var gotToken string
	var gotOptions []bot.Option
	b := &fakeBot{}

	createBot = func(token string, options ...bot.Option) (botRunner, error) {
		gotToken = token
		gotOptions = options
		return b, nil
	}

	cfg := config.Config{TelegramToken: "token-123"}
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	client, err := NewClient(cfg, logrus.NewEntry(logger))
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	if client == nil || client.bot == nil {
		t.Fatalf("expected client and bot to be initialized")
	}

	if gotToken != cfg.TelegramToken {
		t.Fatalf("expected token %q, got %q", cfg.TelegramToken, gotToken)
	}

	if len(gotOptions) != 3 {
		t.Fatalf("expected 3 bot options (allowed updates, default handler, error handler), got %d", len(gotOptions))
	}
}

func TestNewClientPropagatesBotError(t *testing.T) {
	origCreateBot := createBot
	defer func() { createBot = origCreateBot }()

	expected := errors.New("boom")
	createBot = func(string, ...bot.Option) (botRunner, error) {
		return nil, expected
	}

	_, err := NewClient(config.Config{TelegramToken: "token"}, nil)
	if !errors.Is(err, expected) {
		t.Fatalf("expected error %v, got %v", expected, err)
	}
}

func TestClientStartLogsAndUsesContext(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	client := &Client{
		bot:    &fakeBot{},
		logger: logrus.NewEntry(hookLogger),
	}

	ctx := context.Background()
	client.Start(ctx)

	if fb, ok := client.bot.(*fakeBot); ok {
		if fb.startedWith != ctx {
			t.Fatalf("expected bot to start with provided context")
		}
	}

	entries := hook.AllEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 log entries (start/stop), got %d", len(entries))
	}

	if entries[0].Data["event"] != "telegram_listen" {
		t.Fatalf("expected start log event, got %v", entries[0].Data["event"])
	}
	if entries[1].Data["event"] != "telegram_stopped" {
		t.Fatalf("expected stop log event, got %v", entries[1].Data["event"])
	}
}

func TestExtractUpdateMeta(t *testing.T) {
	tests := []struct {
		name   string
		update *models.Update
		want   updateMeta
	}{
		{
			name: "message",
			update: &models.Update{
				Message: &models.Message{
					From: &models.User{ID: 10},
					Chat: models.Chat{ID: 20, Type: models.ChatTypePrivate},
					Date: 1700000000,
					Text: " hello ",
				},
			},
			want: updateMeta{
				userID:     10,
				chatID:     20,
				text:       "hello",
				updateType: "message",
				chatType:   string(models.ChatTypePrivate),
				chatTitle:  "",
				timestamp:  time.Unix(1700000000, 0).UTC(),
			},
		},
		{
			name: "edited message",
			update: &models.Update{
				EditedMessage: &models.Message{
					From:     &models.User{ID: 11},
					Chat:     models.Chat{ID: 21, Type: models.ChatTypeSupergroup, Title: "Super Chat"},
					Text:     "updated",
					Date:     1700000001,
					EditDate: 1700000020,
				},
			},
			want: updateMeta{
				userID:     11,
				chatID:     21,
				text:       "updated",
				updateType: "edited_message",
				chatType:   string(models.ChatTypeSupergroup),
				chatTitle:  "Super Chat",
				timestamp:  time.Unix(1700000020, 0).UTC(),
			},
		},
		{
			name: "callback query",
			update: &models.Update{
				CallbackQuery: &models.CallbackQuery{
					From: models.User{ID: 12},
					Data: "choice",
					Message: models.MaybeInaccessibleMessage{
						Type: models.MaybeInaccessibleMessageTypeMessage,
						Message: &models.Message{
							Chat: models.Chat{ID: 22, Type: models.ChatTypeGroup, Title: "Callback Group"},
							Date: 1700000030,
						},
					},
				},
			},
			want: updateMeta{
				userID:     12,
				chatID:     22,
				text:       "choice",
				updateType: "callback_query",
				chatType:   string(models.ChatTypeGroup),
				chatTitle:  "Callback Group",
				timestamp:  time.Unix(1700000030, 0).UTC(),
			},
		},
		{
			name: "my chat member",
			update: &models.Update{
				MyChatMember: &models.ChatMemberUpdated{
					From: models.User{ID: 13},
					Chat: models.Chat{ID: 23, Type: models.ChatTypeGroup, Title: "My Chat Group"},
					Date: 1700000040,
				},
			},
			want: updateMeta{
				userID:     13,
				chatID:     23,
				updateType: "my_chat_member",
				chatType:   string(models.ChatTypeGroup),
				chatTitle:  "My Chat Group",
				timestamp:  time.Unix(1700000040, 0).UTC(),
			},
		},
		{
			name: "chat member",
			update: &models.Update{
				ChatMember: &models.ChatMemberUpdated{
					From: models.User{ID: 14},
					Chat: models.Chat{ID: 24, Type: models.ChatTypeGroup, Title: "Chat Member Group"},
					Date: 1700000050,
				},
			},
			want: updateMeta{
				userID:     14,
				chatID:     24,
				updateType: "chat_member",
				chatType:   string(models.ChatTypeGroup),
				chatTitle:  "Chat Member Group",
				timestamp:  time.Unix(1700000050, 0).UTC(),
			},
		},
		{
			name:   "unknown",
			update: &models.Update{},
			want:   updateMeta{updateType: "unknown", timestamp: time.Time{}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := extractUpdateMeta(tt.update)
			if got.userID != tt.want.userID || got.chatID != tt.want.chatID || got.text != tt.want.text || got.updateType != tt.want.updateType || got.chatType != tt.want.chatType || got.chatTitle != tt.want.chatTitle || !got.timestamp.Equal(tt.want.timestamp) {
				t.Fatalf("extractUpdateMeta() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDefaultHandlerLogsUpdate(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	handler := defaultHandler(logrus.NewEntry(hookLogger), nil, nil, 0, commandDiagnostics{})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 99},
			Chat: models.Chat{ID: 199, Type: models.ChatTypePrivate},
			Text: "ping",
			Date: 1700001000,
		},
	}

	handler(context.Background(), nil, update)

	var updateEntry *logrus.Entry
	for _, entry := range hook.AllEntries() {
		if entry.Data["event"] == "telegram_update" {
			updateEntry = entry
			break
		}
	}

	if updateEntry == nil {
		t.Fatalf("expected log entry from handler")
	}

	if updateEntry.Data["event"] != "telegram_update" {
		t.Fatalf("expected event=telegram_update, got %v", updateEntry.Data["event"])
	}
	if updateEntry.Data["user_id"] != int64(99) || updateEntry.Data["chat_id"] != int64(199) {
		t.Fatalf("expected user_id=99 and chat_id=199, got user_id=%v chat_id=%v", updateEntry.Data["user_id"], updateEntry.Data["chat_id"])
	}
	if updateEntry.Data["text"] != "ping" {
		t.Fatalf("expected text=ping, got %v", updateEntry.Data["text"])
	}
	if updateEntry.Data["update_type"] != "message" {
		t.Fatalf("expected update_type=message, got %v", updateEntry.Data["update_type"])
	}
	if updateEntry.Data["chat_type"] != "private" {
		t.Fatalf("expected chat_type=private, got %v", updateEntry.Data["chat_type"])
	}
	if updateEntry.Data["handler"] != "generic_message" {
		t.Fatalf("expected handler=generic_message, got %v", updateEntry.Data["handler"])
	}
	ts, ok := updateEntry.Data["update_ts"].(string)
	if !ok {
		t.Fatalf("expected update_ts to be a string, got %T", updateEntry.Data["update_ts"])
	}
	expectedTS := time.Unix(1700001000, 0).UTC().Format(time.RFC3339Nano)
	if ts != expectedTS {
		t.Fatalf("expected update_ts=%s, got %s", expectedTS, ts)
	}
}

func TestDefaultHandlerRegistersUser(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	registrar := &stubUserRegistrar{}
	handler := defaultHandler(logrus.NewEntry(hookLogger), registrar, nil, 0, commandDiagnostics{})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 66},
			Chat: models.Chat{ID: 166, Type: models.ChatTypePrivate},
			Text: "ping",
		},
	}

	handler(nil, nil, update)

	if len(registrar.calls) != 1 || registrar.calls[0] != 66 {
		t.Fatalf("expected registrar to be called with user_id=66, got %v", registrar.calls)
	}

	if findEvent(hook.AllEntries(), "telegram_update") == nil {
		t.Fatalf("expected telegram_update log entry")
	}
}

func TestDefaultHandlerRegistersUserOnStartCommand(t *testing.T) {
	hookLogger, _ := logtest.NewNullLogger()
	registrar := &stubUserRegistrar{}
	handler := defaultHandler(logrus.NewEntry(hookLogger), registrar, nil, 0, commandDiagnostics{})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 67},
			Chat: models.Chat{ID: 167, Type: models.ChatTypePrivate},
			Text: "/start",
		},
	}

	handler(context.Background(), nil, update)

	if len(registrar.calls) != 1 || registrar.calls[0] != 67 {
		t.Fatalf("expected registrar to be called once for /start, got %v", registrar.calls)
	}
}

func TestDefaultHandlerLogsRegistrationErrors(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	registrar := &stubUserRegistrar{err: errors.New("boom")}
	handler := defaultHandler(logrus.NewEntry(hookLogger), registrar, nil, 0, commandDiagnostics{})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 77},
			Chat: models.Chat{ID: 177, Type: models.ChatTypePrivate},
			Text: "hello",
		},
	}

	handler(context.Background(), nil, update)

	entry := findEvent(hook.AllEntries(), "user_registration_failed")
	if entry == nil {
		t.Fatalf("expected user_registration_failed log entry")
	}
	if entry.Data["user_id"] != int64(77) {
		t.Fatalf("expected user_id=77 in failure log, got %v", entry.Data["user_id"])
	}
	if entry.Data["chat_id"] != int64(177) {
		t.Fatalf("expected chat_id=177 in failure log, got %v", entry.Data["chat_id"])
	}
}

func TestDefaultHandlerRegistersGroup(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	groupRegistrar := &stubGroupRegistrar{}
	handler := defaultHandler(logrus.NewEntry(hookLogger), nil, groupRegistrar, 0, commandDiagnostics{})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 88},
			Chat: models.Chat{ID: -500, Type: models.ChatTypeSupergroup, Title: "My Group Title"},
			Text: "hello",
		},
	}

	handler(context.Background(), nil, update)

	if len(groupRegistrar.calls) != 1 {
		t.Fatalf("expected group registrar to be called once, got %d calls", len(groupRegistrar.calls))
	}

	call := groupRegistrar.calls[0]
	if call.chatID != -500 {
		t.Fatalf("expected chat_id=-500, got %d", call.chatID)
	}
	if call.title != "My Group Title" {
		t.Fatalf("expected title 'My Group Title', got %q", call.title)
	}

	if findEvent(hook.AllEntries(), "telegram_update") == nil {
		t.Fatalf("expected telegram_update log entry")
	}
}

func TestDefaultHandlerLogsGroupRegistrationErrors(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	groupRegistrar := &stubGroupRegistrar{err: errors.New("boom")}
	handler := defaultHandler(logrus.NewEntry(hookLogger), nil, groupRegistrar, 0, commandDiagnostics{})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 89},
			Chat: models.Chat{ID: -600, Type: models.ChatTypeGroup, Title: "Error Group"},
			Text: "hi",
		},
	}

	handler(context.Background(), nil, update)

	entry := findEvent(hook.AllEntries(), "group_registration_failed")
	if entry == nil {
		t.Fatalf("expected group_registration_failed log entry")
	}
	if entry.Data["chat_id"] != int64(-600) {
		t.Fatalf("expected chat_id=-600 in failure log, got %v", entry.Data["chat_id"])
	}
	if entry.Data["chat_title"] != "Error Group" {
		t.Fatalf("expected chat_title=Error Group in failure log, got %v", entry.Data["chat_title"])
	}
}

func TestDefaultHandlerSkipsGroupRegistrationForPrivateChats(t *testing.T) {
	hookLogger, _ := logtest.NewNullLogger()
	groupRegistrar := &stubGroupRegistrar{}
	handler := defaultHandler(logrus.NewEntry(hookLogger), nil, groupRegistrar, 0, commandDiagnostics{})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 90},
			Chat: models.Chat{ID: 190, Type: models.ChatTypePrivate},
			Text: "hello",
		},
	}

	handler(context.Background(), nil, update)

	if len(groupRegistrar.calls) != 0 {
		t.Fatalf("expected no group registration for private chat, got %d calls", len(groupRegistrar.calls))
	}
}

func TestDefaultHandlerRoutesStartCommand(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	handler := defaultHandler(logrus.NewEntry(hookLogger), nil, nil, 0, commandDiagnostics{})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 50},
			Chat: models.Chat{ID: 150, Type: models.ChatTypePrivate},
			Text: "/start",
		},
	}

	handler(context.Background(), nil, update)

	routeEntry := findEvent(hook.AllEntries(), "telegram_route")
	if routeEntry == nil {
		t.Fatalf("expected telegram_route log entry")
	}

	if routeEntry.Data["handler"] != "command_start" {
		t.Fatalf("expected handler=command_start, got %v", routeEntry.Data["handler"])
	}
	if routeEntry.Data["chat_type"] != "private" {
		t.Fatalf("expected chat_type=private, got %v", routeEntry.Data["chat_type"])
	}
	if routeEntry.Data["command"] != "start" {
		t.Fatalf("expected command=start, got %v", routeEntry.Data["command"])
	}

	commandEntry := findEvent(hook.AllEntries(), "command_handler")
	if commandEntry == nil {
		t.Fatalf("expected command_handler log entry")
	}

	if commandEntry.Data["handler"] != "command_start" {
		t.Fatalf("expected command handler name command_start, got %v", commandEntry.Data["handler"])
	}
	if commandEntry.Data["chat_type"] != "private" {
		t.Fatalf("expected command handler chat_type=private, got %v", commandEntry.Data["chat_type"])
	}
}

func TestDefaultHandlerRoutesPingCommand(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	handler := defaultHandler(logrus.NewEntry(hookLogger), nil, nil, 0, commandDiagnostics{})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 60},
			Chat: models.Chat{ID: 160, Type: models.ChatTypePrivate},
			Text: "/ping",
		},
	}

	handler(context.Background(), nil, update)

	routeEntry := findEvent(hook.AllEntries(), "telegram_route")
	if routeEntry == nil {
		t.Fatalf("expected telegram_route log entry")
	}

	if routeEntry.Data["handler"] != "command_ping" {
		t.Fatalf("expected handler=command_ping, got %v", routeEntry.Data["handler"])
	}
	if routeEntry.Data["command"] != "ping" {
		t.Fatalf("expected command=ping, got %v", routeEntry.Data["command"])
	}

	commandEntry := findEvent(hook.AllEntries(), "command_handler")
	if commandEntry == nil {
		t.Fatalf("expected command_handler log entry")
	}
	if commandEntry.Data["handler"] != "command_ping" {
		t.Fatalf("expected command handler name command_ping, got %v", commandEntry.Data["handler"])
	}
}

func TestDefaultHandlerRoutesStatusCommand(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	handler := defaultHandler(logrus.NewEntry(hookLogger), nil, nil, 55, commandDiagnostics{})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 55},
			Chat: models.Chat{ID: 155, Type: models.ChatTypePrivate},
			Text: "/status",
		},
	}

	handler(context.Background(), nil, update)

	routeEntry := findEvent(hook.AllEntries(), "telegram_route")
	if routeEntry == nil {
		t.Fatalf("expected telegram_route log entry")
	}

	if routeEntry.Data["handler"] != "command_status" {
		t.Fatalf("expected handler=command_status, got %v", routeEntry.Data["handler"])
	}
	if routeEntry.Data["command"] != "status" {
		t.Fatalf("expected command=status, got %v", routeEntry.Data["command"])
	}

	commandEntry := findEvent(hook.AllEntries(), "command_handler")
	if commandEntry == nil {
		t.Fatalf("expected command_handler log entry")
	}
	if commandEntry.Data["handler"] != "command_status" {
		t.Fatalf("expected command handler name command_status, got %v", commandEntry.Data["handler"])
	}
}

func TestStartCommandRepliesInPrivateChat(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()

	origSendMessage := sendMessage
	defer func() { sendMessage = origSendMessage }()

	var sentParams *bot.SendMessageParams
	sendMessage = func(ctx context.Context, b *bot.Bot, params *bot.SendMessageParams) (*models.Message, error) {
		sentParams = params
		return &models.Message{}, nil
	}

	handler := startCommandHandler(logrus.NewEntry(hookLogger), 42)

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 42},
			Chat: models.Chat{ID: 200, Type: models.ChatTypePrivate},
			Text: "/start",
		},
	}

	handler(context.Background(), &bot.Bot{}, update)

	if sentParams == nil {
		t.Fatalf("expected start command to send a message")
	}
	if sentParams.ChatID != int64(200) {
		t.Fatalf("expected start message to be sent to chat 200, got %v", sentParams.ChatID)
	}
	if !strings.Contains(sentParams.Text, "base build") {
		t.Fatalf("expected welcome text to mention base build, got %q", sentParams.Text)
	}
	if !strings.Contains(sentParams.Text, "role: owner") {
		t.Fatalf("expected welcome text to include owner role, got %q", sentParams.Text)
	}
	if !strings.Contains(sentParams.Text, "Status: registered") {
		t.Fatalf("expected welcome text to include registration status, got %q", sentParams.Text)
	}

	if findEvent(hook.AllEntries(), "command_start_sent") == nil {
		t.Fatalf("expected command_start_sent log entry")
	}
}

func TestStartCommandIgnoredInGroup(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()

	origSendMessage := sendMessage
	defer func() { sendMessage = origSendMessage }()

	sendMessage = func(ctx context.Context, b *bot.Bot, params *bot.SendMessageParams) (*models.Message, error) {
		t.Fatalf("expected no message send for group /start")
		return nil, nil
	}

	handler := startCommandHandler(logrus.NewEntry(hookLogger), 0)

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 43},
			Chat: models.Chat{ID: -300, Type: models.ChatTypeGroup},
			Text: "/start",
		},
	}

	handler(context.Background(), &bot.Bot{}, update)

	if findEvent(hook.AllEntries(), "command_start_ignored") == nil {
		t.Fatalf("expected command_start_ignored log entry")
	}
	if findEvent(hook.AllEntries(), "command_start_sent") != nil {
		t.Fatalf("expected no command_start_sent log entry for group chat")
	}
}

func TestPingCommandRepliesWithDiagnostics(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()

	origSendMessage := sendMessage
	defer func() { sendMessage = origSendMessage }()

	var sentParams *bot.SendMessageParams
	sendMessage = func(ctx context.Context, b *bot.Bot, params *bot.SendMessageParams) (*models.Message, error) {
		sentParams = params
		return &models.Message{}, nil
	}

	checker := &stubMongoChecker{}

	handler := pingCommandHandler(logrus.NewEntry(hookLogger), commandDiagnostics{
		appEnv:       "development",
		processStart: time.Now().Add(-2 * time.Hour),
		mongoChecker: checker,
	})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 120},
			Chat: models.Chat{ID: 220, Type: models.ChatTypePrivate},
			Text: "/ping",
		},
	}

	handler(context.Background(), &bot.Bot{}, update)

	if sentParams == nil {
		t.Fatalf("expected ping command to send a message")
	}
	if sentParams.ChatID != int64(220) {
		t.Fatalf("expected ping message to be sent to chat 220, got %v", sentParams.ChatID)
	}
	if !strings.Contains(sentParams.Text, "pong") {
		t.Fatalf("expected ping response to include pong, got %q", sentParams.Text)
	}
	if !strings.Contains(sentParams.Text, "env: development") {
		t.Fatalf("expected ping response to include env, got %q", sentParams.Text)
	}
	if !strings.Contains(sentParams.Text, "uptime: ") {
		t.Fatalf("expected ping response to include uptime, got %q", sentParams.Text)
	}
	if !strings.Contains(sentParams.Text, "mongo: ok") {
		t.Fatalf("expected ping response to include mongo status ok, got %q", sentParams.Text)
	}
	if checker.calls != 1 {
		t.Fatalf("expected mongo checker to be called once, got %d", checker.calls)
	}

	if findEvent(hook.AllEntries(), "command_ping_sent") == nil {
		t.Fatalf("expected command_ping_sent log entry")
	}
	if findEvent(hook.AllEntries(), "command_ping_mongo_error") != nil {
		t.Fatalf("expected no mongo error log for successful ping")
	}
}

func TestPingCommandReportsMongoError(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()

	origSendMessage := sendMessage
	defer func() { sendMessage = origSendMessage }()

	var sentParams *bot.SendMessageParams
	sendMessage = func(ctx context.Context, b *bot.Bot, params *bot.SendMessageParams) (*models.Message, error) {
		sentParams = params
		return &models.Message{}, nil
	}

	checker := &stubMongoChecker{err: errors.New("mongo down")}

	handler := pingCommandHandler(logrus.NewEntry(hookLogger), commandDiagnostics{
		appEnv:       "production",
		processStart: time.Now().Add(-10 * time.Minute),
		mongoChecker: checker,
	})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 121},
			Chat: models.Chat{ID: 221, Type: models.ChatTypeGroup},
			Text: "/ping",
		},
	}

	handler(context.Background(), &bot.Bot{}, update)

	if sentParams == nil {
		t.Fatalf("expected ping command to send a message even on mongo error")
	}
	if !strings.Contains(sentParams.Text, "mongo: error") {
		t.Fatalf("expected mongo error in ping response, got %q", sentParams.Text)
	}
	if checker.calls != 1 {
		t.Fatalf("expected mongo checker to be called once, got %d", checker.calls)
	}

	if findEvent(hook.AllEntries(), "command_ping_mongo_error") == nil {
		t.Fatalf("expected command_ping_mongo_error log entry")
	}
	if findEvent(hook.AllEntries(), "command_ping_sent") == nil {
		t.Fatalf("expected command_ping_sent log entry even after mongo error")
	}
}

func TestStatusCommandRepliesWithCountsForOwner(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()

	origSendMessage := sendMessage
	defer func() { sendMessage = origSendMessage }()

	var sentParams *bot.SendMessageParams
	sendMessage = func(ctx context.Context, b *bot.Bot, params *bot.SendMessageParams) (*models.Message, error) {
		sentParams = params
		return &models.Message{}, nil
	}

	fetcher := &stubUserFetcher{user: domain.User{UserID: 500, Role: domain.RoleOwner}}
	stats := &stubStatsProvider{usersCount: 7, groupsCount: 3}

	handler := statusCommandHandler(logrus.NewEntry(hookLogger), 500, commandDiagnostics{
		appEnv:        "development",
		userFetcher:   fetcher,
		statsProvider: stats,
	})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 500},
			Chat: models.Chat{ID: 320, Type: models.ChatTypePrivate},
			Text: "/status",
		},
	}

	handler(context.Background(), &bot.Bot{}, update)

	if sentParams == nil {
		t.Fatalf("expected status command to send a message")
	}
	if sentParams.ChatID != int64(320) {
		t.Fatalf("expected status message to be sent to chat 320, got %v", sentParams.ChatID)
	}
	if !strings.Contains(sentParams.Text, "bot_status: running") {
		t.Fatalf("expected status response to include bot status, got %q", sentParams.Text)
	}
	if !strings.Contains(sentParams.Text, "env: development") {
		t.Fatalf("expected status response to include env, got %q", sentParams.Text)
	}
	if !strings.Contains(sentParams.Text, "connected_chats: 3") {
		t.Fatalf("expected status response to include group count, got %q", sentParams.Text)
	}
	if !strings.Contains(sentParams.Text, "registered_users: 7") {
		t.Fatalf("expected status response to include user count, got %q", sentParams.Text)
	}

	if len(fetcher.calls) != 1 || fetcher.calls[0] != 500 {
		t.Fatalf("expected user fetcher to be called once with owner id, got %v", fetcher.calls)
	}
	if stats.userCalls != 1 || stats.groupCalls != 1 {
		t.Fatalf("expected stats provider to be called once for users/groups, got userCalls=%d groupCalls=%d", stats.userCalls, stats.groupCalls)
	}

	if findEvent(hook.AllEntries(), "command_status_sent") == nil {
		t.Fatalf("expected command_status_sent log entry")
	}
	if findEvent(hook.AllEntries(), "command_status_denied") != nil {
		t.Fatalf("expected no command_status_denied log entry for owner")
	}
}

func TestStatusCommandDeniesNonOwner(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()

	origSendMessage := sendMessage
	defer func() { sendMessage = origSendMessage }()

	var sentParams *bot.SendMessageParams
	sendMessage = func(ctx context.Context, b *bot.Bot, params *bot.SendMessageParams) (*models.Message, error) {
		sentParams = params
		return &models.Message{}, nil
	}

	fetcher := &stubUserFetcher{user: domain.User{UserID: 600, Role: domain.RoleUser}}
	stats := &stubStatsProvider{usersCount: 9, groupsCount: 4}

	handler := statusCommandHandler(logrus.NewEntry(hookLogger), 500, commandDiagnostics{
		appEnv:        "production",
		userFetcher:   fetcher,
		statsProvider: stats,
	})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 600},
			Chat: models.Chat{ID: 360, Type: models.ChatTypePrivate},
			Text: "/status",
		},
	}

	handler(context.Background(), &bot.Bot{}, update)

	if sentParams == nil {
		t.Fatalf("expected status command to reply even when denied")
	}
	if !strings.Contains(sentParams.Text, "permission denied") {
		t.Fatalf("expected permission denied message, got %q", sentParams.Text)
	}

	if stats.userCalls != 0 || stats.groupCalls != 0 {
		t.Fatalf("expected stats provider not to be called on denial, got userCalls=%d groupCalls=%d", stats.userCalls, stats.groupCalls)
	}

	if findEvent(hook.AllEntries(), "command_status_denied") == nil {
		t.Fatalf("expected command_status_denied log entry")
	}
	if findEvent(hook.AllEntries(), "command_status_sent") != nil {
		t.Fatalf("expected no command_status_sent log entry for denied user")
	}
}

func TestStatusCommandHandlesCountErrors(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()

	origSendMessage := sendMessage
	defer func() { sendMessage = origSendMessage }()

	var sentParams *bot.SendMessageParams
	sendMessage = func(ctx context.Context, b *bot.Bot, params *bot.SendMessageParams) (*models.Message, error) {
		sentParams = params
		return &models.Message{}, nil
	}

	fetcher := &stubUserFetcher{user: domain.User{UserID: 501, Role: domain.RoleOwner}}
	stats := &stubStatsProvider{
		userErr:     errors.New("users down"),
		groupErr:    errors.New("groups down"),
		usersCount:  0,
		groupsCount: 0,
	}

	handler := statusCommandHandler(logrus.NewEntry(hookLogger), 501, commandDiagnostics{
		appEnv:        "production",
		userFetcher:   fetcher,
		statsProvider: stats,
	})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 501},
			Chat: models.Chat{ID: 361, Type: models.ChatTypePrivate},
			Text: "/status",
		},
	}

	handler(context.Background(), &bot.Bot{}, update)

	if sentParams == nil {
		t.Fatalf("expected status command to send a message even on errors")
	}
	if !strings.Contains(sentParams.Text, "connected_chats: error") {
		t.Fatalf("expected group error indicator, got %q", sentParams.Text)
	}
	if !strings.Contains(sentParams.Text, "registered_users: error") {
		t.Fatalf("expected user error indicator, got %q", sentParams.Text)
	}

	if findEvent(hook.AllEntries(), "command_status_group_count_error") == nil {
		t.Fatalf("expected command_status_group_count_error log entry")
	}
	if findEvent(hook.AllEntries(), "command_status_user_count_error") == nil {
		t.Fatalf("expected command_status_user_count_error log entry")
	}
	if findEvent(hook.AllEntries(), "command_status_sent") == nil {
		t.Fatalf("expected command_status_sent log entry despite errors")
	}
}

func TestDefaultHandlerRoutesUnknownCommandInGroup(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	handler := defaultHandler(logrus.NewEntry(hookLogger), nil, nil, 0, commandDiagnostics{})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 51},
			Chat: models.Chat{ID: 151, Type: models.ChatTypeGroup},
			Text: "/unknown arg",
		},
	}

	handler(context.Background(), nil, update)

	routeEntry := findEvent(hook.AllEntries(), "telegram_route")
	if routeEntry == nil {
		t.Fatalf("expected telegram_route log entry")
	}

	if routeEntry.Data["handler"] != "command_unknown" {
		t.Fatalf("expected handler=command_unknown, got %v", routeEntry.Data["handler"])
	}
	if routeEntry.Data["chat_type"] != "group" {
		t.Fatalf("expected chat_type=group, got %v", routeEntry.Data["chat_type"])
	}

	commandEntry := findEvent(hook.AllEntries(), "command_handler")
	if commandEntry == nil {
		t.Fatalf("expected command_handler log entry")
	}
	if commandEntry.Data["handler"] != "command_unknown" {
		t.Fatalf("expected command handler name command_unknown, got %v", commandEntry.Data["handler"])
	}
}

func TestDefaultHandlerRoutesGenericMessage(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	handler := defaultHandler(logrus.NewEntry(hookLogger), nil, nil, 0, commandDiagnostics{})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 52},
			Chat: models.Chat{ID: 152, Type: models.ChatTypeSupergroup},
			Text: "just text",
		},
	}

	handler(context.Background(), nil, update)

	routeEntry := findEvent(hook.AllEntries(), "telegram_route")
	if routeEntry == nil {
		t.Fatalf("expected telegram_route log entry")
	}

	if routeEntry.Data["handler"] != "generic_message" {
		t.Fatalf("expected handler=generic_message, got %v", routeEntry.Data["handler"])
	}
	if routeEntry.Data["chat_type"] != "group" {
		t.Fatalf("expected chat_type=group, got %v", routeEntry.Data["chat_type"])
	}

	genericEntry := findEvent(hook.AllEntries(), "generic_handler")
	if genericEntry == nil {
		t.Fatalf("expected generic_handler log entry")
	}
	if genericEntry.Data["handler"] != "generic_message" {
		t.Fatalf("expected generic handler name generic_message, got %v", genericEntry.Data["handler"])
	}
	if genericEntry.Data["chat_type"] != "group" {
		t.Fatalf("expected generic handler chat_type=group, got %v", genericEntry.Data["chat_type"])
	}
}

type stubUserRegistrar struct {
	calls []int64
	err   error
}

func (s *stubUserRegistrar) EnsureUser(_ context.Context, userID int64) (bool, error) {
	s.calls = append(s.calls, userID)
	return false, s.err
}

type stubGroupRegistrar struct {
	calls []groupCall
	err   error
}

type groupCall struct {
	chatID int64
	title  string
}

func (s *stubGroupRegistrar) EnsureGroup(_ context.Context, chatID int64, title string) (bool, error) {
	s.calls = append(s.calls, groupCall{chatID: chatID, title: title})
	return false, s.err
}

type stubMongoChecker struct {
	calls int
	err   error
}

func (s *stubMongoChecker) Ping(ctx context.Context) error {
	s.calls++
	return s.err
}

type stubUserFetcher struct {
	user  domain.User
	err   error
	calls []int64
}

func (s *stubUserFetcher) GetByID(ctx context.Context, userID int64) (domain.User, error) {
	s.calls = append(s.calls, userID)
	return s.user, s.err
}

type stubStatsProvider struct {
	usersCount  int64
	groupsCount int64
	userErr     error
	groupErr    error
	userCalls   int
	groupCalls  int
}

func (s *stubStatsProvider) CountUsers(ctx context.Context) (int64, error) {
	s.userCalls++
	return s.usersCount, s.userErr
}

func (s *stubStatsProvider) CountGroups(ctx context.Context) (int64, error) {
	s.groupCalls++
	return s.groupsCount, s.groupErr
}

func findEvent(entries []*logrus.Entry, event string) *logrus.Entry {
	for _, entry := range entries {
		if entry.Data["event"] == event {
			return entry
		}
	}

	return nil
}
