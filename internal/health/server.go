// Package health exposes a lightweight HTTP health endpoint for container probes.
package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"tg_pay_gateway_bot/internal/logging"
)

const (
	mongoPingTimeout   = 2 * time.Second
	readHeaderTimeout  = 2 * time.Second
	healthListenPrefix = ":"
)

// MongoChecker defines the subset of MongoDB client behavior required for health.
type MongoChecker interface {
	Ping(ctx context.Context) error
}

// Server hosts the health endpoint and owns the underlying HTTP server.
type Server struct {
	server       *http.Server
	logger       *logrus.Entry
	mongoChecker MongoChecker
}

type response struct {
	Status string `json:"status"`
	Mongo  string `json:"mongo,omitempty"`
}

// NewServer constructs a health server that exposes GET /healthz on the provided port.
func NewServer(port int, mongoChecker MongoChecker, logger *logrus.Entry) *Server {
	if logger == nil {
		logger = logging.Logger()
	}

	srv := &Server{
		logger:       logger,
		mongoChecker: mongoChecker,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", srv.handleHealth)

	srv.server = &http.Server{
		Addr:              fmt.Sprintf("%s%d", healthListenPrefix, port),
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	return srv
}

// ListenAndServe starts the health server and blocks until shutdown.
func (s *Server) ListenAndServe() error {
	s.logger.WithFields(logging.Fields{
		"event": "health_listen",
		"addr":  s.server.Addr,
	}).Info("starting health server")

	if err := s.server.ListenAndServe(); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			s.logger.WithField("event", "health_stopped").Info("health server stopped")
			return nil
		}

		return fmt.Errorf("health server listen: %w", err)
	}

	s.logger.WithField("event", "health_stopped").Info("health server stopped")
	return nil
}

// Shutdown gracefully stops the health server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}

	return s.server.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := response{Status: "ok"}
	mongoStatus := "ok"

	ctx := r.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	if s.mongoChecker == nil {
		mongoStatus = "error"
		s.logger.WithField("event", "health_mongo_missing").Warn("mongo checker is not configured for health endpoint")
	} else {
		pingCtx, cancel := context.WithTimeout(ctx, mongoPingTimeout)
		err := s.mongoChecker.Ping(pingCtx)
		cancel()

		if err != nil {
			mongoStatus = "error"
			s.logger.WithFields(logging.Fields{
				"event": "health_mongo_error",
			}).WithError(err).Warn("mongo ping failed during health check")
		}
	}

	if mongoStatus != "ok" {
		resp.Status = "degraded"
		resp.Mongo = "error"
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.WithField("event", "health_write_error").WithError(err).Error("failed to encode health response")
	}
}
