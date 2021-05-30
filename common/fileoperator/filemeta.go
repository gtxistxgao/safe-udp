package fileoperator

import "fmt"

type FileMeta struct {
	Name             string `json:"name"`
	Size             int64  `json:"size"`
	TotalPacketCount uint32  `json:"totalPacketCount"`
}

func (f *FileMeta) String() string {
	return fmt.Sprintf("File name: %s. File size %d. Total packet count %d", f.Name, f.Size, f.TotalPacketCount)
}
