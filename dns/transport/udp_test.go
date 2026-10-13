package transport

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	C "github.com/sagernet/sing-box/constant"
	boxDNS "github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing-box/log"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	mDNS "github.com/miekg/dns"
)

// fakePacketConn emulates a UDP connection in the read states reported in
// SagerNet/sing-box#4593: reads complete without yielding a usable DNS
// message (a zero-byte read, or a stream of malformed datagrams), which used
// to spin the multiplexer receive loop forever without invalidating the
// connection.
type fakePacketConn struct {
	done      chan struct{}
	closeOnce sync.Once
	reads     atomic.Int64
	readFn    func(b []byte) (int, error)
	writeFn   func(b []byte)
}

func newFakePacketConn(readFn func(b []byte) (int, error), writeFn func(b []byte)) *fakePacketConn {
	return &fakePacketConn{
		done:    make(chan struct{}),
		readFn:  readFn,
		writeFn: writeFn,
	}
}

func (c *fakePacketConn) Read(b []byte) (int, error) {
	c.reads.Add(1)
	return c.readFn(b)
}

func (c *fakePacketConn) Write(b []byte) (int, error) {
	if c.writeFn != nil {
		c.writeFn(b)
	}
	return len(b), nil
}

func (c *fakePacketConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
	})
	return nil
}

func (c *fakePacketConn) LocalAddr() net.Addr                { return fakePacketAddr{} }
func (c *fakePacketConn) RemoteAddr() net.Addr               { return fakePacketAddr{} }
func (c *fakePacketConn) SetDeadline(t time.Time) error      { return nil }
func (c *fakePacketConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *fakePacketConn) SetWriteDeadline(t time.Time) error { return nil }

type fakePacketAddr struct{}

func (fakePacketAddr) Network() string { return "udp" }
func (fakePacketAddr) String() string  { return "127.0.0.1:5353" }

type fakeConnDialer struct {
	conn net.Conn
}

func (d *fakeConnDialer) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	return d.conn, nil
}

func (d *fakeConnDialer) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	return nil, errors.New("not implemented")
}

var _ N.Dialer = (*fakeConnDialer)(nil)

func newFakeUDPTransport(conn net.Conn) *UDPTransport {
	return NewUDPRaw(
		log.NewNOPFactory().Logger(),
		boxDNS.NewTransportAdapter(C.DNSTypeUDP, "test", nil),
		&fakeConnDialer{conn: conn},
		M.ParseSocksaddr("127.0.0.1:5353"),
	)
}

// testUnproductiveReads drives an exchange through a connection whose reads
// never yield a usable message and asserts that the receive loop fails the
// connection after exactly the expected number of reads, without falling
// through to the exchange context deadline.
func testUnproductiveReads(t *testing.T, conn *fakePacketConn, expectedReads int64) {
	t.Helper()
	transport := newFakeUDPTransport(conn)
	t.Cleanup(func() {
		transport.multiplexer.Close()
	})
	message := new(mDNS.Msg)
	message.SetQuestion("spin.test.", mDNS.TypeA)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	start := time.Now()
	_, err := transport.Exchange(ctx, message)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("exchange unexpectedly succeeded")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("exchange fell through to the context deadline after %v with %d reads; the connection was never invalidated", elapsed, conn.reads.Load())
	}
	if elapsed > time.Second {
		t.Fatalf("exchange took %v; the connection should fail as soon as a read yields no usable response", elapsed)
	}
	if reads := conn.reads.Load(); reads != expectedReads {
		t.Fatalf("receive loop performed %d reads, expected exactly %d (%v)", reads, expectedReads, err)
	}
}

// A read that succeeds with zero bytes must fail the connection instead of
// being treated as an empty batch to retry forever.
func TestUDPTransportEmptyReadFailsConnection(t *testing.T) {
	t.Parallel()
	conn := newFakePacketConn(func(b []byte) (int, error) {
		return 0, nil
	}, nil)
	testUnproductiveReads(t, conn, 1)
}

// A connection that only produces malformed datagrams must be invalidated
// once the receive loop stops making progress, not spin forever.
func TestUDPTransportDiscardedResponsesFailConnection(t *testing.T) {
	t.Parallel()
	malformed := []byte{0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0xff}
	conn := newFakePacketConn(func(b []byte) (int, error) {
		return copy(b, malformed), nil
	}, nil)
	testUnproductiveReads(t, conn, 3)
}

// A valid response after a couple of discarded datagrams must still complete
// the exchange: the discard count only bounds consecutive unproductive reads.
func TestUDPTransportCompletesQueryAfterDiscardedResponses(t *testing.T) {
	t.Parallel()
	malformed := []byte{0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0xff}
	script := make(chan []byte, 4)
	script <- malformed
	script <- malformed
	var conn *fakePacketConn
	conn = newFakePacketConn(
		func(b []byte) (int, error) {
			select {
			case item := <-script:
				return copy(b, item), nil
			case <-conn.done:
				return 0, net.ErrClosed
			}
		},
		func(query []byte) {
			var question mDNS.Msg
			if err := question.Unpack(query); err != nil {
				return
			}
			response := new(mDNS.Msg)
			response.SetReply(&question)
			response.Answer = []mDNS.RR{&mDNS.A{
				Hdr: mDNS.RR_Header{
					Name:   question.Question[0].Name,
					Rrtype: mDNS.TypeA,
					Class:  mDNS.ClassINET,
					Ttl:    60,
				},
				A: net.ParseIP("192.0.2.7").To4(),
			}}
			packed, err := response.Pack()
			if err != nil {
				return
			}
			script <- packed
		},
	)
	transport := newFakeUDPTransport(conn)
	t.Cleanup(func() {
		transport.multiplexer.Close()
	})
	message := new(mDNS.Msg)
	message.SetQuestion("recover.test.", mDNS.TypeA)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	response, err := transport.Exchange(ctx, message)
	if err != nil {
		t.Fatalf("exchange failed after %d reads: %v", conn.reads.Load(), err)
	}
	if len(response.Answer) != 1 {
		t.Fatalf("expected one answer, got %d", len(response.Answer))
	}
	answer, isA := response.Answer[0].(*mDNS.A)
	if !isA || !answer.A.Equal(net.ParseIP("192.0.2.7")) {
		t.Fatalf("unexpected answer: %v", response.Answer[0])
	}
}
