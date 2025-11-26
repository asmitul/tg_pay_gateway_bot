// Package store encapsulates MongoDB client management and collection helpers.
package store

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"tg_pay_gateway_bot/internal/config"
)

// Collection names used across the bot.
const (
	CollectionUsers  = "users"
	CollectionGroups = "groups"
)

// mongoClient captures the subset of mongo.Client behavior we rely on to allow
// lightweight stubbing in tests without a live Mongo deployment.
type mongoClient interface {
	Ping(context.Context, *readpref.ReadPref) error
	Database(string, ...*options.DatabaseOptions) *mongo.Database
	Disconnect(context.Context) error
}

// connectMongo is overridable for tests.
var connectMongo = func(ctx context.Context, opts *options.ClientOptions) (mongoClient, error) {
	return mongo.Connect(ctx, opts)
}

// Manager owns a MongoDB client and the configured database handle.
type Manager struct {
	client mongoClient
	db     *mongo.Database
}

// NewManager initializes the Mongo client using the supplied configuration and
// verifies connectivity with a ping.
func NewManager(ctx context.Context, cfg config.Config) (*Manager, error) {
	if ctx == nil {
		return nil, errors.New("context is required")
	}

	client, err := connectMongo(ctx, options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		return nil, fmt.Errorf("connect mongo: %w", err)
	}

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("ping mongo: %w", err)
	}

	return &Manager{
		client: client,
		db:     client.Database(cfg.MongoDB),
	}, nil
}

// Database returns the configured database handle.
func (m *Manager) Database() *mongo.Database {
	return m.db
}

// Client returns the underlying mongo.Client when available. Tests using fakes
// may receive nil here.
func (m *Manager) Client() *mongo.Client {
	client, ok := m.client.(*mongo.Client)
	if !ok {
		return nil
	}
	return client
}

// Collection returns a collection handle for the given name.
func (m *Manager) Collection(name string) *mongo.Collection {
	return m.db.Collection(name)
}

// Users returns the users collection handle.
func (m *Manager) Users() *mongo.Collection {
	return m.Collection(CollectionUsers)
}

// Groups returns the groups collection handle.
func (m *Manager) Groups() *mongo.Collection {
	return m.Collection(CollectionGroups)
}

// Close disconnects the Mongo client.
func (m *Manager) Close(ctx context.Context) error {
	if m == nil || m.client == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("context is required")
	}

	return m.client.Disconnect(ctx)
}
