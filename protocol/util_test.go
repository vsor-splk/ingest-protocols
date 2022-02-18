package protocol

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/signalfx/golib/v3/log"
	. "github.com/smartystreets/goconvey/convey"
)

var errReadErr = errors.New("could not read")

type testReader struct {
	content []byte
	err     error
}

func (tr *testReader) Read(b []byte) (int, error) {
	if tr.err != nil {
		return 0, tr.err
	}
	n := copy(b, tr.content)
	return n, io.EOF
}

func TestReadFromRequest(t *testing.T) {
	Convey("ReadFromRequest", t, func() {
		reader := &testReader{}
		var req http.Request
		out := bytes.NewBuffer([]byte{})

		Convey("good read", func() {
			reader.content = []byte{0x05}
			req.Body = io.NopCloser(reader)
			req.ContentLength = 1
			err := ReadFromRequest(out, &req, log.Discard)
			So(err, ShouldBeNil)
			So(out.Bytes(), ShouldResemble, []byte{0x05})
		})

		Convey("bad read", func() {
			reader.err = errReadErr
			req.Body = io.NopCloser(reader)
			err := ReadFromRequest(out, &req, log.Discard)
			So(err, ShouldEqual, errReadErr)
		})
	})
}
