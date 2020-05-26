package signalfx

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/mailru/easyjson"
	"github.com/signalfx/golib/v3/log"
	"github.com/signalfx/golib/v3/logsink"
	signalfxformat "github.com/signalfx/ingest-protocols/protocol/signalfx/format"
)

const (
	// ResourceHostName is used to identify host.hostname
	ResourceHostName = "host.hostname"
	// ResourceServiceName is used to identify service.name
	ResourceServiceName = "service.name"

	// AttributeSourceType is used to identify source.type
	AttributeSourceType = "source.type"
	// AttributeIndex is used to identify index
	AttributeIndex = "index"
)

// JSONDecoderHECV1 implements sink decoder for hecv1 splunk log events
type JSONDecoderHECV1 struct {
	Sink   logsink.Sink
	Logger log.Logger
}

// Read implements interface method for decoding incoming json hecv1 format and convert it into golib/logsink.Log format
func (decoder *JSONDecoderHECV1) Read(ctx context.Context, req *http.Request) (err error) {
	var logHECV1List signalfxformat.JSONLogHECV1List

	if err = easyjson.UnmarshalFromReader(req.Body, &logHECV1List); err == nil {
		logs := make(logsink.Logs, 0, len(logHECV1List))
		for _, lEntry := range logHECV1List {
			if lEntry != nil {
				e := &logsink.Log{
					Body: lEntry.Event,
					Resource: map[string]interface{}{
						ResourceHostName:    lEntry.Host,
						ResourceServiceName: lEntry.Source,
					},
					Attributes: map[string]interface{}{
						AttributeSourceType: lEntry.SourceType,
						AttributeIndex:      lEntry.Index,
					},
				}

				if lEntry.Time != nil {
					e.TimeStamp = uint64(time.Duration(*lEntry.Time).Nanoseconds())
				}

				for key, value := range lEntry.Fields {
					e.Attributes[key] = value
				}
				logs = append(logs, e)
			}
		}
		// HEC doesn't follow industry standard http status code and hence we need to update the context
		// with proper protocol information so that log-ingest service can return status code for HEC specifics based
		// on https://docs.splunk.com/Documentation/Splunk/7.1.0/RESTREF/RESTinput#HTTP_status_codes
		err = decoder.Sink.AddLogs(context.WithValue(ctx, LogProtocolType, HECV1Protocol), logs)
	}

	return err
}

// SetupJSONLogPaths tells the router which paths the given handler (which should handle v1/log json formats)
func SetupJSONLogPaths(r *mux.Router, handler http.Handler) {
	SetupJSONByPaths(r, handler, DefaultLogPathV1)
}
