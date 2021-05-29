package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/gtxistxgao/safe-udp/common/udp_client"
	"os"
	"time"
)

func main() {
	ctx := context.Background()
	client := udp_client.New(ctx, "localhost:4567", 500, time.Second*2)
	defer client.Close()

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

	bufferSize := 250
	buffer := make([]byte, bufferSize)

	var index uint32 = 0

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
