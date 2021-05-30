package udp_server

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
)

type UDPServer struct {
	packetConn    net.PacketConn
	maxBufferSize int
}

func New(address string, maxBufferSize int) (*UDPServer, error) {
	// ListenPacket provides us a wrapper around ListenUDP so that
	// we don't need to call `net.ResolveUDPAddr` and then subsequentially
	// perform a `ListenUDP` with the UDP address.
	//
	// The returned value (PacketConn) is pretty much the same as the one
	// from ListenUDP (UDPConn) - the only difference is that `Packet*`
	// methods and interfaces are more broad, also covering `ip`.

	packetConn, err := net.ListenPacket("udp", address)
	if err != nil {
		log.Printf("Got error when start to listen to address %s. Error: %s", address, err)
		return nil, err
	}

	fmt.Println("Start to listen to: ", packetConn.LocalAddr().String())

	return &UDPServer{
		packetConn:    packetConn,
		maxBufferSize: maxBufferSize,
	}, nil
}

func (s *UDPServer) GetPort() string {
	addr := s.packetConn.LocalAddr().String()
	log.Println("Get local address", addr)
	ipAndPort := strings.Split(addr, ":")
	return ipAndPort[len(ipAndPort)-1]
}

func (s *UDPServer) Close() error {
	fmt.Printf("Close connection on %v", s.packetConn.LocalAddr())
	fmt.Println()
	if err := s.packetConn.Close(); err != nil {
		return err
	}

	return nil
}

// Run will continuelly receiving data and publish the data to rawData Channel
func (s *UDPServer) Run(ctx context.Context, rawData chan []byte) error {
	buffer := make([]byte, s.maxBufferSize)
	doneChan := make(chan error, 1)
	go func(output chan []byte) {
		for {
			// By reading from the connection into the buffer, we block until there's
			// new content in the socket that we're listening for new packets.
			//
			// Whenever new packets arrive, `buffer` gets filled and we can continue
			// the execution.
			//
			// note.: `buffer` is not being reset between runs.
			//	  It's expected that only `n` reads are read from it whenever
			//	  inspecting its contents.
			n, addr, err := s.packetConn.ReadFrom(buffer)
			if err != nil {
				doneChan <- err
				return
			}

			temp := make([]byte, n)
			copy(temp, buffer[:n])
			fmt.Printf("packet-received: bytes=%d from=%s\n", n, addr.String())
			output <- temp
		}
	}(rawData)

	select {
	case <-ctx.Done():
		fmt.Println("udp_server run done", ctx.Err())
		return nil
	case err := <-doneChan:
		if err != nil {
			fmt.Println(err)
			return err
		}
	}

	return nil
}
