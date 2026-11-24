package protocol

import (
	"bytes"
	"errors"
	"testing"
)

// sender_req_id is what the receiver stores as rmt_req_id and names in its
// RESOLUTION: it must survive the round trip, and 0, which names no request,
// must be rejected as an invalid message so the peer gets ERROR.
func TestSymlinkRequestSenderReqID(t *testing.T) {
	req := SymlinkRequest{SenderReqID: 9, Src: "/a", Dst: "/b"}
	got, err := ParseSymlinkRequest(req.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if got != req {
		t.Fatalf("got %+v, want %+v", got, req)
	}
	if int(req.Size()) != len(req.Encode()) {
		t.Fatalf("Size() = %d, encoded %d bytes", req.Size(), len(req.Encode()))
	}

	req.SenderReqID = 0
	_, err = ParseSymlinkRequest(req.Encode())
	if !errors.Is(err, MsgErrInvalidMessage) {
		t.Fatalf("sender_req_id 0: got %v, want MsgErrInvalidMessage", err)
	}
}

func TestResolutionEncode(t *testing.T) {
	cases := []struct {
		res  Resolution
		want []byte
	}{
		{Resolution{RMTReqID: 0x01020304, Accepted: true},
			[]byte{1, 2, 3, 4, 1}},
		{Resolution{RMTReqID: 7, Accepted: false},
			[]byte{0, 0, 0, 7, 0}},
	}
	for _, c := range cases {
		got := c.res.Encode()
		if !bytes.Equal(got, c.want) {
			t.Errorf("Encode(%+v) = %v, want %v", c.res, got, c.want)
		}
		if int(c.res.Size()) != len(got) {
			t.Errorf("Size() = %d, encoded %d bytes", c.res.Size(), len(got))
		}
	}
}
