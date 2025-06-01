package networkio

import "log"

// func: a consumer which consumes events and pushes to connections
func EventDispacherWorker() {
	for {
		event := <-masterNotificationPushQueueChan

		// get the conn object from event.connId
		conn, exists := Manager.GetConnFromConnId(event.connId)
		if !exists {
			// well shit
			log.Fatal(exists)
		}

		conn.WriteToConn(event.msg)

		// just do conn.WriteToConn(msg)
	}
}
