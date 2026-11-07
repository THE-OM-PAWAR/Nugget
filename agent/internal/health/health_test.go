package health

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestCheckAppHealthEmptyContainer(t *testing.T) {
	h := CheckAppHealth(context.Background(), nil, "", 3000)
	if h.Status != StatusStopped {
		t.Errorf("Expected StatusStopped for empty container, got %s", h.Status)
	}
}

func TestProbePort(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	parts := strings.Split(ts.URL, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])

	if !probePort(context.Background(), port) {
		t.Errorf("Expected probePort to return true for active HTTP server on port %d", port)
	}

	// Inactive port
	freePort, err := getFreePort()
	if err == nil {
		if probePort(context.Background(), freePort) {
			t.Errorf("Expected probePort to return false for closed port %d", freePort)
		}
	}
}

func getFreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

