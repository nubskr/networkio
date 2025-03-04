package networkio

import (
	"crypto/rand"
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"syscall"
	"time"
)

/*
if a connection breaks, who should be the one initiating the reconnect ? because if both try to reconnect by sending the HANDSHAKE stuff, shit gets fucked

TODO:

when a connection does, we can save their writequeue and readqueue in case the connection is made again later

handshake:

send:
- hi, I am `ClientId` and I wanna talk to you

receive:
- hi `ClientId`, nice to meet you, I am `TargetId` and would be happy to chat

handshake complete, maybe also exchange some encryption keys or smth
*/

// if connection breaks, all goroutines associated with it must exit!

const WRITE_QUEUE_BUFFER = 100
const READ_QUEUE_BUFFER = 100
const MASTER_MESSAGE_QUEUE_BUFFER = 1000

var MasterMessageQueue chan string = make(chan string, MASTER_MESSAGE_QUEUE_BUFFER)

type ConnectionManager struct {
	mu          sync.Mutex
	connections map[string]*Connection
}

var Manager = NewConnectionManager()

func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{connections: make(map[string]*Connection)}
}

func (cm *ConnectionManager) AddConnection(id string, c *Connection) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.connections[id] = c
}

func (cm *ConnectionManager) GetConnFromConnId(id string) (*Connection, bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	conn, exists := cm.connections[id]
	return conn, exists
}

// RemoveConnection closes and unregisters a connection.
func (cm *ConnectionManager) RemoveConnection(id string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if conn, exists := cm.connections[id]; exists {
		conn.Conn.Close()
		delete(cm.connections, id)
	}
}

type Message struct {
	Header string
	UUID   string
	Data   any
}

func getNewMessage(data any, header string) Message {
	return Message{
		Header: header,
		UUID:   getUUID(),
		Data:   data,
	}
}

type Connection struct {
	ConnId              string
	OurId               string // this is our id
	PeerId              string // this is who we're connecting to, just an uuid like ClientId
	IsInbound           bool   // who initiated this connection
	WriteQueue          chan Message
	ReadQueue           chan Message
	ACKQueue            chan Message
	Conn                net.Conn
	sentACKs            sync.Map
	receivedACKs        sync.Map
	handshakeInProgress sync.Mutex
	handshakeDoneChan   chan struct{}
	ConnectionDead      chan struct{}
}

func (c *Connection) doConnChores() {
	go c.readFromConnLoop()
	go c.writeToConnLoop()
	go c.writeACKtoConnLoop()
}

func isConnectionReallyDead(conn net.Conn) bool {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		// Not a TCP connection, can't check properly
		return true
	}

	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return true // Assume dead if we can't access low-level details
	}

	var socketError int
	err = rawConn.Control(func(fd uintptr) {
		socketError, _ = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_ERROR)
	})

	return err != nil || socketError != 0
}

// even if we can reconnect somehow, we can't just switch c.Conn on the fly duh, multiple stuff is using it, read loop is using it, write loop is using it
func tryToReconnect(c *Connection) {
	if c.IsInbound {
		// let the other guy try
		return
	}

	conn := c.Conn
	if isConnectionReallyDead(conn) {
		remoteAddr := conn.RemoteAddr()
		tcpAddr, ok := remoteAddr.(*net.TCPAddr)
		if !ok {
			log.Println("Not a TCP connection")
			Manager.RemoveConnection(c.ConnId)
			return
		}

		addr := tcpAddr.IP.String()
		port := strconv.Itoa(tcpAddr.Port)
		conn.Close()

		// try to reestablish it
		newConn, err := InitConnection(addr, port, c.OurId)
		if err != nil {
			log.Println("Error reconnecting:", err)
			Manager.RemoveConnection(c.ConnId)
			return
		}

		c.Conn = newConn.Conn

		close(c.ConnectionDead)
		c.doConnChores()
		// WARN: changing conn on fly, might cause some race conds
	}
}

func (c *Connection) writeACKtoConnLoop() {
	conn := c.Conn
	for {
		select {
		case msg := <-c.ACKQueue:
			encoder := gob.NewEncoder(conn)
			err := encoder.Encode(msg)
			if err != nil {
				log.Print("something seems shady with the connection sire: ", conn)
				// bhai aese to queue khaali ho jayegi :skull:
				tryToReconnect(c)
				return
			}

		case <-c.ConnectionDead:
			// conn ded
			return
		}
	}
}

func (c *Connection) writeToConnLoop() {
	conn := c.Conn
	for {
		select {
		case msg := <-c.WriteQueue:
			for {
				encoder := gob.NewEncoder(conn)
				err := encoder.Encode(msg)
				if err != nil {
					log.Print("something seems shady with the connection sire: ", conn)
					tryToReconnect(c)
					return
				}

				if msg.Header == "HANDSHAKE" || msg.Header == "ACK" {
					break
				}

				// waiting for ack, TODO: change this nasty shit later
				time.Sleep(2 * time.Second)

				if val, ok := c.receivedACKs.Load(msg.UUID); ok && val.(bool) {
					break
				}
			}

		case <-c.ConnectionDead:
			// conn ded
			return
		}
	}
}

func (c *Connection) readFromConnLoop() {
	conn := c.Conn
	for {
		decoder := gob.NewDecoder(conn)
		var msg Message
		err := decoder.Decode(&msg)
		if err != nil {
			log.Printf("Error decoding message or something wrong with the connetion: %v\n", err)
			// panic("concurrency mix up hogya bruh kahi")
		} else {
			// log.Print("we received something sire: ", msg)
		}

		if msg.Header == "HANDSHAKE" {
			// check if the handshakeDoneChan is open, only send the ACK_HANDSHAKE if not already done
			select {
			case <-c.handshakeDoneChan:
				// Channel is already closed, do nothing
			default:
				ackMsg := getNewMessage(c.OurId, "ACK_HANDSHAKE")
				c.WriteQueue <- ackMsg
				close(c.handshakeDoneChan)
			}
			continue
		}

		if msg.Header == "ACK" {
			c.receivedACKs.Store(msg.Data.(string), true)
			continue
		}
		if msg.Header == "ACK_HANDSHAKE" {
			// msg.data will be the peerId in this case
			c.PeerId = msg.Data.(string)
			// log.Print("handshake mutex unlocked!!!")

			c.handshakeInProgress.Unlock()
		}

		ackMsg := getNewMessage(msg.UUID, "ACK")
		c.ACKQueue <- ackMsg

		if val, ok := c.sentACKs.Load(msg.UUID); ok && val.(bool) {
			// have we already processed this before ?
			continue
		}

		c.sentACKs.Store(msg.UUID, true)

		if msg.Header == "PAYLOAD" {
			c.ReadQueue <- msg
			MasterMessageQueue <- c.ConnId // just so that the application knows where to look at
		}
	}
}

func getUUID() string {
	b := make([]byte, 4)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}
	return string(b)
}

func (c *Connection) WriteToConn(data any) error {
	select {
	case <-c.handshakeDoneChan:
		// safe to send now
	case <-time.After(time.Second * 2):
		// if you want a timeout, you can handle it here
		return fmt.Errorf("timeout waiting for handshake completion")
	}

	c.WriteQueue <- getNewMessage(data, "PAYLOAD")
	return nil
}

func (c *Connection) ReadFromConn() Message {
	select {
	case <-c.handshakeDoneChan:
		// safe to send now
	case <-time.After(time.Second * 2):
		// fmt.Errorf("timeout waiting for handshake completion")
		panic("yeah, we f'ed up")
		return Message{}
	}
	return <-c.ReadQueue
}

func (c *Connection) doHandshake() {
	c.WriteQueue <- getNewMessage(c.OurId, "HANDSHAKE")
	// log.Print("handshake initiated")
}

// we go to someone, a -> b
// note that it is the very starting, so no other communication is being done here
func InitConnection(addr string, port string, ourId string) (*Connection, error) {
	conn, err := net.Dial("tcp", addr+":"+port)
	if err != nil {
		return nil, fmt.Errorf("error connecting to %s:%s: %v", addr, port, err)
	}

	newConn := &Connection{
		ConnId:              getUUID(),
		OurId:               ourId,
		IsInbound:           false,
		WriteQueue:          make(chan Message, WRITE_QUEUE_BUFFER),
		ReadQueue:           make(chan Message, READ_QUEUE_BUFFER),
		ACKQueue:            make(chan Message, 1),
		Conn:                conn,
		sentACKs:            sync.Map{},
		receivedACKs:        sync.Map{},
		handshakeInProgress: sync.Mutex{},
		handshakeDoneChan:   make(chan struct{}),
		ConnectionDead:      make(chan struct{}),
	}

	newConn.doConnChores()

	newConn.handshakeInProgress.Lock()
	newConn.doHandshake()

	// Block until the handshake completes.
	newConn.handshakeInProgress.Lock()
	newConn.handshakeInProgress.Unlock()

	close(newConn.handshakeDoneChan)

	Manager.AddConnection(newConn.ConnId, newConn)

	return newConn, nil
}

func AcceptConnRequestLoop(OurId string) {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Println("Error listening on port 8080:", err)
		return
	}
	log.Print("server listening on 8080 sire")
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}

		newConn := &Connection{
			ConnId:            getUUID(),
			OurId:             OurId,
			IsInbound:         true,
			WriteQueue:        make(chan Message, WRITE_QUEUE_BUFFER),
			ReadQueue:         make(chan Message, READ_QUEUE_BUFFER),
			ACKQueue:          make(chan Message, 1),
			Conn:              conn,
			sentACKs:          sync.Map{},
			receivedACKs:      sync.Map{},
			handshakeDoneChan: make(chan struct{}),
			ConnectionDead:    make(chan struct{}),
		}

		newConn.doConnChores()
		Manager.AddConnection(newConn.ConnId, newConn)
	}
}
