package sched

import "errors"

var (
	ErrRequestsQueueFull = errors.New("requests queue is full")
)
