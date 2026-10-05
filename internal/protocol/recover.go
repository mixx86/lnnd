package protocol

type RecoverRequest struct{}

func (RecoverRequest) Encode() []byte {
	return nil
}

func (RecoverRequest) Type() MsgKind {
	return MsgRecoverRequest
}

func (RecoverRequest) Size() uint32 {
	return 0
}
