package networkio

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitConnection(t *testing.T) {
	go AcceptConnRequestLoop("server")
	time.Sleep(time.Second)

	conn, err := InitConnection("127.0.0.1", "8080", "client")
	require.NoError(t, err, "Failed to initialize connection")
	require.NotNil(t, conn, "Connection object is nil")
	require.NotNil(t, conn.Conn, "Connection socket is nil")
	require.Equal(t, "client", conn.OurId, "OurId mismatch")
}

// TODO: make it more rigorous
func TestHandshake(t *testing.T) {
	go AcceptConnRequestLoop("server")
	time.Sleep(time.Second)

	conn, err := InitConnection("127.0.0.1", "8080", "client")
	require.NoError(t, err)
	require.NotNil(t, conn)

	select {
	case <-conn.handshakeDoneChan:
		// Success
	case <-time.After(3 * time.Second):
		t.Fatal("Handshake timeout")
	}
}

// TODO: this can be made more rigorous!
func TestMessageTransmission(t *testing.T) {
	go AcceptConnRequestLoop("server")
	time.Sleep(time.Second * 2)

	clientConn, err := InitConnection("127.0.0.1", "8080", "client")
	require.NoError(t, err)
	require.NotNil(t, clientConn)

	msg := "Hello ser!"
	err = clientConn.WriteToConn(msg)
	require.NoError(t, err)

	// check from server if the message actually arrives or not
	connId := <-MasterMessageQueue
	conn, exists := Manager.GetConnFromConnId(connId)
	if exists {
		recMsg := conn.ReadFromConn().Data.(string)
		require.Equal(t, msg, recMsg, "Received message does not match sent message")
	} else {
		t.Fatal("Connection not found in manager")
	}

}

// TODO: this is totally bullshit, fix it
func TestReconnect(t *testing.T) {
	go AcceptConnRequestLoop("server")
	time.Sleep(time.Second)

	clientConn, err := InitConnection("127.0.0.1", "8080", "client")
	require.NoError(t, err)
	require.NotNil(t, clientConn)

	clientConn.Conn.Close()
	time.Sleep(time.Second)

	reconnectedConn, err := InitConnection("127.0.0.1", "8080", "client")
	require.NoError(t, err)
	require.NotNil(t, reconnectedConn)
}

// TODO: can be made better
func TestMultipleClients(t *testing.T) {
	go AcceptConnRequestLoop("server")
	time.Sleep(time.Second)

	var clients []*Connection
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		go func(i int) {
			client, err := InitConnection("127.0.0.1", "8080", "client"+string(rune(i)))
			require.NoError(t, err)
			mu.Lock()
			clients = append(clients, client)
			mu.Unlock()
		}(i)
	}
	time.Sleep(2 * time.Second)

	assert.Equal(t, 5, len(clients))
}

// TODO: this doesnt tests for shit, make it better
func TestMessageQueueing(t *testing.T) {
	go AcceptConnRequestLoop("server")
	time.Sleep(time.Second * 2)

	clientConn, err := InitConnection("127.0.0.1", "8080", "client")
	require.NoError(t, err)
	require.NotNil(t, clientConn)

	msg1 := "Message 1"

	err = clientConn.WriteToConn(msg1)
	require.NoError(t, err)
	err = clientConn.WriteToConn(msg1)
	require.NoError(t, err)

	time.Sleep(time.Second * 5)

	assert.Equal(t, 2, len(MasterMessageQueue))
}
