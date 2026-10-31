package protocol

import (
	"bytes"
	"testing"
)

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
