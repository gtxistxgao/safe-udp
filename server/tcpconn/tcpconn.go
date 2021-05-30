package tcpconn

import (
	"bufio"
	"encoding/json"
	"fmt"
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
	_, err := t.conn.Write([]byte(port + "\n"))
	if err != nil {
		log.Println("Fail to tell user the port. Error:", err)
	} else {
		log.Println("Told user the UCP port is", port)
	}
}

func (t *TcpConn) GetFileInfo() filemeta.FileMeta {
	log.Println("Waiting for file info")
	info, _ := t.Wait()
	log.Println("Raw file meta message", info)
	fileMeta := filemeta.FileMeta{}
	err := json.Unmarshal([]byte(info), &fileMeta)
	if err != nil {
		log.Println("Fail to parse file metadata, error: ", err)
	}

	return fileMeta
}

func (t *TcpConn) SendFinishSignal() {
	msg := consts.Finished
	_, err := t.conn.Write([]byte(msg + "\n"))
	if err != nil {
		log.Printf("Fail to send finish signal, Error: %s \n", err)
	} else {
		log.Printf("Told user we have finished.\n", )
	}
}

func (t *TcpConn) RequestPacket(index uint32) {
	msg := fmt.Sprintf("%s%d", consts.NeedPacket, index)
	_, err := t.conn.Write([]byte(msg + "\n"))
	if err != nil {
		log.Printf("Fail to request packet %d. Error: %s \n", index, err)
	} else {
		log.Printf("Told user to send the packet %d again. Msg: %s\n", index, msg)
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

func (t *TcpConn) GetLocalInfo() string {
	return t.conn.LocalAddr().String()
}

func (t *TcpConn) Close() {
	if err := t.conn.Close(); err != nil {
		log.Println(err)
	}
}
