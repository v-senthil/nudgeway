package websocket

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
)

// ClientOptions tunes the timing knobs of a single Client connection. Zero
// values fall back to sensible defaults chosen for the fullWA inbox use case.
type ClientOptions struct {
	// SendBuffer is the depth of the per-client outbound queue. When the
	// queue is full the frame is dropped and the drop counter is incremented
	// rather than blocking the broadcaster.
	SendBuffer int
	// PingInterval controls how often the write pump sends a WebSocket ping
	// to keep intermediaries from idling the connection.
	PingInterval time.Duration
	// WriteTimeout bounds a single frame write; exceeding it terminates the
	// connection.
	WriteTimeout time.Duration
}

// Defaults for ClientOptions.
const (
	defaultSendBuffer   = 64
	defaultPingInterval = 25 * time.Second
	defaultWriteTimeout = 10 * time.Second
)

// Client is one authenticated WebSocket connection. Phase 1 is server→client
// only, so the read pump drains and discards inbound frames and terminates
// on any read error (which includes a normal client-side close).
type Client struct {
	conn   *websocket.Conn
	room   *Room
	logger *slog.Logger

	orgID  string
	userID string

	send chan []byte
	// once closed, closed is triggered exactly once, so Close is idempotent.
	closeOnce sync.Once
	closed    chan struct{}

	dropped atomic.Uint64

	opts ClientOptions
}

// NewClient wraps an accepted websocket.Conn together with the org/user it
// authenticated as. It does not start any goroutines — call Run for that.
func NewClient(conn *websocket.Conn, room *Room, orgID, userID string, logger *slog.Logger, opts ClientOptions) *Client {
	if opts.SendBuffer <= 0 {
		opts.SendBuffer = defaultSendBuffer
	}
	if opts.PingInterval <= 0 {
		opts.PingInterval = defaultPingInterval
	}
	if opts.WriteTimeout <= 0 {
		opts.WriteTimeout = defaultWriteTimeout
	}
	return &Client{
		conn:   conn,
		room:   room,
		logger: logger,
		orgID:  orgID,
		userID: userID,
		send:   make(chan []byte, opts.SendBuffer),
		closed: make(chan struct{}),
		opts:   opts,
	}
}

// OrgID returns the org this Client is scoped to.
func (c *Client) OrgID() string { return c.orgID }

// UserID returns the user this Client authenticated as.
func (c *Client) UserID() string { return c.userID }

// Dropped is the count of broadcast frames dropped because the send channel
// was full. Exposed for tests and metrics.
func (c *Client) Dropped() uint64 { return c.dropped.Load() }

// Enqueue tries to push payload onto the client's send queue without
// blocking. If the queue is full the frame is dropped and the drop counter
// is incremented so a single slow consumer cannot stall the broadcaster.
func (c *Client) Enqueue(payload []byte) {
	select {
	case <-c.closed:
		return
	default:
	}
	select {
	case c.send <- payload:
	default:
		c.dropped.Add(1)
	}
}

// Close terminates the connection and unblocks Run. Safe to call multiple
// times and from multiple goroutines.
func (c *Client) Close(status websocket.StatusCode, reason string) {
	c.closeOnce.Do(func() {
		close(c.closed)
		// Best-effort close frame; ignore errors — the peer may already be gone.
		_ = c.conn.Close(status, reason)
	})
}

// Run blocks the caller for the lifetime of the connection. It launches one
// read pump and one write pump goroutine, then waits for either to return.
// The Room membership is added on entry and removed on exit so callers do
// not have to manage that themselves.
//
// ctx cancellation is honoured: cancelling ctx tears down the connection
// with a normal closure code.
func (c *Client) Run(ctx context.Context) {
	c.room.Add(c)
	defer c.room.Remove(c)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		c.readPump(ctx)
		cancel()
	}()
	go func() {
		defer wg.Done()
		c.writePump(ctx)
		cancel()
	}()

	<-ctx.Done()
	c.Close(websocket.StatusNormalClosure, "server shutdown")
	wg.Wait()
}

// readPump drains inbound frames and discards them. Phase 1 does not accept
// client-initiated messages; the pump exists so the peer's normal close is
// observed promptly.
func (c *Client) readPump(ctx context.Context) {
	for {
		if _, _, err := c.conn.Read(ctx); err != nil {
			if !isNormalClose(err) && ctx.Err() == nil {
				c.logger.Debug("ws read error",
					slog.String("org_id", c.orgID),
					slog.String("user_id", c.userID),
					slog.Any("err", err),
				)
			}
			return
		}
		// Ignore payload — server→client only for now.
	}
}

// writePump serialises writes and heartbeats onto the single connection.
// nhooyr's Conn.Write is safe for concurrent use, but multiplexing everything
// through this goroutine keeps timeouts and error handling in one place.
func (c *Client) writePump(ctx context.Context) {
	pingTicker := time.NewTicker(c.opts.PingInterval)
	defer pingTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case payload, ok := <-c.send:
			if !ok {
				return
			}
			wctx, cancel := context.WithTimeout(ctx, c.opts.WriteTimeout)
			err := c.conn.Write(wctx, websocket.MessageText, payload)
			cancel()
			if err != nil {
				c.logger.Debug("ws write error",
					slog.String("org_id", c.orgID),
					slog.String("user_id", c.userID),
					slog.Any("err", err),
				)
				return
			}
		case <-pingTicker.C:
			pctx, cancel := context.WithTimeout(ctx, c.opts.WriteTimeout)
			err := c.conn.Ping(pctx)
			cancel()
			if err != nil {
				c.logger.Debug("ws ping error",
					slog.String("org_id", c.orgID),
					slog.String("user_id", c.userID),
					slog.Any("err", err),
				)
				return
			}
		}
	}
}

// isNormalClose reports whether err represents a peer-initiated close that
// should not be logged as an anomaly.
func isNormalClose(err error) bool {
	if err == nil {
		return true
	}
	var ce websocket.CloseError
	if errors.As(err, &ce) {
		switch ce.Code {
		case websocket.StatusNormalClosure, websocket.StatusGoingAway, websocket.StatusNoStatusRcvd:
			return true
		}
	}
	return false
}
