package proto

import (
	"context"
	"encoding"
	"encoding/binary"
	"fmt"
	"net"
	"sync"

	"github.com/cedws/w101-client-go/proto/control"
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

	sessionID         uint16
	sessionTimeSecs   uint32
	sessionTimeMillis uint32
	connected         bool

	closeOnce sync.Once
}

func Dial(ctx context.Context, remote string, router *MessageRouter) (*Client, error) {
	conn, err := net.Dial("tcp", remote)
	if err != nil {
		return nil, err
	}

	controlRW := controlReadWriter{
		frameReader{conn},
		frameWriter{conn},
	}

	client := &Client{
		router:    router,
		conn:      conn,
		controlRW: controlRW,
		controlCh: make(chan *Frame),
		messageCh: make(chan *Frame),
	}

	go client.read()

	if err := client.handshake(ctx); err != nil {
		return nil, err
	}

	go client.handleControl()
	go client.handleMessages()

	return client, nil
}

func (c *Client) handshake(ctx context.Context) error {
	for {
		select {
		case frame := <-c.controlCh:
			c.handleControlFrame(frame)
			if c.connected {
				return nil
			}
		case <-ctx.Done():
		}
	}
}

func (c *Client) handleControl() {
	for frame := range c.controlCh {
		c.handleControlFrame(frame)
	}
}

func (c *Client) handleControlFrame(frame *Frame) {
	switch frame.Opcode {
	case control.PktSessionOffer:
		c.handleSessionOffer(frame)
	}
}

func (c *Client) handleMessages() {
	for frame := range c.messageCh {
		var dmlMessage DMLMessage

		if err := dmlMessage.UnmarshalBinary(frame.MessageData); err != nil {
			return
		}

		handlers, ok := c.router.Handlers(dmlMessage.ServiceID, dmlMessage.OrderNumber)
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
	c.sessionTimeSecs = offer.TimeSecs
	c.sessionTimeMillis = offer.TimeMillis
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

func (c *Client) SessionTimeSecs() uint32 {
	return c.sessionTimeSecs
}

func (c *Client) SessionTimeMillis() uint32 {
	return c.sessionTimeMillis
}

func (c *Client) WriteMessage(service, order byte, msg Message) error {
	msgBuf, err := msg.MarshalBinary()
	if err != nil {
		return err
	}

	dml := DMLMessage{
		ServiceID:   service,
		OrderNumber: order,
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

func (r *MessageRouter) Handlers(service, order byte) ([]func(DMLMessage), bool) {
	if _, ok := r.serviceRoutes[service]; !ok {
		return nil, false
	}

	if _, ok := r.serviceRoutes[service][order]; !ok {
		return nil, false
	}

	return r.serviceRoutes[service][order], true
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
