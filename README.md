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
server.log                                                                           ─╯
2021/05/30 10:47:41 New user joined
2021/05/30 10:47:41 User started
2021/05/30 10:47:41 Get local address [::]:64070
2021/05/30 10:47:41 Tell client we are listening to this port 64070
2021/05/30 10:47:41 Told user the UCP port is 64070
2021/05/30 10:47:41 Waiting for file info
2021/05/30 10:47:41 Raw file meta message {"name":"small.txt","size":32}
2021/05/30 10:47:41 Got file info File name: small.txt. File size 32
2021/05/30 10:47:41 0  rawDataProcessWorker started
2021/05/30 10:47:41 1  rawDataProcessWorker started
2021/05/30 10:47:41 2  rawDataProcessWorker started
2021/05/30 10:47:41 3  rawDataProcessWorker started
2021/05/30 10:47:41 4  rawDataProcessWorker started
2021/05/30 10:47:41 5  rawDataProcessWorker started
2021/05/30 10:47:41 6  rawDataProcessWorker started
2021/05/30 10:47:41 7  rawDataProcessWorker started
2021/05/30 10:47:41 8  rawDataProcessWorker started
2021/05/30 10:47:41 9  rawDataProcessWorker started
2021/05/30 10:47:41 minHeapPushWorker started
2021/05/30 10:47:41 minHeapPollWorker started
2021/05/30 10:47:41 saveToDiskWorker started
2021/05/30 10:47:41 Told user the UCP port is Server prepare ready

2021/05/30 10:47:41 Successfully processed data chunk 0 and pushed into processedDataQueue.
2021/05/30 10:47:41 Write 32 bytes data into disk
2021/05/30 10:47:41 Message received from User validate
2021/05/30 10:47:41 User finished send all package. we need to do validation
2021/05/30 10:47:41 All required 1 packets received
2021/05/30 10:47:41 Told user we have finished.
2021/05/30 10:47:42 read tcp 127.0.0.1:8888->127.0.0.1:53666: use of closed network connection
2021/05/30 10:47:42 Message received from User 
2021/05/30 10:47:42 read tcp 127.0.0.1:8888->127.0.0.1:53666: use of closed network connection
2021/05/30 10:47:42 Message received from User 
2021/05/30 10:47:42 sync finished
```

# Client log

```
2021/05/30 10:47:41 Start to dial server
2021/05/30 10:47:41 Got UDP port 64070
2021/05/30 10:47:41 UDP buffer value is: 9216
2021/05/30 10:47:41 Send file info {"name":"small.txt","size":32}
2021/05/30 10:47:41 Server prepare ready

2021/05/30 10:47:41 indexChan limit 1
2021/05/30 10:47:41 readAndEmitWorker started.
2021/05/30 10:47:41 feedbackWorker started.
2021/05/30 10:47:41 Total packet count 1
2021/05/30 10:47:41 Push index 0 into channel
2021/05/30 10:47:41 start to read chunk with index 0
2021/05/30 10:47:41 file read offset 0
2021/05/30 10:47:41 Asked server to validate 9 bytes
2021/05/30 10:47:41 Server is asking Finished

2021/05/30 10:47:41 Finished, so we can cancel context
2021/05/30 10:47:42 Fully cancelled
```