package service

type Broadcaster interface {
	Broadcast(msgType string, data interface{})
}
