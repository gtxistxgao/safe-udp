package util

func BoolPtr(val bool) *bool {
	return &val
}


func Uint32Ptr(val uint32) *uint32 {
	copy := val
	return &copy
}