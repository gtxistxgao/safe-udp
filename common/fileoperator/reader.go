package fileoperator

import (
	"fmt"
	"github.com/gtxistxgao/safe-udp/common/consts"
	"log"
	"os"
)

type Reader struct {
	File     *os.File
	buffer   []byte
	FileMeta FileMeta
}

func NewReader(filePath string) *Reader {
	log.Println("Start to open file")
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatal(err)
	}

	fileinfo, err := file.Stat()
	if err != nil {
		log.Fatal(err)
	}

	totalPacketCount := uint32(fileinfo.Size() / consts.PayloadDataSizeByte)
	if fileinfo.Size()%consts.PayloadDataSizeByte != 0 {
		totalPacketCount++
	}

	fileMetaObject := FileMeta{
		Name:             fileinfo.Name(),
		Size:             fileinfo.Size(),
		TotalPacketCount: totalPacketCount,
	}

	buffer := make([]byte, consts.PayloadDataSizeByte)

	return &Reader{
		File:     file,
		buffer:   buffer,
		FileMeta: fileMetaObject,
	}
}

// TODO: read the data in a fixed size and cache it in the memory. Reduce disk I/O cost
func (r *Reader) ReadAt(offset int64) []byte {
	n, err := r.File.ReadAt(r.buffer, offset)
	fmt.Printf("Read offset %d, %d bytes\n", offset, n)
	if err != nil {
		log.Println(err)
	}

	rst := make([]byte, n)
	copy(rst, r.buffer[:n])
	return rst
}

func (r *Reader) Close() {
	r.File.Close()
}
