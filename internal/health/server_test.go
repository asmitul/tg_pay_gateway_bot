package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
)

type stubMongoChecker struct {
	err error
}

func (s stubMongoChecker) Ping(context.Context) error {
	return s.err
}

func TestHealthHandlerOK(t *testing.T) {
	logger, _ := logtest.NewNullLogger()
	server := NewServer(0, stubMongoChecker{err: nil}, logrus.NewEntry(logger))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	server.server.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rr.Code)
	}

	body := strings.TrimSpace(rr.Body.String())
	if body != `{"status":"ok"}` {
		t.Fatalf("unexpected body: %s", body)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected content-type application/json, got %s", ct)
	}
}

func TestHealthHandlerMongoError(t *testing.T) {
	logger, _ := logtest.NewNullLogger()
	server := NewServer(0, stubMongoChecker{err: errors.New("mongo down")}, logrus.NewEntry(logger))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	server.server.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rr.Code)
	}

	body := strings.TrimSpace(rr.Body.String())
	if body != `{"status":"degraded","mongo":"error"}` {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestHealthHandlerMissingMongoChecker(t *testing.T) {
	logger, _ := logtest.NewNullLogger()
	server := NewServer(0, nil, logrus.NewEntry(logger))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	server.server.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rr.Code)
	}

	body := strings.TrimSpace(rr.Body.String())
	if body != `{"status":"degraded","mongo":"error"}` {
		t.Fatalf("unexpected body: %s", body)
	}
}
