package model

type Chunk struct {
	Index uint32 // 500 per chunk and max uint32 is 2^32. So it can support at most 1.95 TB file
	Data  []byte
}

// MinHeapChunk is a min heap, the top Chunk will be the one with smallest index
type MinHeapChunk []Chunk

func (c *MinHeapChunk) Len() int {
	return len(*c)
}

func (c *MinHeapChunk) Less(i, j int) bool {
	return (*c)[i].Index < (*c)[j].Index
}

func (c *MinHeapChunk) Swap(i, j int) {
	(*c)[i], (*c)[j] = (*c)[j], (*c)[i]
}

func (c *MinHeapChunk) Push(item interface{}) {
	*c = append(*c, item.(Chunk))
}

func (c *MinHeapChunk) Pop() interface{} {
	l := len(*c)
	rst := (*c)[l-1]
	*c = (*c)[:l-1]
	return rst
}

func (c *MinHeapChunk) Peek() Chunk {
	return (*c)[0]
}

func (c *MinHeapChunk) IsEmpty() bool {
	return len(*c) == 0
}
