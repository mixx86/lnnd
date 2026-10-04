package store

import "errors"

var (
    ErrNoPreviousCheckpoint = errors.New("no previous checkpoint")
    ErrInvalidHeader        = errors.New("invalid requests file header")
    ErrInvalidCheckpoint    = errors.New("invalid checkpoint")
    ErrCorruptEntry         = errors.New("corrupt entry")
    ErrTruncatedEntry       = errors.New("truncated entry")
    ErrChecksumMismatch     = errors.New("checkpoint checksum mismatch")
    ErrUnexpectedCheckpoint = errors.New("unexpected checkpoint")
)
