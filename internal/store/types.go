package store

import (
	"sync"
)

type RequestsWriter struct {
	canPath string

	mu              sync.Mutex
	resolvedEntries int
}

type Writers struct {
	RequestsWriter
}
