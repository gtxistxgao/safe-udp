package tcpconn

import (
	"bufio"
	"encoding/json"
	"github.com/gtxistxgao/safe-udp/common/consts"
	"github.com/gtxistxgao/safe-udp/common/filemeta"
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

// Tell user which UDP port the server is listen to
func (t *TcpConn) SendPort(port string) {
	msg := consts.UdpPort + port
	_, err := t.conn.Write([]byte(msg))
	if err != nil {
		log.Println("Fail to tell user the port. Error: ", err)
	} else {
		log.Println("Told user the UCP port is ", port)
	}
}

func (t *TcpConn) GetFileInfo() filemeta.FileMeta {
	info := t.Wait()
	fileMeta := filemeta.FileMeta{}
	err := json.Unmarshal([]byte(info), &fileMeta)
	if err != nil {
		log.Println("Fail to parse file metadata, error: ", err)
	}

	return fileMeta
}

func (t *TcpConn) RequestPacket(index uint32) {
	msg := consts.NeedPacket + string(index)
	_, err := t.conn.Write([]byte(msg))
	if err != nil {
		log.Printf("Fail to request packet %d. Error: %s \n", index, err)
	} else {
		log.Printf("Told user to send the packet %d again.\n", index)
	}
}

func (t *TcpConn) Wait() string {
	message, err := bufio.NewReader(t.conn).ReadString('\n')
	if err != nil {
		log.Println(err)
	}

	return message
}

func (t *TcpConn) GetLocalInfo() string {
	return t.conn.LocalAddr().String()
}

func (t *TcpConn) Close() {
	if err := t.conn.Close(); err != nil {
		log.Println(err)
	}
}
