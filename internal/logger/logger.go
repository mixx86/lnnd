// Package logger provides shared loggers for the LNN daemon.
package logger

import (
	"log"
	"os"
)

var (
	// Daemon is the logger for the lnnd daemon process.
	Daemon = log.New(os.Stdout, "\033[31m[lnnd]\033[0m ", log.LstdFlags|log.Lmsgprefix)
	// Protocol is the logger for protocol-level events.
	Protocol = log.New(os.Stdout, "\033[31m[lnnp]\033[0m ", log.LstdFlags|log.Lmsgprefix)
)
