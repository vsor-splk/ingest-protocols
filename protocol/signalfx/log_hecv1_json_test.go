package signalfx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"github.com/gorilla/mux"
	"github.com/mailru/easyjson"
	"github.com/signalfx/golib/v3/errors"
	"github.com/signalfx/golib/v3/log"
	"github.com/signalfx/golib/v3/logsink"
	"github.com/signalfx/golib/v3/pointer"
	signalfxformat "github.com/signalfx/ingest-protocols/protocol/signalfx/format"
	. "github.com/smartystreets/goconvey/convey"
)

// at least 3 numData is expected to test entryNil & timeNil
func getHecV1Data(numData int, entryNil bool, timeNil bool) ([]byte, error) {
	const (
		keyString     = "key"
		valueString   = "value"
		entryNilIndex = 1
		timeNilIndex  = 2
	)
	dataList := make(signalfxformat.JSONLogHECV1List, numData)
	for i := 0; i < numData; i++ {
		entry := &signalfxformat.JSONLogHECV1{
			Time:       pointer.Int64(time.Now().UnixNano()),
			Host:       "localhost",
			Source:     "test-app" + strconv.Itoa(i),
			SourceType: "my_sample_data" + strconv.Itoa(i),
			Event: map[string]interface{}{
				"message":  "Something happened",
				"severity": "INFO",
			},
			Index: "main",
		}
		entry.Fields = make(map[string]interface{})
		for j := 0; j < numData; j++ {
			entry.Fields[keyString+strconv.Itoa(j)] = valueString + strconv.Itoa(j)
		}

		dataList[i] = entry
	}
	if entryNil {
		dataList[numData-entryNilIndex] = nil
	}
	if timeNil {
		dataList[numData-timeNilIndex].Time = nil
	}
	return easyjson.Marshal((*signalfxformat.JSONLogHECV1List)(unsafe.Pointer(&dataList)))
}

type logTestSink struct {
	ctx         context.Context
	lCount      int64
	err         error
	lastReqRcvd logsink.Logs
}

func (f *logTestSink) AddLogs(ctx context.Context, logs []*logsink.Log) error {
	if f.err == nil {
		f.lastReqRcvd = logs
		f.lCount++
		f.ctx = ctx
	}
	return f.err
}

var (
	errTest = errors.New("you shall not pass")
)

func TestJSONDecoderHECV1_Read(t *testing.T) {
	Convey("testing hecv1 decoder", t, func() {
		finalSink := &logTestSink{}
		dec := &JSONDecoderHECV1{
			Sink:   finalSink,
			Logger: log.DefaultLogger,
		}
		ctx := context.Background()

		numData := 3
		Convey(fmt.Sprintf("sending %d data", numData), func() {

			jsonData2Send, err := getHecV1Data(numData, false, false)
			So(err, ShouldBeNil)
			req := &http.Request{
				Body: ioutil.NopCloser(bytes.NewReader(jsonData2Send)),
			}

			err = dec.Read(ctx, req)
			So(err, ShouldBeNil)

			So(finalSink.lCount, ShouldEqual, 1)
			So(finalSink.ctx.Value(LogProtocolType), ShouldNotBeNil)
			So(finalSink.ctx.Value(LogProtocolType).(LogProtocol), ShouldEqual, HECV1Protocol)
			So(len(finalSink.lastReqRcvd), ShouldEqual, numData)

			Convey("we should fail when there is error", func() {
				finalSink.err = errTest
				req2 := &http.Request{
					Body: ioutil.NopCloser(bytes.NewReader(jsonData2Send)),
				}
				So(dec.Read(ctx, req2), ShouldEqual, errTest)
			})
		})
	})

	Convey("testing hecv1 decoder with input nil", t, func() {
		finalSink := &logTestSink{}
		dec := &JSONDecoderHECV1{
			Sink:   finalSink,
			Logger: log.DefaultLogger,
		}
		ctx := context.Background()

		numData := 6
		Convey(fmt.Sprintf("sending %d data", numData), func() {

			jsonData2Send, err := getHecV1Data(numData, true, true)
			So(err, ShouldBeNil)
			req := &http.Request{
				Body: ioutil.NopCloser(bytes.NewReader(jsonData2Send)),
			}

			err = dec.Read(ctx, req)
			So(err, ShouldBeNil)

			So(finalSink.lCount, ShouldEqual, 1)
			So(finalSink.ctx.Value(LogProtocolType), ShouldNotBeNil)
			So(finalSink.ctx.Value(LogProtocolType).(LogProtocol), ShouldEqual, HECV1Protocol)
			So(len(finalSink.lastReqRcvd), ShouldEqual, numData-1)

			Convey("we should fail when there is error", func() {
				finalSink.err = errTest
				req2 := &http.Request{
					Body: ioutil.NopCloser(bytes.NewReader(jsonData2Send)),
				}
				So(dec.Read(ctx, req2), ShouldEqual, errTest)
			})
		})
	})
}

func TestSetupJSONLogPaths(t *testing.T) {
	Convey("setting up endpoint should work", t, func() {

		router := mux.NewRouter()
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"alive": true}`)
		})

		SetupJSONLogPaths(router, handler)

		Convey(fmt.Sprintf("we should be able to send request to %s", DefaultLogPathV1), func() {
			req := httptest.NewRequest(http.MethodPost, DefaultLogPathV1, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			So(rr.Code, ShouldEqual, http.StatusOK)
		})
	})
}

func BenchmarkJSONDecoderHECV1_Read(b *testing.B) {
	type benchScenario struct {
		desc      string
		data      []byte
		numData   int
		finalSink *logTestSink
		ctx       context.Context
		dec       *JSONDecoderHECV1
	}

	descPrefix := "benchmark hecv1 data"

	benchScenarios := make([]*benchScenario, 0, 10)

	for idx := 1; idx <= cap(benchScenarios); idx++ {
		s := benchScenario{
			numData: idx * 500,
			finalSink: &logTestSink{
				err: nil,
			},
			ctx: context.Background(),
		}
		s.desc = fmt.Sprintf("%s for %d entries", descPrefix, s.numData)
		s.data, _ = getHecV1Data(s.numData, false, false)
		s.dec = &JSONDecoderHECV1{
			Sink:   s.finalSink,
			Logger: log.DefaultLogger,
		}
		benchScenarios = append(benchScenarios, &s)
	}
	b.ResetTimer()
	for _, benchTest := range benchScenarios {
		test := benchTest
		fmt.Println("running benchmark for", test.desc)
		b.Run(test.desc, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				req := &http.Request{
					Body: ioutil.NopCloser(bytes.NewReader(test.data)),
				}
				b.StartTimer()
				err := test.dec.Read(test.ctx, req)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
