package main

import (
	"crypto/rand"
	"encoding/gob"
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

use better serialization

TODO handshake:

send:
- hi, I am `ClientId` and I wanna talk to you

receive:
- hi `ClientId`, nice to meet you, I am `TargetId` and would be happy to chat

handshake complete, maybe also exchange some encryption keys or smth
*/

const WRITE_QUEUE_BUFFER = 100
const READ_QUEUE_BUFFER = 100

type Message struct {
	header string
	uuid   string
	data   any
}

func getNewMessage(data any, header string) Message {
	return Message{
		header: header,
		uuid:   getUUID(),
		data:   data,
	}
}

type Connection struct {
	OurId               string // this is our id
	PeerId              string // this is what we are connecting to, just an uuid like ClientId
	IsInbound           bool   // who initiated this connection
	WriteQueue          chan Message
	ReadQueue           chan Message
	Conn                net.Conn
	sentACKs            sync.Map
	receivedACKs        sync.Map
	handshakeInProgress sync.Mutex
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
			return
		}

		addr := tcpAddr.IP.String()
		port := strconv.Itoa(tcpAddr.Port)
		conn.Close()

		// try to reestablish it
		conn = InitConnection(addr, port, c.OurId).Conn
		// WARN: changing conn on fly, might cause some race conds
	}
}

func (c *Connection) writeToConnLoop() {
	conn := c.Conn
	for {
		msg := <-c.WriteQueue

		for {
			encoder := gob.NewEncoder(conn)
			err := encoder.Encode(msg)
			if err != nil {
				log.Print("something wrong with the connection sire: ", conn)
				// bhai aese to queue khaali ho jayegi :skull:
				return
			}

			// wait for ack, TODO: change this nasty shit later
			time.Sleep(2 * time.Second)

			if val, ok := c.receivedACKs.Load(msg.uuid); ok && val.(bool) {
				break
			}
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
			log.Printf("Error decoding message: %v\n", err)
			return
		}

		if msg.header == "HANDSHAKE" {
			ackMsg := getNewMessage(c.OurId, "ACK_HANDSHAKE")
			c.WriteQueue <- ackMsg
			return
		}

		if msg.header == "ACK" {
			c.receivedACKs.Store(msg.data, true)
			continue
		}
		if msg.header == "ACK_HANDSHAKE" {
			// msg.data will be the peerId in this case
			c.PeerId = msg.data.(string)
			c.handshakeInProgress.Unlock()
			continue
		}

		ackMsg := getNewMessage(msg.uuid, "ACK")
		c.WriteQueue <- ackMsg

		if val, ok := c.sentACKs.Load(msg.uuid); ok && val.(bool) {
			continue
		}

		c.sentACKs.Store(msg.uuid, true)
		c.ReadQueue <- msg
	}
}

func getUUID() string {
	b := make([]byte, 5)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}
	return string(b)
}

func (c *Connection) WriteToConn(data any) error {
	c.WriteQueue <- getNewMessage(data, "PAYLOAD")
	return nil
}

func (c *Connection) ReadFromConn() Message {
	return <-c.ReadQueue
}

func (c *Connection) doHandshake() {
	c.WriteQueue <- getNewMessage(c.OurId, "HANDSHAKE")
	log.Print("handshake initiated")
}

// we go to someone, a -> b
// note that it is the very starting, so no other communication is being done here
func InitConnection(addr string, port string, ourId string) Connection {
	conn, err := net.Dial("tcp", addr+":"+port)
	if err != nil {
		log.Printf("Error connecting to %s:%s: %v\n", addr, port, err)
		return Connection{}
	}

	// Create new outbound connection
	newConn := Connection{
		OurId:               ourId,
		IsInbound:           false,
		WriteQueue:          make(chan Message, WRITE_QUEUE_BUFFER),
		ReadQueue:           make(chan Message, READ_QUEUE_BUFFER),
		Conn:                conn,
		sentACKs:            sync.Map{},
		receivedACKs:        sync.Map{},
		handshakeInProgress: sync.Mutex{},
	}

	go newConn.readFromConnLoop()
	go newConn.writeToConnLoop()

	newConn.handshakeInProgress.Lock()
	newConn.doHandshake()

	// wtf man, are you a caveman ? use channels here
	newConn.handshakeInProgress.Lock()
	newConn.handshakeInProgress.Unlock()

	return newConn
}

func AcceptConnRequestLoop(c *Connection) {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		return
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		// someone comes to us, a <- b, they will init the handshake
		newConn := &Connection{
			OurId:        c.OurId,
			IsInbound:    true,
			WriteQueue:   make(chan Message, WRITE_QUEUE_BUFFER),
			ReadQueue:    make(chan Message, READ_QUEUE_BUFFER),
			Conn:         conn,
			sentACKs:     sync.Map{},
			receivedACKs: sync.Map{},
		}

		go newConn.readFromConnLoop()
		go newConn.writeToConnLoop()
	}
}
