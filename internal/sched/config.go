package sched

import "time"

const (
	RequestsCacheWeight = 1.0
	RequestsWeight      = 2.0
	FilesWeight         = 4.0
)

const (
	requestEnqueueTimeout = 100 * time.Millisecond
	writeConfirmTimeout   = 10 * time.Second
	pressureWaitTimeout   = 2 * time.Second
)
