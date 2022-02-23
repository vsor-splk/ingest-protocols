package otlp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/signalfx/golib/v3/datapoint/dptest"
	"github.com/signalfx/golib/v3/log"
	. "github.com/smartystreets/goconvey/convey"
	metricsservicev1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
)

var errReadErr = errors.New("could not read")

type errorReader struct{}

func (errorReader *errorReader) Read([]byte) (int, error) {
	return 0, errReadErr
}

func TestDecoder(t *testing.T) {
	Convey("httpMetricDecoder", t, func() {
		sendTo := dptest.NewBasicSink()
		decoder := NewHTTPMetricDecoder(sendTo, log.Discard)

		Convey("Bad request reading", func() {
			req := &http.Request{
				Body: io.NopCloser(&errorReader{}),
			}
			req.ContentLength = 1
			ctx := context.Background()
			So(decoder.Read(ctx, req), ShouldEqual, errReadErr)
		})

		Convey("Bad request content", func() {
			req := &http.Request{
				Body: io.NopCloser(bytes.NewBufferString("asdf")),
			}
			req.ContentLength = 4
			ctx := context.Background()
			So(decoder.Read(ctx, req), ShouldNotBeNil)
		})

		Convey("Good request", func(c C) {
			var msg metricsservicev1.ExportMetricsServiceRequest
			msg.ResourceMetrics = []*metricsv1.ResourceMetrics{
				{
					InstrumentationLibraryMetrics: []*metricsv1.InstrumentationLibraryMetrics{
						{
							Metrics: []*metricsv1.Metric{
								{
									Name: "test",
									Data: &metricsv1.Metric_Gauge{
										Gauge: &metricsv1.Gauge{
											DataPoints: []*metricsv1.NumberDataPoint{
												{
													Attributes:       []*commonv1.KeyValue{},
													StartTimeUnixNano: 1000,
													TimeUnixNano:      1000,
													Value:             &metricsv1.NumberDataPoint_AsInt{AsInt: 4},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			b, _ := proto.Marshal(&msg)
			req := &http.Request{
				Body: io.NopCloser(bytes.NewBuffer(b)),
			}
			req.ContentLength = int64(len(b))
			ctx := context.Background()

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				dp := <-sendTo.PointsChan
				c.So(dp, ShouldNotBeNil)
				wg.Done()
			}()

			So(decoder.Read(ctx, req), ShouldBeNil)

			wg.Wait()
		})
	})
}
