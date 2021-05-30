package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gtxistxgao/safe-udp/client/tcpconn"
	"github.com/gtxistxgao/safe-udp/common/consts"
	"github.com/gtxistxgao/safe-udp/common/fileoperator"
	"github.com/gtxistxgao/safe-udp/common/toggle"
	"github.com/gtxistxgao/safe-udp/common/udp_client"
	"github.com/gtxistxgao/safe-udp/common/util"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

var fileName string = "book.pdf"

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	log.SetFlags(log.Lshortfile | log.LstdFlags)

	start := time.Now()
	c := NewClient(ctx, cancel)
	c.Run()

	elapsed := time.Since(start)
	log.Println("File info", c.GetFileMeta())
	log.Println("File sent. cost", elapsed)
}

type Client struct {
	ctx        context.Context
	tcpConn    *tcpconn.TcpConn
	cancel     context.CancelFunc
	fileReader *fileoperator.Reader
	udpClient  *udp_client.UDPClient
}

func NewClient(ctx context.Context, cancel context.CancelFunc) *Client {
	// 1. setup TCP connection
	log.Println("Start to dial server")
	tcpConn, err := net.Dial("tcp", ":8888")
	if err != nil {
		log.Fatal(err)
	}

	// 2. create UDP client
	udpPort, err := getUcpDstPort(tcpConn)
	if err != nil {
		log.Fatal("Fail to send file meta data, error:", err)
	}

	udpClient := udp_client.New(ctx, "localhost:"+udpPort, time.Second*2)
	log.Println("UDP buffer value is:", udpClient.GetBufferValue())

	// 3. exchange File metadata
	fileReader := fileoperator.NewReader(fileName)

	fileMeta, err := json.Marshal(&fileReader.FileMeta)
	if err != nil {
		log.Fatal("Fail to parse file meta data, error:", err)
	}
	log.Println("Send file info", string(fileMeta))
	if _, err := tcpConn.Write(append(fileMeta, '\n')); err != nil {
		log.Fatal("Fail to send file meta data, error:", err)
	}

	// 4. ACK and ready to start
	ACK, _ := bufio.NewReader(tcpConn).ReadString('\n')
	log.Println(ACK)

	return &Client{
		ctx:        ctx,
		tcpConn:    tcpconn.New(tcpConn),
		cancel:     cancel,
		fileReader: fileReader,
		udpClient:  udpClient,
	}
}

func (c *Client) GetFileMeta() string {
	return c.fileReader.FileMeta.String()
}

func (c *Client) Close() {
	log.Println("Start to close client")
	c.tcpConn.Close()
	log.Println("TCP Connection closed")
	c.udpClient.Close()
	log.Println("UDP Connection closed")
	c.fileReader.Close()
	log.Println("File reader closed")
	c.cancel()
	log.Println("Context closed")
}

func (c *Client) Run() {
	if toggle.MultiThreadEmit {
		c.multiThreadEmit()
	} else {
		c.singleThreadEmit()
	}
}

func (c *Client) multiThreadEmit() {
	indexChan := make(chan *uint32, 1)
	log.Println("indexChan limit", 1)

	go c.readAndEmitWorker(indexChan)
	log.Println("readAndEmitWorker started.")
	go c.feedbackWorker(indexChan)
	log.Println("feedbackWorker started.")

	if err := c.tcpConn.RequestValidation(); err != nil {
		log.Println("RequestValidation failed. ", err)
	}

	select {
	case <-c.ctx.Done():
		log.Println("full cycle done cancelled")
	}
}

func (c *Client) feedbackWorker(indexChan chan *uint32) {
	go func() {
		for {
			signal, err := c.tcpConn.Wait()
			if err != nil {
				log.Println(err)
				if strings.Contains(err.Error(), "use of closed network connection") || err == io.EOF {
					c.Close()
					break
				}
				continue
			}

			log.Println("Server is asking", signal)

			if strings.HasPrefix(signal, consts.Finished) {
				log.Println("Finished, cancel context")
				c.Close()
				log.Println("Fully cancelled")
				break
			}

			if strings.HasPrefix(signal, consts.NeedPacket) {
				index := extractPacketIndex(signal)

				log.Printf("User is requesting chunk of %d/%d", index, c.fileReader.FileMeta.TotalPacketCount-1)

				for walker := index; walker < c.fileReader.FileMeta.TotalPacketCount && walker < index+consts.PacketCountPerRound; walker++ {
					log.Println("Push index", walker, "into channel")
					indexChan <- util.Uint32Ptr(walker)
					//time.Sleep(time.Microsecond)
				}

				log.Println("Asking server do validation")
				if err := c.tcpConn.RequestValidation(); err != nil {
					log.Println("RequestValidation failed. ", err)
				}
			}
		}
	}()

	select {
	case <-c.ctx.Done():
		for len(indexChan) > 0 {
			<-indexChan
		}

		indexChan <- nil
		log.Println("feedbackWorker cancelled")
	}
}

func extractPacketIndex(msg string) uint32 {
	packetIndexStr := strings.Trim(msg, consts.NeedPacket)
	index, err := strconv.ParseUint(packetIndexStr, 10, 32)
	if err != nil {
		log.Println(err)
	}

	return uint32(index)
}

func (c *Client) readAndEmitWorker(indexChan chan *uint32) {
	go func() {
		for {
			index := <-indexChan
			if index == nil {
				break
			}

			indexVal := *index
			log.Printf("start to read chunk %d/%d\n", indexVal, c.fileReader.FileMeta.TotalPacketCount-1)
			offset := indexVal * consts.PayloadDataSizeByte
			log.Printf("Read index %d with offset %d.\n", indexVal, offset)
			bytesread := c.fileReader.ReadAt(int64(offset))
			payload := buildPayLoad(bytesread, indexVal)
			err := c.udpClient.SendAsync(c.ctx, payload)
			log.Printf("Chunk %d of size %d sent\n", indexVal, len(bytesread))
			if err != nil {
				fmt.Print(err)
				break
			}
		}
	}()

	select {
	case <-c.ctx.Done():
		for len(indexChan) > 0 {
			<-indexChan
		}

		indexChan <- nil
		log.Println("readAndEmitWorker cancelled")
	}
}

func (c *Client) singleThreadEmit() {
	progress := fmt.Sprintf("%s%d", consts.NeedPacket, 0)
	for strings.HasPrefix(progress, consts.NeedPacket) {
		strArr := strings.Split(progress, ":")
		index, err := strconv.ParseUint(strArr[1], 10, 32)
		if err != nil {
			log.Printf("Fail to parse progress index value. %s. Error: %s. \n", progress, err)
			index = 0 // if cannot find, then start by 0 index
		}

		if toggle.SerialRead {
			serialReadAndEmit(c.ctx, c.fileReader.File, c.udpClient, uint32(index))
		} else {
			c.skipReadAndEmit(c.ctx, c.udpClient, uint32(index), c.fileReader.FileMeta.Size)
		}

		log.Println("Asking server do validation")
		if err := c.tcpConn.RequestValidation(); err != nil {
			log.Println("RequestValidation failed. ", err)
		}

		progress, err = c.tcpConn.Wait()
		if err != nil {
			log.Println("Fail to get validation result ", err)
			progress = fmt.Sprintf("%s%d", consts.NeedPacket, 0)
		}
	}
}

func serialReadAndEmit(ctx context.Context, file *os.File, client *udp_client.UDPClient, start uint32) {
	index := start
	bufferSize := consts.PayloadDataSizeByte
	buffer := make([]byte, bufferSize)
	for {
		bytesread, err := file.Read(buffer)
		log.Println("Bytes read: ", bytesread)
		if err != nil {
			log.Println("Finished reading")
			log.Println(err)
			return
		}

		encoded := base64.StdEncoding.EncodeToString(buffer[:bytesread])
		payload := fmt.Sprintf("%d,%s", index, encoded)
		err = client.SendAsync(ctx, []byte(payload))
		fmt.Printf("Chunk %d sent\n", index)
		if err != nil {
			fmt.Print(err)
		}

		index++
	}
}

func (c *Client) skipReadAndEmit(ctx context.Context, udpClient *udp_client.UDPClient, index uint32, filesize int64) {
	for ; int64(index) < filesize; index++ {
		bytesread := c.fileReader.ReadAt(int64(index) * consts.PayloadDataSizeByte)
		payload := buildPayLoad(bytesread, index)
		err := udpClient.SendAsync(ctx, payload)
		log.Printf("Chunk %d of size %d sent\n", index, bytesread)
		if err != nil {
			fmt.Print(err)
		}
	}
}

func getUcpDstPort(tcpConn net.Conn) (string, error) {
	udpPort, err := bufio.NewReader(tcpConn).ReadString('\n')
	if err != nil {
		return "", err
	}

	if len(udpPort) == 0 {
		return "", fmt.Errorf("Invalid udp port value. Size == 0.")
	}

	log.Printf("Got UDP port %s", udpPort)
	return udpPort[:len(udpPort)-1], nil
}

func buildPayLoad(chunk []byte, index uint32) []byte {
	encoded := base64.StdEncoding.EncodeToString(chunk)
	payload := fmt.Sprintf("%d,%s", index, encoded)
	return []byte(payload)
}
