# safe-udp
Multithread UDP large file transfer server implementation with data encryption

### 2021-05-30

The process works as follow:

- Server start to listen to TCP port 8888
- Client init a connection with server on port 8888
- Server open a new UDP port for datapath
- Server told client which UDP port to send file
- Client told server file name and file size
- Server start to listen to the UDP port
- Run
  - Client side
    - Create a channel indexChan for data chunk
    - 1 go routine to read from the channel indexChan and emit out the UDP packet
    - 1 go routine listen to the tcp connection for communication with server
      - if get "NeedPacket:1234", push 1234 into channel indexChan to let client resend the packet
      - if get "Finished", cancel the context
    - based on file size and packet size, calculate total packet count
    - for loop to push packet index into channel from 0 to end total packet count - 1
  - Server side
    - 1 go routine userChan
    - 1 go routine ready
    - 1 go routine listen to the tcp port
      - if new connection setup, create user and push it to userChan
    - 1 go routine listen to the userChan channel
      - if no signal from ready wait
      - if ready and new connection setup
        - execute user chan
        - finished user, push 1 signal to ready channel

- User workflow
  - every package received from the port will be send to channel 1
  - channel 1: received package
  - 10 workers will process the package, put it into struct and push to channel 2
  - channel 2: processed received package into object
  - 1 worker will get the object and put them into a min heap
  - 1 worker will get the object from min heap:
    - if top one is the one we want
      - pop it
      - write to channel 3
    - if top one is not the one we want, wait.
      - send notice to client asking for that package
  - 1 worker will listen to channel 3
    - hold if no message from channel 3
    - if new message
      - poll it out
      - save into disk
        - if disk save failed -> ask for resend
  - 1 worker will listen to TCP
    - if received "validation", we will do validation
      - if validation passed
        - Told client all packets received
        - Env clean up like cancel context

# Server log

```
╰─ go run server.go                                                                                                                                                                                                                                 ─╯
Start to listen to:  [::]:58001
2021/05/30 14:11:18 controller.go:67: New user joined
2021/05/30 14:11:18 controller.go:90: User started
2021/05/30 14:11:18 udp_server.go:41: Get local address [::]:58001
2021/05/30 14:11:18 user.go:179: Tell client we are listening to this port 58001
2021/05/30 14:11:18 tcpconn.go:29: Told user the UCP port is 58001
2021/05/30 14:11:18 tcpconn.go:34: Waiting for file info
2021/05/30 14:11:18 tcpconn.go:36: Raw file meta message {"name":"small.txt","size":32}
2021/05/30 14:11:18 user.go:184: Got file info File name: small.txt. File size 32
2021/05/30 14:11:18 user.go:66: 0  rawDataProcessWorker started
2021/05/30 14:11:18 user.go:75: minHeapPushWorker started
2021/05/30 14:11:18 user.go:79: minHeapPollWorker started
2021/05/30 14:11:18 user.go:82: saveToDiskWorker started
2021/05/30 14:11:18 tcpconn.go:29: Told user the UCP port is Server prepare ready

2021/05/30 14:11:18 user.go:113: Message received from User validate
2021/05/30 14:11:18 user.go:138: User finished send all package. we need to do validation
2021/05/30 14:11:18 tcpconn.go:62: Told user to send the packet 0 again. Msg: NeedPacket:0
packet-received: bytes=46 from=127.0.0.1:58317
Got raw data with size:  46
2021/05/30 14:11:18 user.go:230: Successfully processed data chunk 0 and pushed into processedDataQueue.
pushed data chunk 0 into min heap
Got signal! Check the current heap top! 
2021/05/30 14:11:18 user.go:345: Write Chunk 0, 32 bytes data into disk
2021/05/30 14:11:18 tcpconn.go:69: EOF
2021/05/30 14:11:18 user.go:113: Message received from User EOF
2021/05/30 14:11:18 user.go:142: All Validation is done. Cleaning up
2021/05/30 14:11:18 tcpconn.go:69: read tcp 127.0.0.1:8888->127.0.0.1:54924: use of closed network connection
2021/05/30 14:11:18 user.go:108: Connection closed. Stop sync
udp_server run done context canceled
User 127.0.0.1:8888 finished task
minHeapPushWorker cancelled
minHeapPushWorker routine finished
minHeapPollWorker cancelled
rawDataProcessWorker cancelled
Got raw data with size:  0
minHeapPollWorker routine finished
saveToDiskWorker cancelled
minHeapPollWorker routine finished
2021/05/30 14:11:18 user.go:128: sync finished

```

# Client log

```
╰─ go run client.go                                                                                        ─╯
2021/05/30 14:11:18 client.go:49: Start to dial server
2021/05/30 14:11:18 client.go:342: Got UDP port 58001
2021/05/30 14:11:18 client.go:62: UDP buffer value is: 9216
2021/05/30 14:11:18 client.go:65: Start to open file
2021/05/30 14:11:18 client.go:85: Send file info {"name":"small.txt","size":32}
2021/05/30 14:11:18 client.go:92: Server prepare ready

2021/05/30 14:11:18 client.go:126: indexChan limit 1
2021/05/30 14:11:18 client.go:129: readAndEmitWorker started.
2021/05/30 14:11:18 client.go:131: feedbackWorker started.
2021/05/30 14:11:18 client.go:146: Asked server to validate 9 bytes
2021/05/30 14:11:18 client.go:165: Server is asking NeedPacket:0
2021/05/30 14:11:18 client.go:177: User is requesting chunk of index 0
2021/05/30 14:11:18 client.go:185: Push index 0 into channel
2021/05/30 14:11:18 client.go:222: start to read chunk with index 0
2021/05/30 14:11:18 client.go:225: file read offset 0
2021/05/30 14:11:18 client.go:227: Read index 0, 32 bytes
2021/05/30 14:11:18 client.go:229: EOF
2021/05/30 14:11:18 udp_client.go:79: packet-written: bytes=46
2021/05/30 14:11:18 client.go:105: Start to close client
2021/05/30 14:11:18 client.go:155: read tcp 127.0.0.1:54924->127.0.0.1:8888: use of closed network connection
2021/05/30 14:11:18 client.go:105: Start to close client
2021/05/30 14:11:18 client.go:107: TCP Connection closed
2021/05/30 14:11:18 client.go:109: UDP Connection closed
2021/05/30 14:11:18 client.go:107: TCP Connection closed
2021/05/30 14:11:18 client.go:111: File reader closed
2021/05/30 14:11:18 client.go:109: UDP Connection closed
2021/05/30 14:11:18 client.go:113: Context closed
2021/05/30 14:11:18 client.go:111: File reader closed
2021/05/30 14:11:18 client.go:113: Context closed
2021/05/30 14:11:18 client.go:137: full cycle done cancelled
2021/05/30 14:11:18 client.go:256: readAndEmitWorker cancelled
2021/05/30 14:11:18 client.go:35: Finished. cost  112.484038ms
```