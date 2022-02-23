package protocol

import (
	"bytes"
	"net/http"

	"github.com/signalfx/golib/v3/log"
	"github.com/signalfx/ingest-protocols/logkey"
)

// ReadFromRequest reads all the http body into the jeff buffer and logs if there is an error.
func ReadFromRequest(jeff *bytes.Buffer, req *http.Request, logger log.Logger) error {
	// for compressed transactions, contentLength isn't trustworthy
	readLen, err := jeff.ReadFrom(req.Body)
	if err != nil {
		logger.Log(log.Err, err, logkey.ReadLen, readLen, logkey.ContentLength, req.ContentLength, "Unable to fully read from buffer")
		return err
	}
	return nil
}
