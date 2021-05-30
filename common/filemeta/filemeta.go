package filemeta

import "fmt"

type FileMeta struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func (f *FileMeta) String() string {
	return fmt.Sprintf("File name: %s. File size %d", f.Name, f.Size)
}
