package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"reflect"
	"sync"
	"time"
)

/*
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
	data   string
}

func getNewMessage(data string, header string) Message {
	return Message{
		header: header,
		uuid:   getUUID(),
		data:   data,
	}
}

type Connection struct {
	OurId        string // this is our id
	PeerId       string // this is what we are connecting to, just an uuid like ClientId
	IsInbound    bool   // who initiated this connection
	WriteQueue   chan Message
	ReadQueue    chan Message
	Conn         net.Conn
	sentACKs     sync.Map
	receivedACKs sync.Map
}

func serializeStruct(s interface{}) (string, error) {
	val := reflect.ValueOf(s)
	if val.Kind() != reflect.Struct {
		return "", errors.New("Not a struct!")
	}
	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *Connection) writeToConnLoop() {
	conn := c.Conn
	for {
		msg := <-c.WriteQueue

		for {
			encoder := json.NewEncoder(conn)
			err := encoder.Encode(msg)
			if err != nil {
				log.Print("Error encoding message: %v\n", err)
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
		decoder := json.NewDecoder(conn)
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
			c.PeerId = msg.data
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
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (c *Connection) WriteToConn(v interface{}) error {
	data, err := serializeStruct(v)
	if err != nil {
		return err
	}
	c.WriteQueue <- getNewMessage(string(data), "PAYLOAD")
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
		OurId:        ourId,
		IsInbound:    false,
		WriteQueue:   make(chan Message, WRITE_QUEUE_BUFFER),
		ReadQueue:    make(chan Message, READ_QUEUE_BUFFER),
		Conn:         conn,
		sentACKs:     sync.Map{},
		receivedACKs: sync.Map{},
	}

	go newConn.readFromConnLoop()
	go newConn.writeToConnLoop()

	newConn.doHandshake()
	// TODO: wait here till the handshake is complete, we only return active connections, no async shit
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
