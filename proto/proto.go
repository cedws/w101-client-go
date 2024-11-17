package proto

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/cedws/w101-client-go/proto/control"
)

const heartbeatInterval = 10 * time.Second

type MessageMarshaler interface {
	Marshal() []byte
}

type MessageUnmarshaler interface {
	Unmarshal([]byte) error
}

type Message interface {
	MessageMarshaler
	MessageUnmarshaler
}

type DMLMessage struct {
	ServiceID   byte
	OrderNumber byte
	Packet      []byte
}

func (d *DMLMessage) Unmarshal(buf []byte) error {
	if len(buf) < 4 {
		return fmt.Errorf("invalid dml message")
	}

	d.ServiceID = buf[0]
	d.OrderNumber = buf[1]

	dataLen := binary.LittleEndian.Uint16(buf[2:4]) + 1
	if dataLen > uint16(len(buf)) {
		return fmt.Errorf("invalid dml message")
	}
	d.Packet = buf[4:dataLen]

	return nil
}

func (d DMLMessage) Marshal() []byte {
	buf := []byte{
		d.ServiceID,
		d.OrderNumber,
	}

	dataLen := len(buf) + len(d.Packet) + 2
	buf = binary.LittleEndian.AppendUint16(buf, uint16(dataLen))
	buf = append(buf, d.Packet...)

	return buf
}

type Client struct {
	router *MessageRouter

	conn      net.Conn
	controlRW controlReadWriter

	readControlCh  chan *Frame
	readMessageCh  chan *Frame
	writeMessageCh chan *Frame

	sessionID         uint16
	sessionTimeSecs   uint32
	sessionTimeMillis uint32
	sessionStart      time.Time
	sessionHeartbeat  *time.Ticker
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
		router: router,

		conn:      conn,
		controlRW: controlRW,

		readControlCh:  make(chan *Frame, 8),
		readMessageCh:  make(chan *Frame, 8),
		writeMessageCh: make(chan *Frame, 8),

		sessionHeartbeat: time.NewTicker(heartbeatInterval),
	}

	go client.read()
	go client.write()

	if err := client.handshake(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("session handshake failed: %w", err)
	}

	go client.handleControl()
	go client.handleMessages()

	return client, nil
}

func (c *Client) handshake(ctx context.Context) error {
	for {
		select {
		case frame := <-c.readControlCh:
			c.handleControlFrame(frame)
			if c.connected {
				go c.heartbeat()
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *Client) heartbeat() {
	for range c.sessionHeartbeat.C {
		keepAlive := &control.ClientKeepAlive{
			SessionID:           c.sessionID,
			TimeMillis:          uint16(time.Now().Nanosecond() / 1_000_000),
			SessionDurationMins: uint16(time.Since(c.sessionStart).Minutes()),
		}

		c.writeMessageCh <- &Frame{
			Control:     true,
			Opcode:      control.PktSessionKeepAlive,
			MessageData: keepAlive.Marshal(),
		}
	}
}

func (c *Client) handleControl() {
	for frame := range c.readControlCh {
		c.handleControlFrame(frame)
	}
}

func (c *Client) handleControlFrame(frame *Frame) {
	switch frame.Opcode {
	case control.PktSessionKeepAlive:
		c.handleSessionKeepAlive(frame)
	case control.PktSessionOffer:
		c.handleSessionOffer(frame)
	case control.PktSessionAccept:
		// ignore
	}
}

func (c *Client) handleMessages() {
	for frame := range c.readMessageCh {
		var dmlMessage DMLMessage

		if err := dmlMessage.Unmarshal(frame.MessageData); err != nil {
			return
		}

		handlers, ok := c.router.Handlers(dmlMessage.ServiceID, dmlMessage.OrderNumber)
		if !ok {
			return
		}

		for _, handler := range handlers {
			if err := handler(dmlMessage); err != nil {
				c.Close()
			}
		}
	}
}

func (c *Client) handleSessionKeepAlive(_ *Frame) {
	c.writeMessageCh <- &Frame{
		Control:     true,
		Opcode:      control.PktSessionKeepAliveRsp,
		MessageData: (&control.KeepAliveRsp{}).Marshal(),
	}
}

func (c *Client) handleSessionOffer(frame *Frame) {
	offer := &control.SessionOffer{}
	if err := offer.Unmarshal(frame.MessageData); err != nil {
		return
	}

	accept := &control.SessionAccept{
		TimeSecs:   offer.TimeSecs,
		TimeMillis: offer.TimeMillis,
		SessionID:  offer.SessionID,
	}

	c.writeMessageCh <- &Frame{
		Control:     true,
		Opcode:      control.PktSessionAccept,
		MessageData: accept.Marshal(),
	}

	c.connected = true
	c.sessionID = offer.SessionID
	c.sessionTimeSecs = offer.TimeSecs
	c.sessionTimeMillis = offer.TimeMillis
	c.sessionStart = time.Now()
}

func (c *Client) read() {
	for {
		frame, err := c.controlRW.Read()
		if err != nil {
			return
		}

		if frame.Control {
			c.readControlCh <- frame
		} else {
			c.readMessageCh <- frame
		}
	}
}

func (c *Client) write() {
	for frame := range c.writeMessageCh {
		if err := c.controlRW.Write(frame); err != nil {
			return
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
	dml := DMLMessage{
		ServiceID:   service,
		OrderNumber: order,
		Packet:      msg.Marshal(),
	}

	c.writeMessageCh <- &Frame{
		MessageData: dml.Marshal(),
	}

	return nil
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.sessionHeartbeat.Stop()
		close(c.readControlCh)
		close(c.readMessageCh)
	})
	return c.conn.Close()
}

type messageRouter map[byte][]func(DMLMessage) error

type serviceRouter map[byte]messageRouter

type MessageRouter struct {
	serviceRoutes serviceRouter
}

func NewMessageRouter() *MessageRouter {
	return &MessageRouter{
		serviceRoutes: make(serviceRouter),
	}
}

func (r *MessageRouter) Handlers(service, order byte) ([]func(DMLMessage) error, bool) {
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
		router.serviceRoutes[service][order] = make([]func(DMLMessage) error, 0)
	}

	decodeFunc := func(d DMLMessage) error {
		var msg T

		// This sucks
		dec, ok := any(&msg).(MessageUnmarshaler)
		if !ok {
			panic("message type does not implement proto.MessageUnmarshaler")
		}

		if err := dec.Unmarshal(d.Packet); err != nil {
			return err
		}

		handler(msg)

		return nil
	}

	router.serviceRoutes[service][order] = append(router.serviceRoutes[service][order], decodeFunc)
}
