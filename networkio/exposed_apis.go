package networkio

// TODO: make the below channels exported so user can access it
var masterNotificationPushQueueChan chan Event = make(chan Event)
var masterNotificationPullQueueChan chan string = make(chan string, MASTER_MESSAGE_QUEUE_BUFFER)

func PushNotification(event *Event) {
	// makes an event

	// pushes it to notiifcation pusher queue
	masterNotificationPushQueueChan <- *event
}

func PullNotification() string {
	// listens to incoming notifications from peers

	// manually pulls a notification and returns it

	return <-masterNotificationPullQueueChan
}
