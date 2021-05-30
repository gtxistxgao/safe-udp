package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gtxistxgao/safe-udp/common/consts"
	"github.com/gtxistxgao/safe-udp/common/filemeta"
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

var fileName string = "small.txt"

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	log.SetFlags(log.Lshortfile | log.LstdFlags)

	start := time.Now()
	c := NewClient(ctx, cancel)
	c.Run()

	elapsed := time.Since(start)
	log.Println("Finished. cost ", elapsed)
}

type Client struct {
	ctx       context.Context
	tcpConn   net.Conn
	cancel    context.CancelFunc
	file      *os.File
	fileMeta  filemeta.FileMeta
	udpClient *udp_client.UDPClient
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

	udpClient := udp_client.New(ctx, "localhost:"+udpPort, 500, time.Second*2)
	log.Println("UDP buffer value is:", udpClient.GetBufferValue())

	// 3. exchange File metadata
	log.Println("Start to open file")
	file, err := os.Open(fileName)
	if err != nil {
		log.Fatal(err)
	}

	fileinfo, err := file.Stat()
	if err != nil {
		log.Fatal(err)
	}

	fileMetaObject := filemeta.FileMeta{
		Name: fileinfo.Name(),
		Size: fileinfo.Size(),
	}

	fileMeta, err := json.Marshal(&fileMetaObject)
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
		ctx:       ctx,
		tcpConn:   tcpConn,
		cancel:    cancel,
		file:      file,
		fileMeta:  fileMetaObject,
		udpClient: udpClient,
	}
}

func (c *Client) Close() {
	log.Println("Start to close client")
	c.tcpConn.Close()
	log.Println("TCP Connection closed")
	c.udpClient.Close()
	log.Println("UDP Connection closed")
	c.file.Close()
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

	go c.readAndEmitWorker(c.ctx, c.file, c.udpClient, indexChan)
	log.Println("readAndEmitWorker started.")
	go c.feedbackWorker(c.ctx, c.tcpConn, indexChan, c.fileMeta.Size)
	log.Println("feedbackWorker started.")

	c.askServerDoValidation(c.tcpConn)

	select {
	case <-c.ctx.Done():
		log.Println("full cycle done cancelled")
	}
}

func (c *Client) askServerDoValidation(tcpConn net.Conn) {
	n, err := tcpConn.Write([]byte(consts.Validate + "\n"))
	if err != nil {
		log.Println("Fail to ask server to validate", err)
	} else {
		log.Println("Asked server to validate", n, "bytes")
	}
}

func (c *Client) feedbackWorker(ctx context.Context, tcpConn net.Conn, indexChan chan *uint32, filesize int64) {
	go func() {
		for {
			signal, err := bufio.NewReader(tcpConn).ReadString('\n')
			if err != nil {
				log.Println(err)
				if strings.Contains(err.Error(), "use of closed network connection") || err == io.EOF {
					c.Close()
					break
				}
				continue
			}

			signal = signal[:len(signal)-1]

			log.Println("Server is asking", signal)

			if strings.HasPrefix(signal, consts.Finished) {
				log.Println("Finished, cancel context")
				c.Close()
				log.Println("Fully cancelled")
				break
			}

			if strings.HasPrefix(signal, consts.NeedPacket) {
				index := extractPacketIndex(signal)

				log.Println("User is requesting chunk of index", index)

				totalPacketCount := uint32(filesize / consts.PayloadDataSizeByte)
				if filesize%consts.PayloadDataSizeByte != 0 {
					totalPacketCount++
				}

				for walker := index; walker < totalPacketCount && walker < index+10000; walker++ {
					log.Println("Push index", walker, "into channel")
					indexChan <- util.Uint32Ptr(walker)
				}
			}
		}
	}()

	select {
	case <-ctx.Done():
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

func (c *Client) readAndEmitWorker(ctx context.Context, file *os.File, udpClient *udp_client.UDPClient, indexChan chan *uint32) {
	go func() {
		for {
			index := <-indexChan
			if index == nil {
				break
			}

			indexVal := *index
			log.Println("start to read chunk with index", indexVal)
			buffer := make([]byte, consts.PayloadDataSizeByte)
			offset := indexVal * consts.PayloadDataSizeByte
			log.Println("file read offset", offset)
			bytesread, err := file.ReadAt(buffer, int64(offset))
			log.Printf("Read index %d, %d bytes\n", indexVal, bytesread)
			if err != nil {
				log.Println(err)
				if err == io.EOF {
					payload := buildPayLoad(buffer, bytesread, indexVal)
					err = udpClient.SendAsync(ctx, payload)
					time.Sleep(time.Millisecond * 100)
					c.Close()
					continue
				}
			}

			payload := buildPayLoad(buffer, bytesread, indexVal)
			err = udpClient.SendAsync(ctx, payload)
			log.Printf("Chunk %d of size %d sent\n", indexVal, bytesread)
			if err != nil {
				fmt.Print(err)
				break
			}
		}
	}()

	select {
	case <-ctx.Done():
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
			serialReadAndEmit(c.ctx, c.file, c.udpClient, uint32(index))
		} else {
			skipReadAndEmit(c.ctx, c.file, c.udpClient, uint32(index), c.fileMeta.Size)
		}

		// Ask server validate the progress
		if _, err := c.tcpConn.Write([]byte("validate")); err != nil {
			log.Fatal(err)
		}

		progress, err = bufio.NewReader(c.tcpConn).ReadString('\n')
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

func skipReadAndEmit(ctx context.Context, file *os.File, udpClient *udp_client.UDPClient, index uint32, filesize int64) {
	buffer := make([]byte, consts.PayloadDataSizeByte)
	for ; int64(index) < filesize; index++ {
		bytesread, err := file.ReadAt(buffer, int64(index)*consts.PayloadDataSizeByte)
		fmt.Printf("Read index %d, %d bytes\n", index, bytesread)
		if err != nil {
			log.Println(err)
		}

		payload := buildPayLoad(buffer, bytesread, index)
		err = udpClient.SendAsync(ctx, payload)
		fmt.Printf("Chunk %d of size %d sent\n", index, bytesread)
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

func buildPayLoad(buffer []byte, bytesread int, index uint32) []byte {
	encoded := base64.StdEncoding.EncodeToString(buffer[:bytesread])
	payload := fmt.Sprintf("%d,%s", index, encoded)
	return []byte(payload)
}
