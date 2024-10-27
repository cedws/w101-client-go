package proto

import (
	"context"
	"encoding"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/cedws/w101-client-go/proto/control"
)

const heartbeatInterval = 10 * time.Second

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
	if len(data) < 4 {
		return fmt.Errorf("invalid dml message")
	}

	d.ServiceID = data[0]
	d.OrderNumber = data[1]

	dataLen := binary.LittleEndian.Uint16(data[2:4]) + 1
	if dataLen > uint16(len(data)) {
		return fmt.Errorf("invalid dml message")
	}
	d.Packet = data[4:dataLen]

	return nil
}

func (d DMLMessage) MarshalBinary() ([]byte, error) {
	buf := []byte{
		d.ServiceID,
		d.OrderNumber,
	}

	dataLen := len(buf) + len(d.Packet) + 2
	buf = binary.LittleEndian.AppendUint16(buf, uint16(dataLen))
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
		controlCh: make(chan *Frame),
		messageCh: make(chan *Frame),

		sessionHeartbeat: time.NewTicker(heartbeatInterval),
	}

	go client.read()

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
		case frame := <-c.controlCh:
			c.handleControlFrame(frame)
			if c.connected {
				go c.heartbeat()
				return nil
			}
		case <-ctx.Done():
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

		keepAliveBuf, _ := keepAlive.MarshalBinary()

		frame := &Frame{
			Control:     true,
			Opcode:      control.PktSessionKeepAlive,
			MessageData: keepAliveBuf,
		}

		if err := c.controlRW.Write(frame); err != nil {
			return
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
	case control.PktSessionKeepAlive:
		c.handleSessionKeepAlive(frame)
	case control.PktSessionOffer:
		c.handleSessionOffer(frame)
	case control.PktSessionAccept:
		// ignore
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
			if err := handler(dmlMessage); err != nil {
				c.Close()
			}
		}
	}
}

func (c *Client) handleSessionKeepAlive(_ *Frame) {
	keepAliveRspBuf, _ := (&control.KeepAliveRsp{}).MarshalBinary()

	resp := &Frame{
		Control:     true,
		Opcode:      control.PktSessionKeepAliveRsp,
		MessageData: keepAliveRspBuf,
	}

	if err := c.controlRW.Write(resp); err != nil {
		return
	}
}

func (c *Client) handleSessionOffer(frame *Frame) {
	offer := &control.SessionOffer{}
	if err := offer.UnmarshalBinary(frame.MessageData); err != nil {
		return
	}

	accept := &control.SessionAccept{
		TimeSecs:   offer.TimeSecs,
		TimeMillis: offer.TimeMillis,
		SessionID:  offer.SessionID,
	}
	acceptBuf, _ := accept.MarshalBinary()

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
	c.sessionStart = time.Now()
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
	msgBuf, _ := msg.MarshalBinary()

	dml := DMLMessage{
		ServiceID:   service,
		OrderNumber: order,
		Packet:      msgBuf,
	}

	buf, _ := dml.MarshalBinary()

	return c.controlRW.Write(&Frame{
		MessageData: buf,
	})
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.sessionHeartbeat.Stop()
		close(c.controlCh)
		close(c.messageCh)
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
		dec, ok := any(&msg).(encoding.BinaryUnmarshaler)
		if !ok {
			panic("message type does not implement encoding.BinaryUnmarshaler")
		}

		if err := dec.UnmarshalBinary(d.Packet); err != nil {
			return err
		}

		handler(msg)

		return nil
	}

	router.serviceRoutes[service][order] = append(router.serviceRoutes[service][order], decodeFunc)
}
