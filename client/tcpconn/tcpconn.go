package tcpconn

import (
	"bufio"
	"github.com/gtxistxgao/safe-udp/common/consts"
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

func (t *TcpConn) Wait() (string, error) {
	message, err := bufio.NewReader(t.conn).ReadString('\n')
	if err != nil {
		log.Println(err)
		return err.Error(), err
	}

	if len(message) == 0 {
		return message, nil
	}

	return message[:len(message)-1], nil
}

func (t *TcpConn) RequestValidation() error {
	_, err := t.conn.Write([]byte(consts.Validate + "\n"))
	if err != nil {
		log.Println("Fail to ask server to validate", err)
		return err
	} else {
		log.Println("Asked server to validate")
	}

	return nil
}

func (t *TcpConn) Close() {
	if err := t.conn.Close(); err != nil {
		log.Println(err)
	}
}
