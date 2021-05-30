package toggle

var SerialWrite bool = true
var SerialRead bool = false
var MultiThreadEmit bool = true

type Mode string

var (
	FireAndForget Mode = "FireAndForget" // client will emit the packet out and server won't respond success or not
	FireAndSync   Mode = "FireAndSync"   // client will emit 1 packet out, server respond ACK to client, then client will send the next packet.
	ServerAsk     Mode = "ServerAsk"     // Server will ask for specific packet from client
)
