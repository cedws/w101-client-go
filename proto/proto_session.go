package proto

import "time"

type Session struct {
	ID         uint16
	TimeSecs   uint32
	TimeMillis uint32
	Start      time.Time
}
