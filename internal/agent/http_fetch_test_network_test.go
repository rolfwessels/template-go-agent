package agent

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Socket-free local httptest servers work even in sandboxes that prohibit
// loopback sockets. Only test files can access this listener and dialer.
type fetchPipeListener struct {
	connections chan net.Conn
	closed      chan struct{}
	once        sync.Once
}

func (l *fetchPipeListener) Accept() (net.Conn, error) {
	select {
	case <-l.closed:
		return nil, net.ErrClosed
	case conn := <-l.connections:
		return conn, nil
	}
}
func (l *fetchPipeListener) Close() error { l.once.Do(func() { close(l.closed) }); return nil }
func (l *fetchPipeListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}
}
func (l *fetchPipeListener) dial(ctx context.Context) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case <-ctx.Done():
		client.Close()
		server.Close()
		return nil, ctx.Err()
	case <-l.closed:
		client.Close()
		server.Close()
		return nil, net.ErrClosed
	case l.connections <- server:
		return client, nil
	}
}

var fetchTestListeners sync.Map

func newUnstartedFetchServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener := &fetchPipeListener{connections: make(chan net.Conn), closed: make(chan struct{})}
	srv := &httptest.Server{Listener: listener, Config: &http.Server{Handler: handler}}
	fetchTestListeners.Store(srv, listener)
	t.Cleanup(func() { srv.Close(); fetchTestListeners.Delete(srv) })
	return srv
}
func dialFetchTestServer(ctx context.Context, srv *httptest.Server) (net.Conn, error) {
	listener, ok := fetchTestListeners.Load(srv)
	if !ok {
		return nil, net.ErrClosed
	}
	return listener.(*fetchPipeListener).dial(ctx)
}
