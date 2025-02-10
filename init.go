package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"reflect"
)

/*
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
	uuid string
	data string
}

func getNewMessage(data string) Message {
	return Message{
		uuid: getUUID(),
		data: data,
	}
}

type Connection struct {
	ClientId        string // this is our id
	TargetId        string // this is what we are connecting to, just an uuid like ClientId
	IsInbound       bool   // who initiated this connection
	WriteQueue      chan Message
	ReadQueue       chan Message
	Conn            net.Conn
	IsUUIDProcessed map[string]bool
}

// private func
func (c *Connection) writeToConnLoop() {
	for {
		msg := <-c.WriteQueue
		conn := c.Conn
		// serialize the
	}
	for {
		// reads from the queue

		// sends it, waits for ack, if times out, sends it again, waits for ack, and keeps going like that

		// if ack is received, just moves on with life
	}
}

func getUUID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}
	// UUID v4
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// use gob for serialization here ?
func (c *Connection) WriteToConn(v interface{}) error {
	// serialises stuff
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Struct {
		return errors.New("Not a struct!")
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.WriteQueue <- getNewMessage(string(data))
	return nil
}

func (c *Connection) ReceiveLoop() {
	// just keeps listening

	for {
		conn := c.Conn
		// read the data from this conn

		// send ack for the data received

		// acks are sent directly, no queue needed for them ? or do we need a queue for them ? yes we do actually, oh wait, we will only be sending one ack at a time, won't we ?

	}
}

// connect to some address, us -> them, also do the handshake part
func InitConnection(c *Connection) error {

}

// TODO: implement the handshake part
func AcceptConnRequestLoop(c *Connection) {
	listener, err := net.Listen("tcp", ":8080") // or whatever port you want to use
	if err != nil {
		return
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		// Create new connection for inbound request
		newConn := &Connection{
			ClientId:        c.ClientId,
			IsInbound:       true,
			WriteQueue:      make(chan Message, WRITE_QUEUE_BUFFER),
			ReadQueue:       make(chan Message, READ_QUEUE_BUFFER),
			Conn:            conn,
			IsUUIDProcessed: make(map[string]bool),
		}

		// Start read and write routines for new connection
		go newConn.ReceiveLoop()
		go newConn.writeToConnLoop()
	}
}
