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

# Client log

```
...
2021/05/30 16:21:45 client.go:206: Read index 14835 with offset 22252500.
Read offset 22252500, 1500 bytes
2021/05/30 16:21:45 client.go:166: Asking server do validation
2021/05/30 16:21:45 udp_client.go:78: packet-written: bytes=2006
2021/05/30 16:21:45 tcpconn.go:40: Asked server to validate
2021/05/30 16:21:45 client.go:210: Chunk 14835 of size 1500 sent
2021/05/30 16:21:45 client.go:204: start to read chunk 14836/14836
2021/05/30 16:21:45 client.go:206: Read index 14836 with offset 22254000.
Read offset 22254000, 526 bytes
2021/05/30 16:21:45 reader.go:53: EOF
2021/05/30 16:21:45 udp_client.go:78: packet-written: bytes=710
2021/05/30 16:21:45 client.go:210: Chunk 14836 of size 526 sent
2021/05/30 16:21:45 client.go:146: Server is asking Finished
2021/05/30 16:21:45 client.go:149: Finished, cancel context
2021/05/30 16:21:45 client.go:95: Start to close client
2021/05/30 16:21:45 client.go:97: TCP Connection closed
2021/05/30 16:21:45 client.go:99: UDP Connection closed
2021/05/30 16:21:45 client.go:101: File reader closed
2021/05/30 16:21:45 client.go:103: Context closed
2021/05/30 16:21:45 client.go:151: Fully cancelled
2021/05/30 16:21:45 client.go:181: feedbackWorker cancelled
2021/05/30 16:21:45 client.go:225: readAndEmitWorker cancelled
2021/05/30 16:21:45 client.go:129: full cycle done cancelled
2021/05/30 16:21:45 client.go:36: File info File name: book.pdf. File size 22254526. Total packet count 14837
```