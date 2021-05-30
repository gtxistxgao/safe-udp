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
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

var cancelFunc context.CancelFunc

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	cancelFunc = cancel

	fmt.Println("Start to open file")
	start := time.Now()
	file, err := os.Open("big.txt")
	if err != nil {
		fmt.Print(err)
	}

	defer file.Close()

	fileinfo, err := file.Stat()
	if err != nil {
		fmt.Println(err)
		return
	}

	filesize := fileinfo.Size()
	fmt.Println("File size: ", filesize)

	log.Println("Start to dial server")
	tcpConn, err := net.Dial("tcp", ":8888")
	if err != nil {
		log.Fatal(err)
	}

	defer tcpConn.Close()

	udpPort, err := getUcpDstPort(tcpConn)
	if err != nil {
		log.Fatal("Fail to send file meta data, error:", err)
	}

	udpClient := udp_client.New(ctx, "localhost:"+udpPort, 500, time.Second*2)
	defer udpClient.Close()

	log.Println("UDP buffer value is:", udpClient.GetBufferValue())

	fileMeta, err := json.Marshal(&filemeta.FileMeta{
		Name: fileinfo.Name(),
		Size: fileinfo.Size(),
	})
	if err != nil {
		log.Fatal("Fail to send file meta data, error:", err)
	}

	log.Println("Send file info", string(fileMeta))

	if _, err := tcpConn.Write(append(fileMeta, '\n')); err != nil {
		log.Fatal("Fail to send file meta data, error:", err)
	}

	ACK, _ := bufio.NewReader(tcpConn).ReadString('\n')
	log.Println(ACK)

	if toggle.MultiThreadEmit {
		multiThreadEmit(ctx, file, udpClient, filesize, tcpConn)
	} else {
		singleThreadEmit(ctx, file, udpClient, filesize, tcpConn)
	}

	elapsed := time.Since(start)
	fmt.Println("Finished. cost ", elapsed)
}

func multiThreadEmit(ctx context.Context, file *os.File, udpClient *udp_client.UDPClient, filesize int64, tcpConn net.Conn) {
	indexChan := make(chan *uint32, 1)
	log.Println("indexChan limit", 1)

	go readAndEmitWorker(ctx, file, udpClient, indexChan)
	log.Println("readAndEmitWorker started.")
	go feedbackWorker(ctx, tcpConn, indexChan)
	log.Println("feedbackWorker started.")

	totalPacketCount := uint32(filesize / consts.PayloadDataSizeByte)
	if filesize%consts.PayloadDataSizeByte != 0 {
		totalPacketCount++
	}

	log.Println("Total packet count", totalPacketCount)
	var index uint32 = 0

	for ; index < totalPacketCount; index++ {
		log.Println("Push index", index, "into channel")
		indexChan <- &index
		time.Sleep(time.Millisecond)
	}

	n, err := tcpConn.Write([]byte(consts.Validate + "\n"))
	if err != nil {
		log.Println("Fail to ask server to validate", err)
	} else {
		log.Println("Asked server to validate", n, "bytes")
	}

	select {
	case <-ctx.Done():
		fmt.Println("full cycle done cancelled")
	}
}

func feedbackWorker(ctx context.Context, tcpConn net.Conn, indexChan chan *uint32) {
	go func() {
		for {
			signal, err := bufio.NewReader(tcpConn).ReadString('\n')
			if err != nil {
				log.Println(err)
				continue
			}

			log.Println("Server is asking", signal)

			if strings.HasPrefix(signal, consts.Finished) {
				log.Println("Finished, so we can cancel context")
				//time.Sleep(time.Second)
				cancelFunc()
				break
			}

			if strings.HasPrefix(signal, consts.NeedPacket) {
				index := extractPacketIndex(signal)
				indexChan <- &index
			}
		}
	}()

	select {
	case <-ctx.Done():
		for len(indexChan) > 0 {
			<-indexChan
		}

		indexChan <- nil
		fmt.Println("feedbackWorker cancelled")
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

func readAndEmitWorker(ctx context.Context, file *os.File, udpClient *udp_client.UDPClient, indexChan chan *uint32) {
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
			fmt.Printf("Read index %d, %d bytes\n", indexVal, bytesread)
			if err != nil {
				fmt.Println(err)
				if err == io.EOF {
					payload := buildPayLoad(buffer, bytesread, indexVal)
					err = udpClient.SendAsync(ctx, payload)
					continue
				}
			}

			payload := buildPayLoad(buffer, bytesread, indexVal)
			err = udpClient.SendAsync(ctx, payload)
			fmt.Printf("Chunk %d of size %d sent\n", indexVal, bytesread)
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
		fmt.Println("readAndEmitWorker cancelled")
	}
}

func singleThreadEmit(ctx context.Context, file *os.File, udpClient *udp_client.UDPClient, filesize int64, tcpConn net.Conn) {
	progress := fmt.Sprintf("%s%d", consts.NeedPacket, 0)
	for strings.HasPrefix(progress, consts.NeedPacket) {
		strArr := strings.Split(progress, ":")
		index, err := strconv.ParseUint(strArr[1], 10, 32)
		if err != nil {
			log.Printf("Fail to parse progress index value. %s. Error: %s. \n", progress, err)
			index = 0 // if cannot find, then start by 0 index
		}

		if toggle.SerialRead {
			serialReadAndEmit(ctx, file, udpClient, uint32(index))
		} else {
			skipReadAndEmit(ctx, file, udpClient, uint32(index), filesize)
		}

		// Ask server validate the progress
		if _, err := tcpConn.Write([]byte("validate")); err != nil {
			log.Fatal(err)
		}

		progress, err = bufio.NewReader(tcpConn).ReadString('\n')
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
		fmt.Println("Bytes read: ", bytesread)
		if err != nil {
			fmt.Println("Finished reading")
			fmt.Println(err)
			return
		}

		encoded := base64.StdEncoding.EncodeToString(buffer[:bytesread])
		payload := fmt.Sprintf("%d,%s", index, encoded)
		err = client.SendAsync(ctx, []byte(payload))
		fmt.Printf("Chunk %d sent", index)
		fmt.Println()
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
			fmt.Println(err)
		}

		payload := buildPayLoad(buffer, bytesread, index)
		err = udpClient.SendAsync(ctx, payload)
		fmt.Printf("Chunk %d of size %d sent", index, bytesread)
		fmt.Println()
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
