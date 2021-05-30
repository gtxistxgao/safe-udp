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
2021/05/30 16:45:50 client.go:36: File sent. cost 5.654177829s
2021/05/30 16:45:50 client.go:38: File info File name: book.pdf. File size 22254526. Total packet count 14837
2021/05/30 16:45:50 client.go:184: feedbackWorker cancelled
2021/05/30 16:45:50 client.go:39: Speed: 3843.695088 Kb/s
2021/05/30 16:45:50 client.go:228: readAndEmitWorker cancelled
2021/05/30 16:45:50 client.go:40: Speed: 3.753608 Mb/s
```