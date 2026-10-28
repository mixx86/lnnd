package sched

import "errors"

var (
	ErrRequestsQueueFull = errors.New("requests queue is full")
	ErrWriteTimeout = errors.New("timed out waiting for write confirmation")
	ErrUnknownSource = errors.New("unknown source")
)
