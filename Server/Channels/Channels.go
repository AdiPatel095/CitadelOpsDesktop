package Channels

type OutgoingMessageWithCost struct {
	Payload []byte
	Cost    int
}

var (
	IncomingMessages = make(chan []string, 100)
	OutgoingMessages = make(chan interface{}, 100)
)
