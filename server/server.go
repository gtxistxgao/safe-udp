package main

import (
	"container/heap"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/gtxistxgao/safe-udp/common/model"
	"github.com/gtxistxgao/safe-udp/common/udp_server"
	"github.com/gtxistxgao/safe-udp/common/util"
	"log"
	"os"
	"strconv"
)

/*

every package received from the port will be send to channel 1

channel 1: received package

10 workers will process the package, put it into struct and push to channel 2

channel 2: processed received package into object

1 worker will get the object and put them into a min heap

1 worker will get the object:
  - if top one is the one we want
    - pop it
    - write file
    - TODO: send message to client "Hay I got it and you can ditch that package from your memory now"
  - TODO:if top one is not the one we want, wait.
    - TODO:if after 2 second, still not get the package, send notice to client asking for that package
      - TODO:send msg to channel 3

TODO: 1 worker will check channel 3 and send package to client for the missing packet

*/

const DISK_WRITE_SPEED_MB = 25
const PAYLOAD_DATA_SIZE = 250

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	rawDataBufferCountLimit := DISK_WRITE_SPEED_MB * (1 << 20) / PAYLOAD_DATA_SIZE

	rawData := make(chan []byte, rawDataBufferCountLimit)
	defer close(rawData)

	go serverWorker(ctx, rawData)

	processedData := make(chan *model.Chunk, rawDataBufferCountLimit)
	defer close(processedData)

	for i := 0; i < 10; i++ {
		go rawDataProcessWorker(ctx, rawData, processedData)
		log.Println(i, " rawDataProcessWorker started")
	}

	minHeapChunk := &model.MinHeapChunk{}
	heap.Init(minHeapChunk)

	signal := make(chan *bool, rawDataBufferCountLimit)

	go minHeapPushWorker(ctx, minHeapChunk, processedData, signal)
	log.Println("minHeapPushWorker started")

	dataToBeWritten := make(chan []byte, 1)
	go minHeapPollWorker(ctx, minHeapChunk, signal, dataToBeWritten)
	log.Println("minHeapPollWorker started")

	go saveToDiskWorker(ctx, dataToBeWritten, "serversaved.txt")
	log.Println("saveToDiskWorker started")


	select {
	case <-ctx.Done():
		fmt.Println("full process cancelled")
	}
}

func serverWorker(ctx context.Context, rawData chan []byte) {
	server, err := udp_server.New(":4567", 1024)
	if err != nil {
		log.Fatal("start udp server hit error: ", err)
	}

	if err := server.Run(ctx, rawData); err != nil {
		log.Fatal("Server Run hit error: ", err)
	}
}

func rawDataProcessWorker(ctx context.Context, rawData chan []byte, processedData chan *model.Chunk) {
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

func minHeapPushWorker(ctx context.Context, minHeapChunk *model.MinHeapChunk, processedData chan *model.Chunk, signal chan *bool) {
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

func minHeapPollWorker(ctx context.Context, minHeapChunk *model.MinHeapChunk, signal chan *bool, dataToBeWritten chan []byte) {
	var startAt uint32 = 0
	go func(indexToWait uint32, dataToBeWritten chan []byte) {
		for {
			s := <-signal
			if s == nil {
				fmt.Println("minHeapPollWorker routine finished")
				break
			}

			fmt.Println("Got signal! Check the current heap top! ")
			// remove duplicate package that we already processed
			for top := minHeapChunk.Peek(); top.Index < indexToWait; top = minHeapChunk.Peek() {
				log.Println("Drop chunk with index: ", top.Index)
				heap.Pop(minHeapChunk)
			}

			if minHeapChunk.IsEmpty() {
				fmt.Println("Min heap top is empty! Try Again")
				tryAgain := true
				signal <- &tryAgain
				continue
			}

			topIndex := minHeapChunk.Peek().Index
			if topIndex != indexToWait {
				log.Printf("Expect index %d, but top package %d. Need to try again\n", indexToWait, topIndex)
				tryAgain := true
				signal <- &tryAgain
				continue
			}

			dataToBeWritten <- heap.Pop(minHeapChunk).(model.Chunk).Data
			indexToWait++
		}
	}(startAt, dataToBeWritten)

	select {
	case <-ctx.Done():
		for len(signal) > 0 {
			<-signal
		}

		signal <- nil
		fmt.Println("minHeapPollWorker cancelled")
	}
}

func saveToDiskWorker(ctx context.Context, dataToBeWritten chan []byte, filePath string) {
	go func(dataToBeWritten chan []byte) {
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
			data := <-dataToBeWritten
			if data == nil {
				fmt.Println("minHeapPollWorker routine finished")
				break
			}

			if n, writeErr := file.Write(data); writeErr != nil {
				log.Println("Write data to disk error: ", writeErr)
				break
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
