package telegram

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"

	"tg_pay_gateway_bot/internal/config"
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
					Text: " hello ",
				},
			},
			want: updateMeta{userID: 10, chatID: 20, text: "hello", updateType: "message", chatType: string(models.ChatTypePrivate)},
		},
		{
			name: "edited message",
			update: &models.Update{
				EditedMessage: &models.Message{
					From: &models.User{ID: 11},
					Chat: models.Chat{ID: 21, Type: models.ChatTypeSupergroup},
					Text: "updated",
				},
			},
			want: updateMeta{userID: 11, chatID: 21, text: "updated", updateType: "edited_message", chatType: string(models.ChatTypeSupergroup)},
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
							Chat: models.Chat{ID: 22, Type: models.ChatTypeGroup},
						},
					},
				},
			},
			want: updateMeta{userID: 12, chatID: 22, text: "choice", updateType: "callback_query", chatType: string(models.ChatTypeGroup)},
		},
		{
			name: "my chat member",
			update: &models.Update{
				MyChatMember: &models.ChatMemberUpdated{
					From: models.User{ID: 13},
					Chat: models.Chat{ID: 23, Type: models.ChatTypeGroup},
				},
			},
			want: updateMeta{userID: 13, chatID: 23, updateType: "my_chat_member", chatType: string(models.ChatTypeGroup)},
		},
		{
			name: "chat member",
			update: &models.Update{
				ChatMember: &models.ChatMemberUpdated{
					From: models.User{ID: 14},
					Chat: models.Chat{ID: 24, Type: models.ChatTypeGroup},
				},
			},
			want: updateMeta{userID: 14, chatID: 24, updateType: "chat_member", chatType: string(models.ChatTypeGroup)},
		},
		{
			name:   "unknown",
			update: &models.Update{},
			want:   updateMeta{updateType: "unknown"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := extractUpdateMeta(tt.update)
			if got.userID != tt.want.userID || got.chatID != tt.want.chatID || got.text != tt.want.text || got.updateType != tt.want.updateType || got.chatType != tt.want.chatType {
				t.Fatalf("extractUpdateMeta() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDefaultHandlerLogsUpdate(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	handler := defaultHandler(logrus.NewEntry(hookLogger))

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 99},
			Chat: models.Chat{ID: 199, Type: models.ChatTypePrivate},
			Text: "ping",
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
}

func TestDefaultHandlerRoutesStartCommand(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	handler := defaultHandler(logrus.NewEntry(hookLogger))

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

func TestDefaultHandlerRoutesUnknownCommandInGroup(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	handler := defaultHandler(logrus.NewEntry(hookLogger))

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
	handler := defaultHandler(logrus.NewEntry(hookLogger))

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

func findEvent(entries []*logrus.Entry, event string) *logrus.Entry {
	for _, entry := range entries {
		if entry.Data["event"] == event {
			return entry
		}
	}

	return nil
}
