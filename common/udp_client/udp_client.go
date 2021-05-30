package udp_client

import (
	"context"
	"fmt"
	"log"
	"net"
	"syscall"
	"time"
)

type UDPClient struct {
	conn         *net.UDPConn
	maxChunkSize int
	timeoutLimit time.Duration
}

func New(ctx context.Context, address string, maxChunkSize int, timeoutLimit time.Duration) *UDPClient {
	// Resolve the UDP address so that we can make use of DialUDP
	// with an actual IP and port instead of a name (in case a
	// hostname is specified).
	raddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		fmt.Print(err)
		return nil
	}

	// Although we're not in a connection-oriented transport,
	// the act of `dialing` is analogous to the act of performing
	// a `connect(2)` syscall for a socket of type SOCK_DGRAM:
	// - it forces the underlying socket to only read and write
	//   to and from a specific remote address.
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		fmt.Print(err)
		return nil
	}

	return &UDPClient{
		conn:         conn,
		maxChunkSize: maxChunkSize,
		timeoutLimit: timeoutLimit,
	}
}

func (c *UDPClient) Close() {
	// Closes the underlying file descriptor associated with the,
	// socket so that it no longer refers to any file.
	defer c.conn.Close()
}

func (c *UDPClient) SetBufferValue(bufferValue int) {
	c.conn.SetWriteBuffer(bufferValue)
}

func (c *UDPClient) GetBufferValue() int {
	fd, _ := c.conn.File()
	value, _ := syscall.GetsockoptInt(int(fd.Fd()), syscall.SOL_SOCKET, syscall.SO_SNDBUF)
	return value
}

func (c *UDPClient) SendAsync(ctx context.Context, chunk []byte) error {
	if len(chunk) > c.maxChunkSize {
		return fmt.Errorf("chunk size %d exceeded the max chunk size limit %d", len(chunk), c.maxChunkSize)
	}

	doneChan := make(chan error, 1)
	go func() {
		// It is possible that this action blocks, although this
		// should only occur in very resource-intensive situations:
		// - when you've filled up the socket buffer and the OS
		//   can't dequeue the queue fast enough.
		n, err := c.conn.Write(chunk)
		if err != nil {
			doneChan <- err
			return
		}

		log.Printf("packet-written: bytes=%d\n", n)
		doneChan <- nil
	}()

	select {
	case <-ctx.Done():
		fmt.Println("cancelled")
		return ctx.Err()
	case err := <-doneChan:
		if err != nil {
			fmt.Println(err)
			return err
		}
	case <-time.After(c.timeoutLimit):
		fmt.Println("operation timeout")
	}

	return nil
}
