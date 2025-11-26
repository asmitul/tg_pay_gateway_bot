package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"tg_pay_gateway_bot/internal/config"
)

func TestNewManagerConnectsAndExposesCollections(t *testing.T) {
	fake := newFakeMongoClient(t)
	restore := stubConnect(fake, nil)
	t.Cleanup(restore)

	cfg := config.Config{
		MongoURI: "mongodb://stub-host:27017",
		MongoDB:  "tg_bot_test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	manager, err := NewManager(ctx, cfg)
	if err != nil {
		t.Fatalf("expected manager to initialize, got error: %v", err)
	}

	if manager.Database().Name() != cfg.MongoDB {
		t.Fatalf("expected database %s, got %s", cfg.MongoDB, manager.Database().Name())
	}

	if len(fake.databaseRequests) != 1 || fake.databaseRequests[0] != cfg.MongoDB {
		t.Fatalf("expected database request for %s, got %v", cfg.MongoDB, fake.databaseRequests)
	}

	if manager.Users().Name() != CollectionUsers {
		t.Fatalf("expected users collection name %s, got %s", CollectionUsers, manager.Users().Name())
	}

	if manager.Groups().Name() != CollectionGroups {
		t.Fatalf("expected groups collection name %s, got %s", CollectionGroups, manager.Groups().Name())
	}

	if err := manager.Close(ctx); err != nil {
		t.Fatalf("expected clean disconnect, got %v", err)
	}

	if !fake.disconnectCalled {
		t.Fatalf("expected disconnect to be called")
	}
}

func TestNewManagerFailsOnPingAndCleansUp(t *testing.T) {
	fake := newFakeMongoClient(t)
	fake.pingErr = errors.New("ping failed")

	restore := stubConnect(fake, nil)
	t.Cleanup(restore)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := NewManager(ctx, config.Config{MongoURI: "mongodb://stub", MongoDB: "tg_bot_test"})
	if err == nil {
		t.Fatalf("expected ping error")
	}

	if !fake.disconnectCalled {
		t.Fatalf("expected disconnect after ping failure")
	}
}

func TestNewManagerPropagatesConnectError(t *testing.T) {
	restore := stubConnect(nil, errors.New("connect failed"))
	t.Cleanup(restore)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := NewManager(ctx, config.Config{MongoURI: "mongodb://stub", MongoDB: "tg_bot_test"})
	if err == nil {
		t.Fatalf("expected connection error")
	}
}

func TestNewManagerValidatesContext(t *testing.T) {
	_, err := NewManager(nil, config.Config{MongoURI: "mongodb://stub", MongoDB: "tg_bot_test"})
	if err == nil {
		t.Fatalf("expected error for nil context")
	}
}

func TestManagerCloseRequiresContext(t *testing.T) {
	fake := newFakeMongoClient(t)
	restore := stubConnect(fake, nil)
	t.Cleanup(restore)

	manager, err := NewManager(context.Background(), config.Config{MongoURI: "mongodb://stub", MongoDB: "tg_bot_test"})
	if err != nil {
		t.Fatalf("expected manager to initialize, got error: %v", err)
	}

	if err := manager.Close(nil); err == nil {
		t.Fatalf("expected error for nil context")
	}
}

type fakeMongoClient struct {
	client           *mongo.Client
	pingErr          error
	disconnectErr    error
	disconnectCalled bool
	databaseRequests []string
}

func newFakeMongoClient(t *testing.T) *fakeMongoClient {
	t.Helper()

	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://example.com:27017"))
	if err != nil {
		t.Fatalf("failed to build fake client: %v", err)
	}

	return &fakeMongoClient{client: client}
}

func (f *fakeMongoClient) Ping(context.Context, *readpref.ReadPref) error {
	return f.pingErr
}

func (f *fakeMongoClient) Database(name string, opts ...*options.DatabaseOptions) *mongo.Database {
	f.databaseRequests = append(f.databaseRequests, name)
	return f.client.Database(name, opts...)
}

func (f *fakeMongoClient) Disconnect(context.Context) error {
	f.disconnectCalled = true
	return f.disconnectErr
}

func stubConnect(fake mongoClient, err error) func() {
	prev := connectMongo
	connectMongo = func(context.Context, *options.ClientOptions) (mongoClient, error) {
		return fake, err
	}

	return func() {
		connectMongo = prev
	}
}
