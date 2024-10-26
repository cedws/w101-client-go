package proto

import (
	"encoding"
	"encoding/binary"
	"fmt"
	"net"
	"sync"

	"github.com/cedws/go-dml-codegen/proto/control"
)

type Message interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

type DMLMessage struct {
	ServiceID   byte
	OrderNumber byte
	Packet      []byte
}

func (d *DMLMessage) UnmarshalBinary(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("invalid dml message")
	}

	d.ServiceID = data[0]
	d.OrderNumber = data[1]
	d.Packet = data[4:]

	return nil
}

func (d DMLMessage) MarshalBinary() ([]byte, error) {
	buf := []byte{
		d.ServiceID,
		d.OrderNumber,
	}
	buf = binary.LittleEndian.AppendUint16(buf, uint16(len(buf)+len(d.Packet)+2))
	buf = append(buf, d.Packet...)

	return buf, nil
}

type Client struct {
	router *MessageRouter

	conn      net.Conn
	controlRW controlReadWriter
	controlCh chan *Frame
	messageCh chan *Frame
	closeOnce sync.Once

	sessionID uint16
	connected bool
}

func Dial(router *MessageRouter, remote string) (*Client, error) {
	conn, err := net.Dial("tcp", remote)
	if err != nil {
		return nil, err
	}

	controlRW := controlReadWriter{frameReader{conn}, frameWriter{conn}}

	client := &Client{
		router:    router,
		conn:      conn,
		controlRW: controlRW,
		controlCh: make(chan *Frame),
		messageCh: make(chan *Frame),
	}

	go client.handleControl()
	go client.handleMessages()
	go client.read()

	return client, nil
}

func (c *Client) handleControl() {
	for frame := range c.controlCh {
		switch frame.Opcode {
		case control.PktSessionOffer:
			c.handleSessionOffer(frame)
		}
	}
}

func (c *Client) handleMessages() {
	for frame := range c.messageCh {
		var dmlMessage DMLMessage

		if err := dmlMessage.UnmarshalBinary(frame.MessageData); err != nil {
			return
		}

		service, ok := c.router.serviceRoutes[dmlMessage.ServiceID]
		if !ok {
			return
		}

		handlers, ok := service[dmlMessage.OrderNumber]
		if !ok {
			return
		}

		for _, handler := range handlers {
			handler(dmlMessage)
		}
	}
}

func (c *Client) handleSessionOffer(frame *Frame) {
	if c.connected {
		return
	}

	offer := &control.SessionOffer{}
	if err := offer.UnmarshalBinary(frame.MessageData); err != nil {
		return
	}

	accept := &control.SessionAccept{
		TimeSecs:   offer.TimeSecs,
		TimeMillis: offer.TimeMillis,
		SessionID:  offer.SessionID,
	}
	acceptBuf, err := accept.MarshalBinary()
	if err != nil {
		return
	}

	resp := &Frame{
		Control:     true,
		Opcode:      control.PktSessionAccept,
		MessageData: acceptBuf,
	}

	if err := c.controlRW.Write(resp); err != nil {
		return
	}

	c.connected = true
	c.sessionID = offer.SessionID

	x := LatestFileListV2{
		Locale: "English",
	}
	xb, _ := x.MarshalBinary()

	d := DMLMessage{
		ServiceID:   8,
		OrderNumber: 2,
		Packet:      xb,
	}
	db, _ := d.MarshalBinary()

	c.controlRW.Write(&Frame{
		MessageData: db,
	})
}

func (c *Client) read() {
	for {
		frame, err := c.controlRW.Read()
		if err != nil {
			return
		}

		if frame.Control {
			c.controlCh <- frame
		} else {
			c.messageCh <- frame
		}
	}
}

func (c *Client) SessionID() uint16 {
	return c.sessionID
}

func (c *Client) WriteMessage(service, order int, msg Message) error {
	msgBuf, err := msg.MarshalBinary()
	if err != nil {
		return err
	}

	// TODO: can service/order be byte?
	dml := DMLMessage{
		ServiceID:   byte(service),
		OrderNumber: byte(order),
		Packet:      msgBuf,
	}

	buf, err := dml.MarshalBinary()
	if err != nil {
		return err
	}

	return c.controlRW.Write(&Frame{
		MessageData: buf,
	})
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		close(c.controlCh)
		close(c.messageCh)
	})
	return c.conn.Close()
}

type messageRouter map[byte][]func(DMLMessage)

type serviceRouter map[byte]messageRouter

type MessageRouter struct {
	serviceRoutes serviceRouter
}

func NewMessageRouter() *MessageRouter {
	return &MessageRouter{
		serviceRoutes: make(serviceRouter),
	}
}

func RegisterMessageHandler[T any](router *MessageRouter, service, order byte, handler func(T)) {
	if _, ok := router.serviceRoutes[service]; !ok {
		router.serviceRoutes[service] = make(messageRouter)
	}

	if _, ok := router.serviceRoutes[service][order]; !ok {
		router.serviceRoutes[service][order] = make([]func(DMLMessage), 0)
	}

	decodeFunc := func(d DMLMessage) {
		var msg T

		// This sucks
		if err := any(&msg).(encoding.BinaryUnmarshaler).UnmarshalBinary(d.Packet); err != nil {
			// TODO
		}

		handler(msg)
	}

	router.serviceRoutes[service][order] = append(router.serviceRoutes[service][order], decodeFunc)
}