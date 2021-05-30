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
╰─ go run server.go > server.log                                                                           ─╯
2021/05/30 11:05:34 controller.go:67: New user joined
2021/05/30 11:05:34 controller.go:90: User started
2021/05/30 11:05:34 udp_server.go:41: Get local address [::]:61752
2021/05/30 11:05:34 user.go:179: Tell client we are listening to this port 61752
2021/05/30 11:05:34 tcpconn.go:28: Told user the UCP port is 61752
2021/05/30 11:05:34 tcpconn.go:33: Waiting for file info
2021/05/30 11:05:34 tcpconn.go:35: Raw file meta message {"name":"small.txt","size":32}
2021/05/30 11:05:34 user.go:184: Got file info File name: small.txt. File size 32
2021/05/30 11:05:34 user.go:66: 0  rawDataProcessWorker started
2021/05/30 11:05:34 user.go:75: minHeapPushWorker started
2021/05/30 11:05:34 user.go:79: minHeapPollWorker started
2021/05/30 11:05:34 user.go:82: saveToDiskWorker started
2021/05/30 11:05:34 tcpconn.go:28: Told user the UCP port is Server prepare ready

2021/05/30 11:05:34 user.go:230: Successfully processed data chunk 0 and pushed into processedDataQueue.
2021/05/30 11:05:34 user.go:345: Write 32 bytes data into disk
2021/05/30 11:05:34 user.go:113: Message received from User validate
2021/05/30 11:05:34 user.go:138: User finished send all package. we need to do validation
2021/05/30 11:05:34 user.go:164: All required 1 packets received
2021/05/30 11:05:34 tcpconn.go:51: Told user we have finished.
2021/05/30 11:05:35 tcpconn.go:68: read tcp 127.0.0.1:8888->127.0.0.1:54049: use of closed network connection
2021/05/30 11:05:35 user.go:108: Connection closed. Stop sync
2021/05/30 11:05:35 user.go:128: sync finished
```

# Client log

```
╰─ go run client.go > client.log                                                                           ─╯
2021/05/30 11:05:34 client.go:48: Start to dial server
2021/05/30 11:05:34 client.go:305: Got UDP port 61752
2021/05/30 11:05:34 client.go:64: UDP buffer value is: 9216
2021/05/30 11:05:34 client.go:74: Send file info {"name":"small.txt","size":32}
2021/05/30 11:05:34 client.go:81: Server prepare ready

2021/05/30 11:05:34 client.go:95: indexChan limit 1
2021/05/30 11:05:34 client.go:98: readAndEmitWorker started.
2021/05/30 11:05:34 client.go:100: feedbackWorker started.
2021/05/30 11:05:34 client.go:107: Total packet count 1
2021/05/30 11:05:34 client.go:111: Push index 0 into channel
2021/05/30 11:05:34 client.go:185: start to read chunk with index 0
2021/05/30 11:05:34 client.go:188: file read offset 0
2021/05/30 11:05:34 client.go:120: Asked server to validate 9 bytes
2021/05/30 11:05:34 client.go:138: Server is asking Finished

2021/05/30 11:05:34 client.go:141: Finished, so we can cancel context
2021/05/30 11:05:35 client.go:144: Fully cancelled
```