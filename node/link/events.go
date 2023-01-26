package link

type Event interface{}

type EventConnEstablished struct {
	Conn *Conn
}

type EventConnClosed struct {
	Conn *Conn
}

type EventLinkEstablished struct {
	Link *Link
}

type EventLinkClosed struct {
	Link *Link
}