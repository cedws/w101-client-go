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

	conn    net.Conn
	frameRW frameReadWriter

	readControlCh  chan *Frame
	readMessageCh  chan *Frame
	writeMessageCh chan *Frame

	session          Session
	sessionHeartbeat *time.Ticker
	connected        bool

	closeOnce sync.Once
}

func Dial(ctx context.Context, remote string, router *MessageRouter) (*Client, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", remote)
	if err != nil {
		return nil, err
	}

	frameRW := frameReadWriter{
		FrameReader{conn},
		FrameWriter{conn},
	}

	client := &Client{
		router: router,

		conn:    conn,
		frameRW: frameRW,

		readControlCh:  make(chan *Frame, 8),
		readMessageCh:  make(chan *Frame, 8),
		writeMessageCh: make(chan *Frame, 8),

		sessionHeartbeat: time.NewTicker(heartbeatInterval),
	}

	go client.read()
	go client.write()

	if err := client.handshake(ctx); err != nil {
		return nil, fmt.Errorf("session handshake failed: %w", err)
	}

	go client.handleControl()
	go client.handleMessages()

	return client, nil
}

func (c *Client) handshake(ctx context.Context) error {
	for {
		select {
		case frame, ok := <-c.readControlCh:
			if !ok {
				return fmt.Errorf("connection closed before handshake")
			}

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
			SessionID:           c.session.ID,
			TimeMillis:          uint16(time.Now().Nanosecond() / 1_000_000),
			SessionDurationMins: uint16(time.Since(c.session.Start).Minutes()),
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

		if err := c.router.Handle(dmlMessage.ServiceID, dmlMessage.OrderNumber, dmlMessage); err != nil {
			c.Close()
			return
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
	c.session = Session{
		ID:         offer.SessionID,
		TimeSecs:   offer.TimeSecs,
		TimeMillis: offer.TimeMillis,
		Start:      time.Now(),
	}
}

func (c *Client) read() {
	defer c.Close()

	for {
		frame, err := c.frameRW.Read()
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
	defer c.Close()

	for frame := range c.writeMessageCh {
		if err := c.frameRW.Write(frame); err != nil {
			return
		}
	}
}

func (c *Client) SessionID() uint16 {
	return c.session.ID
}

func (c *Client) SessionTimeSecs() uint32 {
	return c.session.TimeSecs
}

func (c *Client) SessionTimeMillis() uint32 {
	return c.session.TimeMillis
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
		close(c.writeMessageCh)
	})
	return c.conn.Close()
}

type messageRouter [255][]func(DMLMessage) error

type serviceRouter [255]messageRouter

type MessageRouter struct {
	middleware    []func(any)
	serviceRoutes serviceRouter
}

func NewMessageRouter() MessageRouter {
	return MessageRouter{}
}

func (r *MessageRouter) Handle(service, order byte, d DMLMessage) error {
	for _, handler := range r.serviceRoutes[service][order] {
		if err := handler(d); err != nil {
			return err
		}
	}

	return nil
}

// RegisterMiddleware registers a middleware function that will be called for every message
// that matches type T. Middleware can receive every message by registering middleware that
// receives the *any* type.
func RegisterMiddleware[T any](router *MessageRouter, handler func(T)) {
	handleFunc := func(msg any) {
		msgType, ok := msg.(T)
		if ok {
			handler(msgType)
		}
	}

	router.middleware = append(router.middleware, handleFunc)
}

func RegisterMessageHandler[T any](router *MessageRouter, service, order byte, handler func(T)) {
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

		for _, middleware := range router.middleware {
			middleware(msg)
		}

		handler(msg)

		return nil
	}

	router.serviceRoutes[service][order] = append(router.serviceRoutes[service][order], decodeFunc)
}
