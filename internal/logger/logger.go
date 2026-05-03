package logger

import (
	"log"
	"os"
)

var (
	Daemon   = log.New(os.Stdout, "\033[31m[lnnd]\033[0m ", log.LstdFlags|log.Lmsgprefix)
	Protocol = log.New(os.Stdout, "\033[31m[lnnp]\033[0m ", log.LstdFlags|log.Lmsgprefix)
)
