package user

import (
	"container/heap"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/gtxistxgao/safe-udp/common/consts"
	"github.com/gtxistxgao/safe-udp/common/filemeta"
	"github.com/gtxistxgao/safe-udp/common/model"
	"github.com/gtxistxgao/safe-udp/common/udp_server"
	"github.com/gtxistxgao/safe-udp/common/util"
	"github.com/gtxistxgao/safe-udp/server/tcpconn"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type User struct {
	ctx       context.Context
	cancel    context.CancelFunc
	userInfo  string
	tcpConn   *tcpconn.TcpConn
	udpServer *udp_server.UDPServer
	fileInfo  filemeta.FileMeta
	progress  uint32 // progress donate the next packet index we are expecting
}

func New(tcpConn net.Conn) *User {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	server, err := udp_server.New(":0", 1024)
	if err != nil {
		log.Fatal("start udp server hit error: ", err)
	}

	return &User{
		ctx:       ctx,
		cancel:    cancel,
		userInfo:  tcpConn.LocalAddr().String(),
		udpServer: server,
		tcpConn:   tcpconn.New(tcpConn),
		progress:  0, // TODO: recording last time and support resuming
	}
}

func (u *User) Start() {
	rawDataBufferCountLimit := (consts.MaxMemoryBufferMB * (1 << 20)) / consts.PayloadDataSizeByte
	rawData := make(chan []byte, rawDataBufferCountLimit)
	defer close(rawData)

	// sync with client about the file
	u.preSync()

	go u.serverWorker(u.ctx, rawData)

	processedData := make(chan *model.Chunk, rawDataBufferCountLimit)
	defer close(processedData)

	for i := 0; i < consts.RawDataWorkerNumber; i++ {
		go u.rawDataProcessWorker(u.ctx, rawData, processedData)
		log.Println(i, " rawDataProcessWorker started")
	}

	minHeapChunk := &model.MinHeapChunk{}
	heap.Init(minHeapChunk)

	signal := make(chan *bool, rawDataBufferCountLimit)

	go u.minHeapPushWorker(u.ctx, minHeapChunk, processedData, signal)
	log.Println("minHeapPushWorker started")

	dataToBeWritten := make(chan *model.Chunk, 1)
	go u.minHeapPollWorker(u.ctx, minHeapChunk, signal, dataToBeWritten)
	log.Println("minHeapPollWorker started")

	go u.saveToDiskWorker(u.ctx, dataToBeWritten, u.fileInfo.Name)
	log.Println("saveToDiskWorker started")

	u.tcpConn.SendPort("Server prepare ready\n") // tell client to start to send

	go u.sync()

	select {
	case <-u.ctx.Done():
		fmt.Printf("User %s finished task\n", u.userInfo)
	}

	fmt.Printf("User %s finished task\n", u.userInfo)
}

func (u *User) sync() {
	signal := make(chan *bool, 1)
	go func() {
		for {
			sig := <-signal
			if sig == nil {
				break
			}

			message, err := u.tcpConn.Wait()
			if err != nil {
				if strings.Contains(err.Error(), "use of closed network connection") {
					log.Printf("Connection closed. Stop sync")
					break
				}
			}

			log.Printf("Message received from User %s\n", message)
			u.triage(message)
			signal <- util.BoolPtr(true)
		}
	}()

	signal <- util.BoolPtr(true)

	select {
	case <-u.ctx.Done():
		for len(signal) > 0 {
			<-signal
		}

		signal <- nil
		log.Println("sync finished")
	}
}

func (u *User) triage(msg string) {
	switch msg {
	case "beat":
		log.Println("The user is still there.")
		break
	case consts.Validate:
		log.Println("User finished send all package. we need to do validation")
		u.validate()
		break
	case "EOF":
		log.Println("All Validation is done. Cleaning up")
		u.Close()
		break
	case "finish":
		log.Println("All Validation is done. Cleaning up")
		u.Close()
		break
	}
}

func (u *User) beat() {
	log.Printf("The user %s alive\n", u.tcpConn.GetLocalInfo())
}

func (u *User) validate() {
	totalPacketCount := uint32(u.fileInfo.Size / consts.PayloadDataSizeByte)
	if u.fileInfo.Size%consts.PayloadDataSizeByte != 0 {
		totalPacketCount++
	}

	finished := u.progress == totalPacketCount
	if finished {
		log.Printf("All required %d packets received\n", u.progress)
		// we can clean up  resources
		u.tcpConn.SendFinishSignal()
		time.Sleep(time.Second)
		u.Close()

	} else {
		// hay we are not finished yet. send me this packet again!
		u.tcpConn.RequestPacket(u.progress)
	}
}

func (u *User) preSync() {
	// Tell user which UDP port to send to
	port := u.udpServer.GetPort()
	log.Println("Tell client we are listening to this port", port)
	u.tcpConn.SendPort(port) // tell user which UDP port to sent file

	// Learn the file info
	u.fileInfo = u.tcpConn.GetFileInfo()
	log.Println("Got file info", u.fileInfo.String())
}

func (u *User) serverWorker(ctx context.Context, rawData chan []byte) {
	if err := u.udpServer.Run(ctx, rawData); err != nil {
		log.Fatal("Server Run hit error: ", err)
	}
}

func (u *User) Close() {
	u.tcpConn.Close()
	u.cancel()
}

func (u *User) rawDataProcessWorker(ctx context.Context, rawData chan []byte, processedData chan *model.Chunk) {
	go func() {
		for {
			data := <-rawData
			fmt.Println("Got raw data with size: ", len(data))
			if data == nil {
				break
			}

			sep := 0
			for data[sep] != ',' {
				sep++
			}

			indexStr := string(data[:sep])
			index, err := strconv.ParseUint(indexStr, 10, 32)
			if err != nil {
				log.Println("Parse string to index hit error. String: ", indexStr, " Error: ", err)
			}

			encodedData := data[sep+1:]
			unencodedData, err := base64.StdEncoding.DecodeString(string(encodedData))
			if err != nil {
				log.Println("Decode base64 hit error: ", err)
			}

			index32 := uint32(index)
			c := model.Chunk{
				Index: index32,
				Data:  unencodedData,
			}

			log.Printf("Successfully processed data chunk %d and pushed into processedDataQueue.\n", index32)

			processedData <- &c
		}
	}()

	select {
	case <-ctx.Done():
		for len(rawData) > 0 {
			<-rawData
		}

		rawData <- nil
		fmt.Println("rawDataProcessWorker cancelled")
	}
}

func (u *User) minHeapPushWorker(ctx context.Context, minHeapChunk *model.MinHeapChunk, processedData chan *model.Chunk, signal chan *bool) {
	go func(min *model.MinHeapChunk, signal chan *bool) {
		for {
			c := <-processedData
			if c == nil {
				fmt.Println("minHeapPushWorker routine finished")
				break
			}

			heap.Push(min, *c)
			fmt.Printf("pushed data chunk %d into min heap\n", c.Index)
			signal <- util.BoolPtr(true)
		}
	}(minHeapChunk, signal)

	select {
	case <-ctx.Done():
		for len(processedData) > 0 {
			<-processedData
		}

		processedData <- nil
		fmt.Println("minHeapPushWorker cancelled")
	}
}

func (u *User) minHeapPollWorker(ctx context.Context, minHeapChunk *model.MinHeapChunk, signal chan *bool, dataToBeWritten chan *model.Chunk) {
	go func(dataToBeWritten chan *model.Chunk) {
		for {
			s := <-signal
			if s == nil {
				fmt.Println("minHeapPollWorker routine finished")
				break
			}

			fmt.Println("Got signal! Check the current heap top! ")
			if minHeapChunk.IsEmpty() {
				fmt.Printf("Min heap top is empty! Ask User send chunk %d. \n", u.progress)
				u.tcpConn.RequestPacket(u.progress)
				continue
			}

			// remove duplicate package that we already processed
			for top := minHeapChunk.Peek(); top.Index < u.progress; top = minHeapChunk.Peek() {
				log.Println("Drop chunk with index: ", top.Index)
				heap.Pop(minHeapChunk)
			}

			topIndex := minHeapChunk.Peek().Index
			if topIndex > u.progress {
				log.Printf("Expect index %d, but top package %d.\n", u.progress, topIndex)
				u.tcpConn.RequestPacket(u.progress)
				fmt.Printf("Requested User send chunk %d. \n", u.progress)
				continue
			}

			topOne := heap.Pop(minHeapChunk).(model.Chunk)
			dataToBeWritten <- &topOne
			u.progress++
		}
	}(dataToBeWritten)

	select {
	case <-ctx.Done():
		for len(signal) > 0 {
			<-signal
		}

		signal <- nil
		fmt.Println("minHeapPollWorker cancelled")
	}
}

func (u *User) saveToDiskWorker(ctx context.Context, dataToBeWritten chan *model.Chunk, filePath string) {
	go func(dataToBeWritten chan *model.Chunk) {
		file, err := os.Create(filePath)
		if err != nil {
			log.Fatal(err)
		}

		defer func() {
			if err := file.Close(); err != nil {
				log.Println("Close file failed: ", err)
			}
		}()

		for {
			chunk := <-dataToBeWritten
			if chunk == nil {
				fmt.Println("minHeapPollWorker routine finished")
				break
			}

			if n, writeErr := file.Write(chunk.Data); writeErr != nil {
				u.tcpConn.RequestPacket(chunk.Index)
				log.Printf("Fail to write index %d to disk. Ask user send it again. Error: %s", chunk.Index, writeErr)
				continue
			} else {
				log.Printf("Write %d bytes data into disk\n", n)

			}
		}
	}(dataToBeWritten)

	select {
	case <-ctx.Done():
		for len(dataToBeWritten) > 0 {
			<-dataToBeWritten
		}

		dataToBeWritten <- nil
		fmt.Println("saveToDiskWorker cancelled")
	}
}

//func jumpWriteWorker(ctx context.Context, processedData chan *model.Chunk, filePath string) {
//	go func(processedData chan *model.Chunk) {
//		file, err := os.Create(filePath)
//		if err != nil {
//			log.Fatal(err)
//		}
//
//		defer func() {
//			if err := file.Close(); err != nil {
//				log.Println("Close file failed: ", err)
//			}
//		}()
//
//		for {
//			data := <-processedData
//			if data == nil {
//				fmt.Println("jumpWriteWorker routine finished")
//				break
//			}
//
//			offset := int64(consts.PayloadDataSize * data.Index)
//			if n, writeErr := file.WriteAt(data.Data, offset); writeErr != nil {
//				log.Println("Write data to disk error: ", writeErr)
//				break
//			} else {
//				log.Printf("Chunk Index %d, %d bytes saved.\n", data.Index, n)
//			}
//		}
//	}(processedData)
//
//	select {
//	case <-ctx.Done():
//		for len(processedData) > 0 {
//			<-processedData
//		}
//
//		processedData <- nil
//		fmt.Println("jumpWriteWorker cancelled")
//	}
//}
