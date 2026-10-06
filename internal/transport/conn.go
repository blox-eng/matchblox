// Package transport carries proto messages: a local socket, or stdio when
// the console reaches the host over SSH.
package transport

import (
	"bufio"
	"io"
	"sync"

	"github.com/blox-eng/matchblox/internal/proto"
)

// Conn is safe for one reader and many senders.
type Conn interface {
	Send(kind proto.Kind, id string, body any) error
	Recv() (proto.Envelope, error)
	Close() error
}

type conn struct {
	mu    sync.Mutex
	w     io.Writer
	r     *bufio.Reader
	close func() error
}

func (c *conn) Send(kind proto.Kind, id string, body any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return proto.Encode(c.w, kind, id, body)
}

func (c *conn) Recv() (proto.Envelope, error) { return proto.Decode(c.r) }

func (c *conn) Close() error { return c.close() }

// NewConn wraps a stream, such as an accepted socket.
func NewConn(rwc io.ReadWriteCloser) Conn {
	return &conn{w: rwc, r: bufio.NewReader(rwc), close: rwc.Close}
}

// Pipe copies messages both ways until either side ends, then closes both.
// `serve --stdio` uses it to join SSH to the local service.
func Pipe(a, b Conn) {
	var once sync.Once
	done := make(chan struct{})
	stop := func() {
		once.Do(func() {
			a.Close()
			b.Close()
			close(done)
		})
	}
	copyTo := func(dst, src Conn) {
		defer stop()
		for {
			env, err := src.Recv()
			if err != nil {
				return
			}
			if err := dst.Send(env.Kind, env.ID, env.Body); err != nil {
				return
			}
		}
	}
	go copyTo(a, b)
	go copyTo(b, a)
	<-done
}
