package tcpconn

import (
	"bufio"
	"log"
	"net"
)

type TcpConn struct {
	conn net.Conn
}

func New(conn net.Conn) *TcpConn {
	return &TcpConn{
		conn: conn,
	}
}

func (t *TcpConn) Wait() string {
	message, err := bufio.NewReader(t.conn).ReadString('\n')
	if err != nil {
		log.Println(err)
	}

	return message
}

func (t *TcpConn) Close() {
	if err := t.conn.Close(); err != nil {
		log.Println(err)
	}
}
