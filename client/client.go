package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/gtxistxgao/safe-udp/common/consts"
	"github.com/gtxistxgao/safe-udp/common/toggle"
	"github.com/gtxistxgao/safe-udp/common/udp_client"
	"os"
	"time"
)

func main() {
	ctx := context.Background()
	client := udp_client.New(ctx, "localhost:4567", 500, time.Second*2)
	defer client.Close()

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

	if toggle.SerialRead {
		serialReadAndEmit(ctx, file, client)
	} else {
		bufferSize := consts.PayloadDataSize
		buffer := make([]byte, bufferSize)
		var index int64 = 0
		for ; index < filesize; index += consts.PayloadDataSize {
			bytesread, err := file.ReadAt(buffer, index)
			fmt.Printf("Read index %d, %d bytes\n", index, bytesread)
			if err != nil {
				fmt.Println("Finished reading")
				fmt.Println(err)
				return
			}

			payload := buildPayLoad(buffer, bytesread, index)
			err = client.SendAsync(ctx, payload)
			fmt.Printf("Chunk %d of size %d sent", index, bytesread)
			fmt.Println()
			if err != nil {
				fmt.Print(err)
			}
		}
	}

	elapsed := time.Since(start)
	fmt.Println("Finished. cost ", elapsed)
}

func buildPayLoad(buffer []byte, bytesread int, index int64) []byte {
	encoded := base64.StdEncoding.EncodeToString(buffer[:bytesread])
	payload := fmt.Sprintf("%d,%s", index, encoded)
	return []byte(payload)
}

func serialReadAndEmit(ctx context.Context, file *os.File, client *udp_client.UDPClient) {
	var index uint32 = 0
	bufferSize := consts.PayloadDataSize
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
