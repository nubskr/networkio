package networkio

type Event struct {
	connId string
	msg    Message
}

func getNewEvent(connId string, msg string) *Event {
	return &Event{
		connId: connId,
		msg:    *getNewMessage(msg, "PAYLOAD"),
	}
}

func (e *Event) waitToBeDelivered() {
	// essentially blocks untill ack is received for that message from peer

	// poll ? whatever
}
