package otlp

import (
	"bytes"
	"context"
	"net/http"
	"sync"

	"github.com/signalfx/golib/v3/datapoint/dpsink"
	"github.com/signalfx/golib/v3/log"
	"github.com/signalfx/ingest-protocols/logkey"
	"github.com/signalfx/ingest-protocols/protocol"
	"github.com/signalfx/ingest-protocols/protocol/signalfx"
	metricsservicev1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/protobuf/proto"
)

type httpMetricDecoder struct {
	sink   dpsink.Sink
	logger log.Logger
	buffs  sync.Pool
}

func NewHTTPMetricDecoder(sink dpsink.Sink, logger log.Logger) signalfx.ErrorReader {
	return &httpMetricDecoder{
		sink:   sink,
		logger: log.NewContext(logger).With(logkey.Protocol, "otlp"),
		buffs: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}
}

func (d *httpMetricDecoder) Read(ctx context.Context, req *http.Request) (err error) {
	jeff := d.buffs.Get().(*bytes.Buffer)
	defer d.buffs.Put(jeff)
	jeff.Reset()
	if err = protocol.ReadFromRequest(jeff, req, d.logger); err != nil {
		return err
	}
	var msg metricsservicev1.ExportMetricsServiceRequest
	if err = proto.Unmarshal(jeff.Bytes(), &msg); err != nil {
		return err
	}
	dps := FromOTLPMetricRequest(&msg)
	if len(dps) > 0 {
		err = d.sink.AddDatapoints(ctx, dps)
	}
	return nil
}
