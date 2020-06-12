package signalfx

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mailru/easyjson"

	jaegerpb "github.com/jaegertracing/jaeger/model"
	"github.com/signalfx/golib/v3/pointer"
	"github.com/signalfx/golib/v3/trace"
	signalfxformat "github.com/signalfx/ingest-protocols/protocol/signalfx/format"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSink struct {
	handler func([]*trace.Span)
}

func (s *fakeSink) AddSpans(ctx context.Context, spans []*trace.Span) error {
	s.handler(spans)
	return nil
}

// Benchmark the case where all spans are Zipkin v2.
func BenchmarkZipkinV2TraceDecoder(b *testing.B) {
	spanJSON := `
	 {
       "traceId": "0123456789abcdef",
       "name": "span1",
       "id": "abc1234567890def",
       "kind": "CLIENT",
       "timestamp": 1000,
       "duration": 100,
       "debug": true,
       "shared": true,
       "localEndpoint": {
         "serviceName": "myclient",
         "ipv4": "127.0.0.1"
       },
       "remoteEndpoint": {
         "serviceName": "myserver",
         "ipv4": "127.0.1.1",
         "port": 443
       },
       "annotations": [
         {
           "timestamp": 1001,
           "value": "something happened"
         },
         {
           "timestamp": 1010,
           "value": "something else happened"
         }
       ],
       "tags": {
         "additionalProp1": "string",
         "additionalProp2": "string",
         "additionalProp3": "string"
       }
     }`

	const numSpans = 100
	reqBody := `[`
	for i := 0; i < numSpans; i++ {
		reqBody += spanJSON + ","
	}

	reqBody = strings.TrimSuffix(reqBody, ",") + `]`

	decoder := JSONTraceDecoderV1{
		Sink: &fakeSink{
			handler: func(_ []*trace.Span) {},
		},
	}

	req := http.Request{
		Body: ioutil.NopCloser(bytes.NewBufferString(reqBody)),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := decoder.Read(context.Background(), &req)
		if err != nil {
			fmt.Println(i)
			b.Fatal(err)
		}
		req = http.Request{
			Body: ioutil.NopCloser(bytes.NewBufferString(reqBody)),
		}
	}
	b.StopTimer()
}

func interfaceAddr(i interface{}) *interface{} {
	return &i
}

var falseVar = false

func TestZipkinTraceDecoder(t *testing.T) {
	reqBody := ioutil.NopCloser(bytes.NewBufferString(`[
	 {
       "traceId": "0123456789abcdef",
       "name": "span1",
       "id": "abc1234567890def",
       "kind": "CLIENT",
       "timestamp": 1000,
       "duration": 100,
       "debug": true,
       "shared": true,
       "localEndpoint": {
         "serviceName": "myclient",
         "ipv4": "127.0.0.1"
       },
       "remoteEndpoint": {
         "serviceName": "myserver",
         "ipv4": "127.0.1.1",
         "port": 443
       },
       "annotations": [
         {
           "timestamp": 1001,
           "value": "something happened"
         }
       ],
       "tags": {
         "additionalProp1": "string",
         "additionalProp2": "string",
         "additionalProp3": "string"
       }
     },
	 {
       "traceId": "abcdef0123456789",
       "name": "span2",
       "parentId": "0123456789abcdef",
       "id": "abcdef",
       "kind": "SERVER",
       "timestamp": 2000,
       "duration": 200,
       "debug": false,
       "shared": false,
       "tags": {
         "additionalProp1": "string",
         "additionalProp2": "string",
         "additionalProp3": "string"
       }
     },
	 {
       "traceId": "abcdef0123456789",
       "name": "span2",
       "parentId": "0123456789abcdef",
       "id": "badspan-cannot-have-kind-and-binary-annotations",
       "kind": "SERVER",
       "timestamp": 2000,
       "duration": 200,
       "debug": false,
       "shared": false,
	   "binaryAnnotations": [
	     {"key": "a", "value": "v"}
	   ]
     },
	 {
       "traceId": "abcdef0123456789",
       "name": "span2",
       "parentId": "0123456789abcdef",
       "id": "badspan-cannot-have-object-as-binary-annotation-value",
       "timestamp": 2000,
       "duration": 200,
       "debug": false,
       "shared": false,
	   "binaryAnnotations": [
	     {"key": "a", "value": {"k": "v"}}
	   ]
     },
	 {
       "traceId": "abcdef0123456789",
       "name": "span3",
       "parentId": "0000000000000000",
       "id": "oldspan",
       "timestamp": 2000,
       "duration": 200,
	   "binaryAnnotations": [
	     {"key": "a", "value": "v"}
	   ]
     }
	]`))

	spans := []*trace.Span{}
	decoder := JSONTraceDecoderV1{
		Sink: &fakeSink{
			handler: func(ss []*trace.Span) {
				spans = append(spans, ss...)
			},
		},
	}

	req := http.Request{
		Body: reqBody,
	}
	err := decoder.Read(context.Background(), &req)

	Convey("Valid spans should be sent even if some error", t, func() {
		So(err.Error(), ShouldContainSubstring, "invalid binary annotation type")

		So(spans, ShouldResemble, []*trace.Span{
			{
				TraceID:  "0123456789abcdef",
				ParentID: nil,
				ID:       "abc1234567890def",
				Name:     pointer.String("span1"),
				Kind:     &ClientKind,
				LocalEndpoint: &trace.Endpoint{
					ServiceName: pointer.String("myclient"),
					Ipv4:        pointer.String("127.0.0.1"),
				},
				RemoteEndpoint: &trace.Endpoint{
					ServiceName: pointer.String("myserver"),
					Ipv4:        pointer.String("127.0.1.1"),
					Port:        pointer.Int32(443),
				},
				Timestamp: pointer.Int64(1000),
				Duration:  pointer.Int64(100),
				Debug:     &trueVar,
				Shared:    &trueVar,
				Annotations: []*trace.Annotation{
					{Timestamp: pointer.Int64(1001), Value: pointer.String("something happened")},
				},
				Tags: map[string]string{
					"additionalProp1": "string",
					"additionalProp2": "string",
					"additionalProp3": "string",
				},
			},
			{
				TraceID:        "abcdef0123456789",
				ParentID:       pointer.String("0123456789abcdef"),
				ID:             "abcdef",
				Name:           pointer.String("span2"),
				Kind:           &ServerKind,
				LocalEndpoint:  nil,
				RemoteEndpoint: nil,
				Timestamp:      pointer.Int64(2000),
				Duration:       pointer.Int64(200),
				Debug:          &falseVar,
				Shared:         &falseVar,
				Tags: map[string]string{
					"additionalProp1": "string",
					"additionalProp2": "string",
					"additionalProp3": "string",
				},
			},
			{
				TraceID:   "abcdef0123456789",
				ParentID:  nil,
				ID:        "oldspan",
				Name:      pointer.String("span3"),
				Timestamp: pointer.Int64(2000),
				Duration:  pointer.Int64(200),
				Tags: map[string]string{
					"a": "v",
				},
			},
		})
	})
}

// Tests converted from
// https://github.com/openzipkin/zipkin/blob/2.8.4/zipkin/src/test/java/zipkin/internal/V2SpanConverterTest.java
// nolint:funlen
func TestZipkinTraceConversion(t *testing.T) {
	frontend := &trace.Endpoint{
		ServiceName: pointer.String("frontend"),
		Ipv4:        pointer.String("127.0.0.1"),
	}

	backend := &trace.Endpoint{
		ServiceName: pointer.String("backend"),
		Ipv4:        pointer.String("192.168.99.101"),
		Port:        pointer.Int32(9000),
	}

	kafka := &trace.Endpoint{
		ServiceName: pointer.String("kafka"),
	}

	Convey("Zipkin v2 spans gets passed through unaltered", t, func() {
		span := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Duration:  pointer.Float64(207000),
			Span: trace.Span{
				TraceID:        "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID:       pointer.String("6b221d5bc9e6496c"),
				ID:             "5b4185666d50f68b",
				Name:           pointer.String("get"),
				Kind:           &ClientKind,
				LocalEndpoint:  frontend,
				RemoteEndpoint: backend,
				Timestamp:      pointer.Int64(1472470996199000),
				Duration:       pointer.Int64(207000),
				Annotations: []*trace.Annotation{
					{Timestamp: pointer.Int64(1472470996403000), Value: pointer.String("no")},
					{Timestamp: pointer.Int64(1472470996238000), Value: pointer.String("ws")},
					{Timestamp: pointer.Int64(1472470996403000), Value: pointer.String("wr")},
				},
				Tags: map[string]string{
					"http_path":            "/api",
					"clnt/finagle.version": "6.45.0",
				},
			},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span.Span})
	})

	Convey("client", t, func() {
		simpleClient := trace.Span{
			TraceID:        "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:       pointer.String("6b221d5bc9e6496c"),
			ID:             "5b4185666d50f68b",
			Name:           pointer.String("get"),
			Kind:           &ClientKind,
			LocalEndpoint:  frontend,
			RemoteEndpoint: backend,
			Timestamp:      pointer.Int64(1472470996199000),
			Duration:       pointer.Int64(207000),
			Annotations: []*trace.Annotation{
				{Timestamp: pointer.Int64(1472470996403000), Value: pointer.String("no")},
				{Timestamp: pointer.Int64(1472470996238000), Value: pointer.String("ws")},
				{Timestamp: pointer.Int64(1472470996403000), Value: pointer.String("wr")},
			},
			Tags: map[string]string{
				"http_path":            "/api",
				"clnt/finagle.version": "6.45.0",
			},
		}

		client := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Duration:  pointer.Float64(207000),
			Span: trace.Span{TraceID: "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("get"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("cs"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996238000), Value: pointer.String("ws"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996403000), Value: pointer.String("wr"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996406000), Value: pointer.String("cr"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996403000), Value: pointer.String("no"), Endpoint: frontend},
			},
			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("http_path"), Value: interfaceAddr("/api"), Endpoint: frontend},
				{Key: pointer.String("clnt/finagle.version"), Value: interfaceAddr("6.45.0"), Endpoint: frontend},
				{Key: pointer.String("sa"), Value: interfaceAddr(true), Endpoint: backend},
			},
		}

		sp, err := client.fromZipkinV1()
		So(err, ShouldBeNil)
		So(sp, ShouldResemble, []*trace.Span{&simpleClient})
	})

	Convey("client_unfinished", t, func() {
		simpleClient := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:      pointer.String("6b221d5bc9e6496c"),
			ID:            "5b4185666d50f68b",
			Name:          pointer.String("get"),
			Kind:          &ClientKind,
			LocalEndpoint: frontend,
			Timestamp:     pointer.Int64(1472470996199000),
			Annotations: []*trace.Annotation{
				{Timestamp: pointer.Int64(1472470996238000), Value: pointer.String("ws")},
			},
			Tags: map[string]string{},
		}

		client := &InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("get"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("cs"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996238000), Value: pointer.String("ws"), Endpoint: frontend},
			},
		}

		sp, _ := client.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&simpleClient})
	})

	Convey("client_unstarted", t, func() {
		simpleClient := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:      pointer.String("6b221d5bc9e6496c"),
			ID:            "5b4185666d50f68b",
			Name:          pointer.String("get"),
			Kind:          &ClientKind,
			LocalEndpoint: frontend,
			Timestamp:     pointer.Int64(1472470996199000),
			Duration:      pointer.Int64(100),
			Tags:          map[string]string{},
		}

		client := &InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Duration:  pointer.Float64(100),
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("get"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199100), Value: pointer.String("cr"), Endpoint: frontend},
			},
		}

		sp, _ := client.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&simpleClient})
	})

	Convey("noAnnotationsExceptAddresses", t, func() {
		span2 := trace.Span{
			TraceID:        "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:       pointer.String("6b221d5bc9e6496c"),
			ID:             "5b4185666d50f68b",
			Name:           pointer.String("get"),
			LocalEndpoint:  frontend,
			RemoteEndpoint: backend,
			Timestamp:      pointer.Int64(1472470996199000),
			Duration:       pointer.Int64(207000),
			Tags:           map[string]string{},
		}

		span := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Duration:  pointer.Float64(207000),
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("get"),
			},
			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("ca"), Value: interfaceAddr(true), Endpoint: frontend},
				{Key: pointer.String("sa"), Value: interfaceAddr(true), Endpoint: backend},
			},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("fromSpan_redundantAddressAnnotations", t, func() {
		span2 := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:      pointer.String("6b221d5bc9e6496c"),
			ID:            "5b4185666d50f68b",
			Kind:          &ClientKind,
			Name:          pointer.String("get"),
			LocalEndpoint: frontend,
			Timestamp:     pointer.Int64(1472470996199000),
			Duration:      pointer.Int64(207000),
			Tags:          map[string]string{},
		}

		span := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Duration:  pointer.Float64(207000),
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("get"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("cs"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996406000), Value: pointer.String("cr"), Endpoint: frontend},
			},
			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("ca"), Value: interfaceAddr(true), Endpoint: frontend},
				{Key: pointer.String("sa"), Value: interfaceAddr(true), Endpoint: frontend},
			},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("server", t, func() {
		simpleServer := trace.Span{
			TraceID:        "7180c278b62e8f6a216a2aea45d08fc9",
			ID:             "216a2aea45d08fc9",
			Name:           pointer.String("get"),
			Kind:           &ServerKind,
			LocalEndpoint:  backend,
			RemoteEndpoint: frontend,
			Timestamp:      pointer.Int64(1472470996199000),
			Duration:       pointer.Int64(207000),
			Tags: map[string]string{
				"http_path":            "/api",
				"clnt/finagle.version": "6.45.0",
			},
		}

		server := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Duration:  pointer.Float64(207000),
			Span: trace.Span{
				TraceID: "7180c278b62e8f6a216a2aea45d08fc9",
				ID:      "216a2aea45d08fc9",
				Name:    pointer.String("get"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("sr"), Endpoint: backend},
				{Timestamp: pointer.Float64(1472470996406000), Value: pointer.String("ss"), Endpoint: backend},
			},
			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("http_path"), Value: interfaceAddr("/api"), Endpoint: backend},
				{Key: pointer.String("clnt/finagle.version"), Value: interfaceAddr("6.45.0"), Endpoint: backend},
				{Key: pointer.String("ca"), Value: interfaceAddr(true), Endpoint: frontend},
			},
		}

		sp, _ := server.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&simpleServer})
	})

	Convey("client_missingCs", t, func() {
		span2 := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ID:            "216a2aea45d08fc9",
			Name:          pointer.String("get"),
			Kind:          &ClientKind,
			LocalEndpoint: frontend,
			Timestamp:     pointer.Int64(1472470996199000),
			Duration:      pointer.Int64(207000),
			Tags:          map[string]string{},
		}

		span := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Duration:  pointer.Float64(207000),
			Span: trace.Span{
				TraceID: "7180c278b62e8f6a216a2aea45d08fc9",
				ID:      "216a2aea45d08fc9",
				Name:    pointer.String("get"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996406000), Value: pointer.String("cs"), Endpoint: frontend},
			},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("server_missingSr", t, func() {
		span2 := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ID:            "216a2aea45d08fc9",
			Name:          pointer.String("get"),
			Kind:          &ServerKind,
			LocalEndpoint: backend,
			Timestamp:     pointer.Int64(1472470996199000),
			Duration:      pointer.Int64(207000),
			Tags:          map[string]string{},
		}

		span := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Duration:  pointer.Float64(207000),
			Span: trace.Span{
				TraceID: "7180c278b62e8f6a216a2aea45d08fc9",
				ID:      "216a2aea45d08fc9",
				Name:    pointer.String("get"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996406000), Value: pointer.String("ss"), Endpoint: backend},
			},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("missingEndpoints", t, func() {
		span2 := trace.Span{
			TraceID:   "1",
			ParentID:  pointer.String("1"),
			ID:        "2",
			Name:      pointer.String("foo"),
			Timestamp: pointer.Int64(1472470996199000),
			Duration:  pointer.Int64(207000),
			Tags:      map[string]string{},
		}

		span := InputSpan{
			Duration:  pointer.Float64(207000),
			Timestamp: pointer.Float64(1472470996199000),
			Span: trace.Span{
				TraceID:  "1",
				ParentID: pointer.String("1"),
				ID:       "2",
				Name:     pointer.String("foo"),
			},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("missingEndpoints_coreAnnotation", t, func() {
		span2 := trace.Span{
			TraceID:   "1",
			ParentID:  pointer.String("1"),
			ID:        "2",
			Name:      pointer.String("foo"),
			Timestamp: pointer.Int64(1472470996199000),
			Annotations: []*trace.Annotation{
				{Timestamp: pointer.Int64(1472470996199000), Value: pointer.String("sr")},
			},
			Tags: map[string]string{},
		}

		span := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Span: trace.Span{
				TraceID:  "1",
				ParentID: pointer.String("1"),
				ID:       "2",
				Name:     pointer.String("foo"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("sr"), Endpoint: nil},
			},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("incomplete_only_sr", t, func() {
		span2 := trace.Span{
			TraceID:       "1",
			ParentID:      pointer.String("1"),
			ID:            "2",
			Name:          pointer.String("foo"),
			Kind:          &ServerKind,
			Timestamp:     pointer.Int64(1472470996199000),
			Shared:        &trueVar,
			LocalEndpoint: backend,
			Tags:          map[string]string{},
		}

		span := InputSpan{
			Span: trace.Span{
				TraceID:  "1",
				ParentID: pointer.String("1"),
				ID:       "2",
				Name:     pointer.String("foo"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("sr"), Endpoint: backend},
			},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("lateRemoteEndpoint_ss", t, func() {
		span2 := trace.Span{
			TraceID:        "1",
			ParentID:       pointer.String("1"),
			ID:             "2",
			Name:           pointer.String("foo"),
			Kind:           &ServerKind,
			LocalEndpoint:  backend,
			RemoteEndpoint: frontend,
			Annotations: []*trace.Annotation{
				{Timestamp: pointer.Int64(1472470996199000), Value: pointer.String("ss")},
			},
			Tags: map[string]string{},
		}

		span := InputSpan{
			Span: trace.Span{
				TraceID:  "1",
				ParentID: pointer.String("1"),
				ID:       "2",
				Name:     pointer.String("foo"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("ss"), Endpoint: backend},
			},
			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("ca"), Value: interfaceAddr(true), Endpoint: frontend},
			},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	/** Late flushed data on a server span */
	Convey("lateRemoteEndpoint_ca", t, func() {
		span2 := trace.Span{
			TraceID:        "1",
			ParentID:       pointer.String("1"),
			ID:             "2",
			Name:           pointer.String("foo"),
			Kind:           &ServerKind,
			RemoteEndpoint: frontend,
			Tags:           map[string]string{},
		}

		span := InputSpan{
			Span: trace.Span{
				TraceID:  "1",
				ParentID: pointer.String("1"),
				ID:       "2",
				Name:     pointer.String("foo"),
			},
			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("ca"), Value: interfaceAddr(true), Endpoint: frontend},
			},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("lateRemoteEndpoint_cr", t, func() {
		span2 := trace.Span{
			TraceID:        "1",
			ParentID:       pointer.String("1"),
			ID:             "2",
			Name:           pointer.String("foo"),
			Kind:           &ClientKind,
			LocalEndpoint:  frontend,
			RemoteEndpoint: backend,
			Annotations: []*trace.Annotation{
				{Timestamp: pointer.Int64(1472470996199000), Value: pointer.String("cr")},
			},
			Tags: map[string]string{},
		}

		span := InputSpan{
			Span: trace.Span{
				TraceID:  "1",
				ParentID: pointer.String("1"),
				ID:       "2",
				Name:     pointer.String("foo"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("cr"), Endpoint: frontend},
			},
			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("sa"), Value: interfaceAddr(true), Endpoint: backend},
			},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("lateRemoteEndpoint_sa", t, func() {
		span2 := trace.Span{
			TraceID:        "1",
			ParentID:       pointer.String("1"),
			ID:             "2",
			Name:           pointer.String("foo"),
			Kind:           &ClientKind,
			RemoteEndpoint: backend,
			Tags:           map[string]string{},
		}

		span := InputSpan{
			Span: trace.Span{
				TraceID:  "1",
				ParentID: pointer.String("1"),
				ID:       "2",
				Name:     pointer.String("foo"),
			},
			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("sa"), Value: interfaceAddr(true), Endpoint: backend},
			},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("localSpan_emptyComponent", t, func() {
		simpleLocal := trace.Span{
			TraceID:       "1",
			ParentID:      pointer.String("1"),
			ID:            "2",
			Name:          pointer.String("local"),
			LocalEndpoint: frontend,
			Timestamp:     pointer.Int64(1472470996199000),
			Duration:      pointer.Int64(207000),
			Tags:          map[string]string{},
		}

		local := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Duration:  pointer.Float64(207000),
			Span: trace.Span{
				TraceID:  "1",
				ParentID: pointer.String("1"),
				ID:       "2",
				Name:     pointer.String("local"),
			},

			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("lc"), Value: interfaceAddr(""), Endpoint: frontend},
			},
		}

		sp, _ := local.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&simpleLocal})
	})

	Convey("clientAndServer", t, func() {
		noNameService := &trace.Endpoint{
			ServiceName: nil,
			Ipv4:        pointer.String("127.0.0.1"),
		}
		shared := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Duration:  pointer.Float64(207000),
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("get"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("cs"), Endpoint: noNameService},
				{Timestamp: pointer.Float64(1472470996238000), Value: pointer.String("ws"), Endpoint: noNameService},
				{Timestamp: pointer.Float64(1472470996250000), Value: pointer.String("sr"), Endpoint: backend},
				{Timestamp: pointer.Float64(1472470996350000), Value: pointer.String("ss"), Endpoint: backend},
				{Timestamp: pointer.Float64(1472470996403000), Value: pointer.String("wr"), Endpoint: noNameService},
				{Timestamp: pointer.Float64(1472470996406000), Value: pointer.String("cr"), Endpoint: noNameService},
			},

			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("http_path"), Value: interfaceAddr("/api"), Endpoint: noNameService},
				{Key: pointer.String("http_path"), Value: interfaceAddr("/backend"), Endpoint: backend},
				{Key: pointer.String("clnt/finagle.version"), Value: interfaceAddr("6.45.0"), Endpoint: noNameService},
				{Key: pointer.String("srv/finagle.version"), Value: interfaceAddr("6.44.0"), Endpoint: backend},
				{Key: pointer.String("ca"), Value: interfaceAddr(true), Endpoint: noNameService},
				{Key: pointer.String("sa"), Value: interfaceAddr(true), Endpoint: backend},
			},
		}

		// the client side owns timestamp and duration
		client := trace.Span{
			TraceID:        "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:       pointer.String("6b221d5bc9e6496c"),
			ID:             "5b4185666d50f68b",
			Name:           pointer.String("get"),
			Kind:           &ClientKind,
			LocalEndpoint:  noNameService,
			RemoteEndpoint: backend,
			Timestamp:      pointer.Int64(1472470996199000),
			Duration:       pointer.Int64(207000),
			Annotations: []*trace.Annotation{
				{Timestamp: pointer.Int64(1472470996238000), Value: pointer.String("ws")},
				{Timestamp: pointer.Int64(1472470996403000), Value: pointer.String("wr")},
			},
			Tags: map[string]string{
				"http_path":            "/api",
				"clnt/finagle.version": "6.45.0",
			},
		}

		// notice server tags are different than the client, and the client's annotations aren't here
		server := trace.Span{
			TraceID:        "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:       pointer.String("6b221d5bc9e6496c"),
			ID:             "5b4185666d50f68b",
			Name:           pointer.String("get"),
			Kind:           &ServerKind,
			Shared:         &trueVar,
			LocalEndpoint:  backend,
			RemoteEndpoint: noNameService,
			Timestamp:      pointer.Int64(1472470996250000),
			Duration:       pointer.Int64(100000),
			Tags: map[string]string{
				"http_path":           "/backend",
				"srv/finagle.version": "6.44.0",
			},
		}

		sp, _ := shared.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&client, &server})
	})

	/**
	 * The old span format had no means of saying it is shared or not. This uses lack of timestamp as
	 * a signal
	 */
	Convey("assumesServerWithoutTimestampIsShared", t, func() {
		span := InputSpan{
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("get"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996250000), Value: pointer.String("sr"), Endpoint: backend},
				{Timestamp: pointer.Float64(1472470996350000), Value: pointer.String("ss"), Endpoint: backend},
			},
		}

		span2 := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:      pointer.String("6b221d5bc9e6496c"),
			ID:            "5b4185666d50f68b",
			Name:          pointer.String("get"),
			Kind:          &ServerKind,
			Shared:        &trueVar,
			LocalEndpoint: backend,
			Timestamp:     pointer.Int64(1472470996250000),
			Duration:      pointer.Int64(100000),
			Tags:          map[string]string{},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("clientAndServer_loopback", t, func() {
		shared := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Duration:  pointer.Float64(207000),
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("get"),
			},

			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("cs"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996250000), Value: pointer.String("sr"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996350000), Value: pointer.String("ss"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996406000), Value: pointer.String("cr"), Endpoint: frontend},
			},
		}

		client := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:      pointer.String("6b221d5bc9e6496c"),
			ID:            "5b4185666d50f68b",
			Name:          pointer.String("get"),
			Kind:          &ClientKind,
			LocalEndpoint: frontend,
			Timestamp:     pointer.Int64(1472470996199000),
			Duration:      pointer.Int64(207000),
			Tags:          map[string]string{},
		}

		server := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:      pointer.String("6b221d5bc9e6496c"),
			ID:            "5b4185666d50f68b",
			Name:          pointer.String("get"),
			Kind:          &ServerKind,
			Shared:        &trueVar,
			LocalEndpoint: frontend,
			Timestamp:     pointer.Int64(1472470996250000),
			Duration:      pointer.Int64(100000),
			Tags:          map[string]string{},
		}

		sp, _ := shared.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&client, &server})
	})

	Convey("oneway_loopback", t, func() {
		shared := InputSpan{
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("get"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("cs"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996250000), Value: pointer.String("sr"), Endpoint: frontend},
			},
		}

		client := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:      pointer.String("6b221d5bc9e6496c"),
			ID:            "5b4185666d50f68b",
			Name:          pointer.String("get"),
			Kind:          &ClientKind,
			LocalEndpoint: frontend,
			Timestamp:     pointer.Int64(1472470996199000),
			Tags:          map[string]string{},
		}

		server := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:      pointer.String("6b221d5bc9e6496c"),
			ID:            "5b4185666d50f68b",
			Name:          pointer.String("get"),
			Kind:          &ServerKind,
			Shared:        &trueVar,
			LocalEndpoint: frontend,
			Timestamp:     pointer.Int64(1472470996250000),
			Tags:          map[string]string{},
		}

		sp, _ := shared.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&client, &server})
	})

	Convey("producer", t, func() {
		span := InputSpan{
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("send"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("ms"), Endpoint: frontend},
			},
		}

		span2 := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:      pointer.String("6b221d5bc9e6496c"),
			ID:            "5b4185666d50f68b",
			Name:          pointer.String("send"),
			Kind:          &ProducerKind,
			LocalEndpoint: frontend,
			Timestamp:     pointer.Int64(1472470996199000),
			Tags:          map[string]string{},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("producer_remote", t, func() {
		span := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("send"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("ms"), Endpoint: frontend},
			},
			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("ma"), Value: interfaceAddr(true), Endpoint: kafka},
			},
		}

		span2 := trace.Span{
			TraceID:        "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:       pointer.String("6b221d5bc9e6496c"),
			ID:             "5b4185666d50f68b",
			Name:           pointer.String("send"),
			Kind:           &ProducerKind,
			LocalEndpoint:  frontend,
			Timestamp:      pointer.Int64(1472470996199000),
			RemoteEndpoint: kafka,
			Tags:           map[string]string{},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("producer_duration", t, func() {
		span := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Duration:  pointer.Float64(51000),
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("send"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("ms"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996250000), Value: pointer.String("ws"), Endpoint: frontend},
			},
		}

		span2 := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:      pointer.String("6b221d5bc9e6496c"),
			ID:            "5b4185666d50f68b",
			Name:          pointer.String("send"),
			Kind:          &ProducerKind,
			LocalEndpoint: frontend,
			Timestamp:     pointer.Int64(1472470996199000),
			Duration:      pointer.Int64(51000),
			Tags:          map[string]string{},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("consumer", t, func() {
		span := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("send"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("mr"), Endpoint: frontend},
			},
		}

		span2 := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:      pointer.String("6b221d5bc9e6496c"),
			ID:            "5b4185666d50f68b",
			Name:          pointer.String("send"),
			Kind:          &ConsumerKind,
			LocalEndpoint: frontend,
			Timestamp:     pointer.Int64(1472470996199000),
			Tags:          map[string]string{},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("consumer_remote", t, func() {
		span := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("send"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("mr"), Endpoint: frontend},
			},
			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("ma"), Value: interfaceAddr(true), Endpoint: kafka},
			},
		}

		span2 := trace.Span{
			TraceID:        "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:       pointer.String("6b221d5bc9e6496c"),
			ID:             "5b4185666d50f68b",
			Name:           pointer.String("send"),
			Kind:           &ConsumerKind,
			LocalEndpoint:  frontend,
			RemoteEndpoint: kafka,
			Timestamp:      pointer.Int64(1472470996199000),
			Tags:           map[string]string{},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("consumer_duration", t, func() {
		span := InputSpan{
			Timestamp: pointer.Float64(1472470996199000),
			Duration:  pointer.Float64(51000),
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("send"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("wr"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996250000), Value: pointer.String("mr"), Endpoint: frontend},
			},
		}

		span2 := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:      pointer.String("6b221d5bc9e6496c"),
			ID:            "5b4185666d50f68b",
			Name:          pointer.String("send"),
			Kind:          &ConsumerKind,
			LocalEndpoint: frontend,
			Timestamp:     pointer.Int64(1472470996199000),
			Duration:      pointer.Int64(51000),
			Tags:          map[string]string{},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	/** shared span IDs for messaging spans isn't supported, but shouldn't break */
	Convey("producerAndConsumer", t, func() {
		shared := InputSpan{
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("whatev"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("ms"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996238000), Value: pointer.String("ws"), Endpoint: frontend},
				{Timestamp: nil, Value: pointer.String("wr"), Endpoint: backend},
				{Timestamp: pointer.Float64(1472470996406000), Value: pointer.String("mr"), Endpoint: backend},
			},
			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("ma"), Value: interfaceAddr(true), Endpoint: kafka},
			},
		}

		producer := trace.Span{
			TraceID:        "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:       pointer.String("6b221d5bc9e6496c"),
			ID:             "5b4185666d50f68b",
			Name:           pointer.String("whatev"),
			Kind:           &ProducerKind,
			LocalEndpoint:  frontend,
			RemoteEndpoint: kafka,
			Timestamp:      pointer.Int64(1472470996199000),
			Duration:       pointer.Int64(1472470996238000 - 1472470996199000),
			Tags:           map[string]string{},
		}

		consumer := trace.Span{
			TraceID:        "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:       pointer.String("6b221d5bc9e6496c"),
			ID:             "5b4185666d50f68b",
			Name:           pointer.String("whatev"),
			Kind:           &ConsumerKind,
			Shared:         &trueVar,
			LocalEndpoint:  backend,
			RemoteEndpoint: kafka,
			Timestamp:      pointer.Int64(1472470996406000),
			Duration:       nil,
			Tags:           map[string]string{},
		}

		sp, _ := shared.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&producer, &consumer})
	})

	/** shared span IDs for messaging spans isn't supported, but shouldn't break */
	Convey("producerAndConsumer_loopback_shared", t, func() {
		shared := InputSpan{
			Span: trace.Span{
				TraceID:  "7180c278b62e8f6a216a2aea45d08fc9",
				ParentID: pointer.String("6b221d5bc9e6496c"),
				ID:       "5b4185666d50f68b",
				Name:     pointer.String("message"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("ms"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996238000), Value: pointer.String("ws"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996403000), Value: pointer.String("wr"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996406000), Value: pointer.String("mr"), Endpoint: frontend},
			},
		}

		producer := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:      pointer.String("6b221d5bc9e6496c"),
			ID:            "5b4185666d50f68b",
			Name:          pointer.String("message"),
			Kind:          &ProducerKind,
			LocalEndpoint: frontend,
			Timestamp:     pointer.Int64(1472470996199000),
			Duration:      pointer.Int64(1472470996238000 - 1472470996199000),
			Tags:          map[string]string{},
		}

		consumer := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ParentID:      pointer.String("6b221d5bc9e6496c"),
			ID:            "5b4185666d50f68b",
			Name:          pointer.String("message"),
			Kind:          &ConsumerKind,
			Shared:        &trueVar,
			LocalEndpoint: frontend,
			Timestamp:     pointer.Int64(1472470996403000),
			Duration:      pointer.Int64(1472470996406000 - 1472470996403000),
			Tags:          map[string]string{},
		}

		sp, _ := shared.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&producer, &consumer})
	})

	Convey("dataMissingEndpointGoesOnFirstSpan", t, func() {
		shared := InputSpan{
			Span: trace.Span{
				TraceID: "7180c278b62e8f6a216a2aea45d08fc9",
				ID:      "5b4185666d50f68b",
				Name:    pointer.String("missing"),
			},
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1472470996199000), Value: pointer.String("foo"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996238000), Value: pointer.String("bar"), Endpoint: frontend},
				{Timestamp: pointer.Float64(1472470996250000), Value: pointer.String("baz"), Endpoint: backend},
				{Timestamp: pointer.Float64(1472470996350000), Value: pointer.String("qux"), Endpoint: backend},
				{Timestamp: pointer.Float64(1472470996403000), Value: pointer.String("missing"), Endpoint: nil},
			},
			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("foo"), Value: interfaceAddr("bar"), Endpoint: frontend},
				{Key: pointer.String("baz"), Value: interfaceAddr("qux"), Endpoint: backend},
				{Key: pointer.String("missing"), Value: interfaceAddr(""), Endpoint: nil},
			},
		}

		first := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ID:            "5b4185666d50f68b",
			Name:          pointer.String("missing"),
			LocalEndpoint: frontend,
			Annotations: []*trace.Annotation{
				{Timestamp: pointer.Int64(1472470996199000), Value: pointer.String("foo")},
				{Timestamp: pointer.Int64(1472470996238000), Value: pointer.String("bar")},
				{Timestamp: pointer.Int64(1472470996403000), Value: pointer.String("missing")},
			},
			Tags: map[string]string{
				"foo":     "bar",
				"missing": "",
			},
		}

		second := trace.Span{
			TraceID:       "7180c278b62e8f6a216a2aea45d08fc9",
			ID:            "5b4185666d50f68b",
			Name:          pointer.String("missing"),
			LocalEndpoint: backend,
			Annotations: []*trace.Annotation{
				{Timestamp: pointer.Int64(1472470996250000), Value: pointer.String("baz")},
				{Timestamp: pointer.Int64(1472470996350000), Value: pointer.String("qux")},
			},
			Tags: map[string]string{
				"baz": "qux",
			},
		}

		sp, _ := shared.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&first, &second})
	})

	Convey("convertBinaryAnnotations", t, func() {
		span := InputSpan{
			Span: trace.Span{
				TraceID: "1",
				Name:    pointer.String("test"),
				ID:      "2",
			},
			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("bool"), Value: interfaceAddr(true), Endpoint: frontend},
				{Key: pointer.String("bytes"), Value: interfaceAddr([]byte("hello")), Endpoint: frontend},
				{Key: pointer.String("short"), Value: interfaceAddr(uint16(20)), Endpoint: frontend},
				{Key: pointer.String("int"), Value: interfaceAddr(int32(32800)), Endpoint: frontend},
				{Key: pointer.String("long"), Value: interfaceAddr(int64(2147483700)), Endpoint: frontend},
				{Key: pointer.String("double"), Value: interfaceAddr(3.1415), Endpoint: frontend},
				{Key: pointer.String("novalue"), Value: nil, Endpoint: frontend},
			},
		}

		span2 := trace.Span{
			TraceID:       "1",
			Name:          pointer.String("test"),
			ID:            "2",
			LocalEndpoint: frontend,
			Tags: map[string]string{
				"bool":   "true",
				"bytes":  "hello",
				"short":  "20",
				"int":    "32800",
				"long":   "2147483700",
				"double": "3.1415",
			},
		}

		sp, _ := span.fromZipkinV1()
		So(sp, ShouldResemble, []*trace.Span{&span2})
	})

	Convey("convertBadBinaryAnnotations", t, func() {
		span := InputSpan{
			Span: trace.Span{
				TraceID: "1",
				Name:    pointer.String("test"),
				ID:      "2",
			},
			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("badtype"), Value: interfaceAddr([]int{1, 2, 3}), Endpoint: frontend},
			},
		}

		_, err := span.fromZipkinV1()
		So(err.Error(), ShouldContainSubstring, "invalid binary annotation type")
	})

	Convey("test traceErrs", t, func() {
		var t *traceErrs
		t = t.Append(nil)
		So(t, ShouldBeNil)
	})
}

// nolint:funlen
func TestParseJaegerFromRequest(t *testing.T) {
	Convey("SignalFx / Zipkin v2 spans get converted to jaeger batches", t, func() {
		// the following test data comes from the github.com/signalfx/golib/trace/translator tests
		var sourceSpans = []*trace.Span{
			{
				TraceID:  "a2969a8955571a3f",
				ParentID: pointer.String("000000000068c4e3"),
				ID:       "0000000000147d98",
				Name:     pointer.String("get"),
				Kind:     &ServerKind,
				LocalEndpoint: &trace.Endpoint{
					ServiceName: pointer.String("api1"),
					Ipv4:        pointer.String("10.53.69.61"),
				},
				RemoteEndpoint: &trace.Endpoint{
					ServiceName: pointer.String("rtapi"),
					Ipv4:        pointer.String("192.53.69.61"),
					Port:        pointer.Int32(53931),
				},
				Timestamp: pointer.Int64(1485467191639875),
				Duration:  pointer.Int64(22938),
				Debug:     nil,
				Shared:    nil,
				Annotations: []*trace.Annotation{
					{Timestamp: pointer.Int64(1485467191639875), Value: pointer.String("{\"key1\":\"value1\",\"key2\":\"value2\"}")},
					{Timestamp: pointer.Int64(1485467191639875), Value: pointer.String("nothing")},
					{Timestamp: pointer.Int64(1485467191639875), Value: nil}, // this nil annotation should be dropped by v2AnnotationsToJaegerLogs
				},
				Tags: map[string]string{
					"http.url":       "http://127.0.0.1:15598/client_transactions",
					"someBool":       "true",
					"someFalseBool":  "false",
					"someDouble":     "129.8",
					"hostname":       "api246-sjc1",
					"jaeger.version": "Python-3.1.0",
				},
			},
			{
				TraceID:  "0000000000000001b2969a8955571a3f",
				ParentID: pointer.String("000000000068c4e3"),
				ID:       "00000021d092272e",
				Name:     pointer.String("post"),
				Kind:     &ClientKind,
				LocalEndpoint: &trace.Endpoint{
					ServiceName: pointer.String("api1"),
					Ipv4:        pointer.String("10.53.69.61"),
				},
				RemoteEndpoint: &trace.Endpoint{
					Ipv4: pointer.String("10.0.0.1"),
					Port: pointer.Int32(53931),
				},
				Timestamp:   pointer.Int64(1485467191639875),
				Duration:    pointer.Int64(22938),
				Debug:       nil,
				Shared:      nil,
				Annotations: []*trace.Annotation{},
				Tags: map[string]string{
					"hostname":       "api246-sjc1",
					"jaeger.version": "Python-3.1.0",
				},
			},
			{
				TraceID:  "c2969a8955571a3f",
				ParentID: pointer.String("000000000068c4e3"),
				ID:       "0031d092272e",
				Name:     pointer.String("post"),
				Kind:     &ConsumerKind,
				LocalEndpoint: &trace.Endpoint{
					ServiceName: pointer.String("api1"),
					Ipv4:        pointer.String("10.53.69.61"),
				},
				RemoteEndpoint: nil,
				Timestamp:      pointer.Int64(1485467191639875),
				Duration:       pointer.Int64(22938),
				Debug:          nil,
				Shared:         nil,
				Annotations:    []*trace.Annotation{},
				Tags: map[string]string{
					"hostname":       "api246-sjc1",
					"jaeger.version": "Python-3.1.0",
				},
			},
			{
				TraceID:  "d2969a8955571a3f",
				ParentID: pointer.String("000000000068c4e3"),
				ID:       "A8DE7706B08C3A",
				Name:     pointer.String("post"),
				Kind:     &ProducerKind,
				LocalEndpoint: &trace.Endpoint{
					ServiceName: pointer.String("api1"),
					Ipv4:        pointer.String("10.53.69.61"),
				},
				RemoteEndpoint: &trace.Endpoint{
					Ipv6: pointer.String("::1"),
				},
				Timestamp:   pointer.Int64(1485467191639875),
				Duration:    pointer.Int64(22938),
				Debug:       pointer.Bool(true),
				Shared:      nil,
				Annotations: []*trace.Annotation{},
				Tags: map[string]string{
					"hostname":       "api246-sjc1",
					"jaeger.version": "Python-3.1.0",
				},
			},
			{
				TraceID:  "e2969a8955571a3f",
				ParentID: pointer.String("000000000078c4e3"),
				ID:       "B8DE7706B08C3A",
				Name:     pointer.String("post"),
				Kind:     &ProducerKind,
				LocalEndpoint: &trace.Endpoint{
					ServiceName: pointer.String("api1"),
					Ipv4:        pointer.String("10.53.69.61"),
				},
				RemoteEndpoint: &trace.Endpoint{
					Ipv6: pointer.String("::1"),
				},
				Timestamp:   pointer.Int64(1485467191639875),
				Duration:    pointer.Int64(22938),
				Debug:       pointer.Bool(true),
				Shared:      nil,
				Annotations: []*trace.Annotation{},
				Tags: map[string]string{
					"elements":       "100",
					"hostname":       "api246-sjc1",
					"jaeger.version": "Python-3.1.0",
				},
			},
			{
				TraceID:  "f2969a8955671a3f",
				ParentID: pointer.String("000000000035c4e2"),
				ID:       "CFEC5BE6328C3A",
				Name:     pointer.String("post"),
				Kind:     &ProducerKind,
				LocalEndpoint: &trace.Endpoint{
					ServiceName: pointer.String("api2"),
					Ipv4:        pointer.String("10.53.69.70"),
				},
				RemoteEndpoint: &trace.Endpoint{
					Ipv6: pointer.String("::1"),
				},
				Debug:     pointer.Bool(true),
				Timestamp: pointer.Int64(1485467191639875),
				Duration:  pointer.Int64(22938),
				Tags: map[string]string{
					"hostname":       "api2-233",
					"jaeger.version": "Python-3.6.0",
				},
			},
			{
				TraceID: "fa281a8955571a3a",
				ID:      "DFEC5BE6328C3A",
				Name:    pointer.String("get"),
				Kind:    &ClientKind,
				LocalEndpoint: &trace.Endpoint{
					ServiceName: pointer.String("api3"),
					Ipv4:        pointer.String("10.53.67.53"),
				},
				RemoteEndpoint: &trace.Endpoint{
					Ipv6: pointer.String("::1"),
				},
				Timestamp: pointer.Int64(1485467191639875),
				Duration:  pointer.Int64(22938),
				Tags: map[string]string{
					"hostname":       "api3-sjc1",
					"jaeger.version": "Python-3.6.0",
				},
			},
		}

		wantFirstProcess := &jaegerpb.Process{
			ServiceName: "api1",
			Tags: []jaegerpb.KeyValue{
				{
					Key:   "ip",
					VType: jaegerpb.ValueType_STRING,
					VStr:  "10.53.69.61",
				},
				{
					Key:   "hostname",
					VType: jaegerpb.ValueType_STRING,
					VStr:  "api246-sjc1",
				},
				{
					Key:   "jaeger.version",
					VType: jaegerpb.ValueType_STRING,
					VStr:  "Python-3.1.0",
				},
			},
		}

		var wantPostRequest = []*jaegerpb.Span{
			{
				SpanID:        jaegerpb.SpanID(0x147d98),
				TraceID:       jaegerpb.TraceID{Low: 11715721395283892799},
				OperationName: "get",
				StartTime:     time.Date(2017, 01, 26, 21, 46, 31, 639875000, time.UTC),
				Duration:      time.Duration(22938000),
				Flags:         0,
				Process:       wantFirstProcess,
				ProcessID:     "",
				References: []jaegerpb.SpanRef{
					{
						TraceID: jaegerpb.TraceID{Low: 11715721395283892799},
						SpanID:  jaegerpb.SpanID(0x68c4e3),
						RefType: jaegerpb.SpanRefType_CHILD_OF,
					},
				},
				Tags: []jaegerpb.KeyValue{
					{
						Key:   "peer.ipv4",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "192.53.69.61",
					},
					{
						Key:    "peer.port",
						VType:  jaegerpb.ValueType_INT64,
						VInt64: 53931,
					},
					{
						Key:   "span.kind",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "server",
					},
					{
						Key:   "someFalseBool",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "false",
					},
					{
						Key:   "someDouble",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "129.8",
					},
					{
						Key:   "http.url",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "http://127.0.0.1:15598/client_transactions",
					},
					{
						Key:   "someBool",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "true",
					},
				},
				Logs: []jaegerpb.Log{
					{
						Timestamp: time.Date(2017, 01, 26, 21, 46, 31, 639875000, time.UTC),
						Fields: []jaegerpb.KeyValue{
							{
								Key:   "key1",
								VType: jaegerpb.ValueType_STRING,
								VStr:  "value1",
							},
							{
								Key:   "key2",
								VType: jaegerpb.ValueType_STRING,
								VStr:  "value2",
							},
						},
					},
					{
						Timestamp: time.Date(2017, 01, 26, 21, 46, 31, 639875000, time.UTC),
						Fields: []jaegerpb.KeyValue{
							{
								Key:   "annotation",
								VType: jaegerpb.ValueType_STRING,
								VStr:  "nothing",
							},
						},
					},
				},
				Warnings: nil,
			},
			{
				TraceID:       jaegerpb.TraceID{Low: 12868642899890739775, High: 1},
				SpanID:        jaegerpb.SpanID(0x21d092272e),
				OperationName: "post",
				StartTime:     time.Date(2017, 01, 26, 21, 46, 31, 639875000, time.UTC),
				Duration:      time.Microsecond * 22938,
				Process:       wantFirstProcess,
				References: []jaegerpb.SpanRef{{
					TraceID: jaegerpb.TraceID{Low: 12868642899890739775, High: 1},
					SpanID:  jaegerpb.SpanID(6866147),
					RefType: jaegerpb.SpanRefType_CHILD_OF,
				}},
				Tags: []jaegerpb.KeyValue{
					{
						Key:   "span.kind",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "client",
					},
					{
						Key:    "peer.port",
						VType:  jaegerpb.ValueType_INT64,
						VInt64: 53931,
					},
					{
						Key:   "peer.ipv4",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "10.0.0.1",
					},
				},
				Logs: []jaegerpb.Log{},
			}, {
				TraceID:       jaegerpb.TraceID{Low: 14021564404497586751},
				SpanID:        jaegerpb.SpanID(213952636718),
				OperationName: "post",
				StartTime:     time.Date(2017, 01, 26, 21, 46, 31, 639875000, time.UTC),
				Duration:      time.Microsecond * 22938,
				Process:       wantFirstProcess,
				References: []jaegerpb.SpanRef{{
					TraceID: jaegerpb.TraceID{Low: 14021564404497586751},
					SpanID:  jaegerpb.SpanID(6866147),
					RefType: jaegerpb.SpanRefType_CHILD_OF,
				}},
				Tags: []jaegerpb.KeyValue{
					{
						Key:   "span.kind",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "consumer",
					},
				},
				Logs: []jaegerpb.Log{},
			}, {

				TraceID:       jaegerpb.TraceID{Low: 15174485909104433727},
				SpanID:        jaegerpb.SpanID(47532398882098234),
				OperationName: "post",
				StartTime:     time.Date(2017, 01, 26, 21, 46, 31, 639875000, time.UTC),
				Duration:      time.Microsecond * 22938,
				Flags:         2,
				Process:       wantFirstProcess,
				References: []jaegerpb.SpanRef{
					{
						RefType: jaegerpb.SpanRefType_CHILD_OF,
						TraceID: jaegerpb.TraceID{Low: 15174485909104433727},
						SpanID:  jaegerpb.SpanID(6866147),
					},
				},
				Tags: []jaegerpb.KeyValue{
					{
						Key:   "span.kind",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "producer",
					},
					{
						Key:   "peer.ipv6",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "::1",
					},
				},
				Logs: []jaegerpb.Log{},
			}, {

				TraceID:       jaegerpb.TraceID{Low: 16327407413711280703},
				SpanID:        jaegerpb.SpanID(52035998509468730),
				OperationName: "post",
				StartTime:     time.Date(2017, 01, 26, 21, 46, 31, 639875000, time.UTC),
				Duration:      time.Microsecond * 22938,
				Flags:         2,
				Process:       wantFirstProcess,
				References: []jaegerpb.SpanRef{
					{
						RefType: jaegerpb.SpanRefType_CHILD_OF,
						TraceID: jaegerpb.TraceID{Low: 16327407413711280703},
						SpanID:  jaegerpb.SpanID(7914723),
					},
				},
				Tags: []jaegerpb.KeyValue{
					{
						Key:   "span.kind",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "producer",
					},
					{
						Key:   "peer.ipv6",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "::1",
					},
					{
						Key:   "elements",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "100",
					},
				},
				Logs: []jaegerpb.Log{},
			},
			{
				TraceID:       jaegerpb.TraceID{Low: 17480328918319176255},
				SpanID:        jaegerpb.SpanID(58525199627357242),
				OperationName: "post",
				StartTime:     time.Date(2017, 01, 26, 21, 46, 31, 639875000, time.UTC),
				Duration:      time.Microsecond * 22938,
				Flags:         2,
				Process: &jaegerpb.Process{
					ServiceName: "api2",
					Tags: []jaegerpb.KeyValue{
						{
							Key:   "ip",
							VType: jaegerpb.ValueType_STRING,
							VStr:  "10.53.69.70",
						},
						{
							Key:   "jaeger.version",
							VType: jaegerpb.ValueType_STRING,
							VStr:  "Python-3.6.0",
						},
						{
							Key:   "hostname",
							VType: jaegerpb.ValueType_STRING,
							VStr:  "api2-233",
						},
					},
				},
				References: []jaegerpb.SpanRef{
					{
						RefType: jaegerpb.SpanRefType_CHILD_OF,
						TraceID: jaegerpb.TraceID{Low: 17480328918319176255},
						SpanID:  jaegerpb.SpanID(0x35c4e2),
					},
				},
				Tags: []jaegerpb.KeyValue{
					{
						Key:   "span.kind",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "producer",
					},
					{
						Key:   "peer.ipv6",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "::1",
					},
				},
				Logs: []jaegerpb.Log{},
			},
			{
				TraceID:       jaegerpb.TraceID{Low: 18025686685695023674},
				SpanID:        jaegerpb.SpanID(63028799254727738),
				OperationName: "get",
				StartTime:     time.Date(2017, 01, 26, 21, 46, 31, 639875000, time.UTC),
				Duration:      time.Microsecond * 22938,
				Process: &jaegerpb.Process{
					ServiceName: "api3",
					Tags: []jaegerpb.KeyValue{
						{
							Key:   "ip",
							VType: jaegerpb.ValueType_STRING,
							VStr:  "10.53.67.53",
						},
						{
							Key:   "jaeger.version",
							VType: jaegerpb.ValueType_STRING,
							VStr:  "Python-3.6.0",
						},
						{
							Key:   "hostname",
							VType: jaegerpb.ValueType_STRING,
							VStr:  "api3-sjc1",
						},
					},
				},
				Tags: []jaegerpb.KeyValue{
					{
						Key:   "span.kind",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "client",
					},
					{
						Key:   "peer.ipv6",
						VType: jaegerpb.ValueType_STRING,
						VStr:  "::1",
					},
				},
				Logs: []jaegerpb.Log{},
			},
		}

		// make http request from sourceSpans
		payload, err := json.Marshal(sourceSpans)
		assert.Nil(t, err)
		req := httptest.NewRequest("POST", "/v1/trace", bytes.NewReader(payload))

		got, err := ParseJaegerSpansFromRequest(req)
		assert.Nil(t, err)
		assert.NotNil(t, got)

		//assert that the returned batches match the desired batches
		require.Equal(t, len(wantPostRequest), len(got))
		assertSpansAreEqual(t, wantPostRequest, got)
	})

	Convey("Bad SignalFx (Zipkin V2) trace with binary annotations returns an error", t, func() {
		var sourceSpans = []*InputSpan{
			{
				Span: trace.Span{
					TraceID:  "a2969a8955571a3f",
					ParentID: pointer.String("000000000068c4e3"),
					ID:       "0000000000147d98",
					Name:     pointer.String("get"),
					Kind:     &ServerKind,
					LocalEndpoint: &trace.Endpoint{
						ServiceName: pointer.String("api1"),
						Ipv4:        pointer.String("10.53.69.61"),
					},
					RemoteEndpoint: &trace.Endpoint{
						ServiceName: pointer.String("rtapi"),
						Ipv4:        pointer.String("192.53.69.61"),
						Port:        pointer.Int32(53931),
					},
					Timestamp: pointer.Int64(1485467191639875),
					Duration:  pointer.Int64(22938),
					Debug:     nil,
					Shared:    nil,
					Annotations: []*trace.Annotation{
						{Timestamp: pointer.Int64(1485467191639875), Value: pointer.String("{\"key1\":\"value1\",\"key2\":\"value2\"}")},
						{Timestamp: pointer.Int64(1485467191639875), Value: pointer.String("nothing")},
					},
					Tags: map[string]string{
						"http.url":       "http://127.0.0.1:15598/client_transactions",
						"someBool":       "true",
						"someFalseBool":  "false",
						"someDouble":     "129.8",
						"hostname":       "api246-sjc1",
						"jaeger.version": "Python-3.1.0",
					},
				},
				BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
					{Key: pointer.String("bool"), Value: interfaceAddr(true), Endpoint: &trace.Endpoint{
						ServiceName: pointer.String("frontend"),
						Ipv4:        pointer.String("127.0.0.1"),
					}},
				},
			},
		}

		// make http request from sourceSpans
		payload, err := json.Marshal(sourceSpans)
		assert.Nil(t, err)
		req := httptest.NewRequest("POST", "/v1/trace", bytes.NewReader(payload))

		// parse the request and get the jaeger batch map
		_, err = ParseJaegerSpansFromRequest(req)
		assert.Error(t, err)
	})

	Convey("Zipkin v1 with binary annotations gets converted", t, func() {
		frontend := &trace.Endpoint{
			ServiceName: pointer.String("frontend"),
			Ipv4:        pointer.String("127.0.0.1"),
		}

		span := []*InputSpan{
			{
				Span: trace.Span{
					TraceID: "1",
					Name:    pointer.String("test"),
					ID:      "2",
				},
				BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
					{Key: pointer.String("bool"), Value: interfaceAddr(true), Endpoint: frontend},
					{Key: pointer.String("bytes"), Value: interfaceAddr([]byte("hello")), Endpoint: frontend},
					{Key: pointer.String("short"), Value: interfaceAddr(uint16(20)), Endpoint: frontend},
					{Key: pointer.String("int"), Value: interfaceAddr(int32(32800)), Endpoint: frontend},
					{Key: pointer.String("long"), Value: interfaceAddr(int64(2147483700)), Endpoint: frontend},
					{Key: pointer.String("double"), Value: interfaceAddr(3.1415), Endpoint: frontend},
					{Key: pointer.String("novalue"), Value: nil, Endpoint: frontend},
				},
			},
		}

		want := []*jaegerpb.Span{
			{
				TraceID:       jaegerpb.NewTraceID(0x0, 0x1),
				SpanID:        jaegerpb.NewSpanID(2),
				OperationName: "test",
				Logs:          []jaegerpb.Log{},
				Tags: []jaegerpb.KeyValue{
					{
						Key:   "bool",
						VStr:  "true",
						VType: jaegerpb.ValueType_STRING,
					},
					{
						Key: "bytes",
						// byte arrays are json marshalled into base64
						VStr:  base64.StdEncoding.EncodeToString([]byte("hello")),
						VType: jaegerpb.ValueType_STRING,
					},
					{
						Key:   "short",
						VStr:  "20",
						VType: jaegerpb.ValueType_STRING,
					},
					{
						Key:   "int",
						VStr:  "32800",
						VType: jaegerpb.ValueType_STRING,
					},
					{
						Key:   "long",
						VStr:  "2147483700",
						VType: jaegerpb.ValueType_STRING,
					},
					{
						Key:   "double",
						VStr:  "3.1415",
						VType: jaegerpb.ValueType_STRING,
					},
				},
				Process: jaegerpb.NewProcess("frontend", []jaegerpb.KeyValue{
					{
						Key:   "ip",
						VStr:  "127.0.0.1",
						VType: jaegerpb.ValueType_STRING,
					},
				}),
			},
		}

		// make http request from sourceSpans
		payload, err := json.Marshal(span)
		assert.Nil(t, err)
		req := httptest.NewRequest("POST", "/v1/trace", bytes.NewReader(payload))

		// parse the request and get the jaeger batch map
		got, err := ParseJaegerSpansFromRequest(req)
		assert.Nil(t, err)
		assert.NotNil(t, got)
		assert.NotEmpty(t, got)

		//assert that the returned batches match the desired batches
		require.Equal(t, len(want), len(got))
		assertSpansAreEqual(t, want, got)
	})

	Convey("Bad Zipkin v1 with invalid binary annotation values returns an error", t, func() {
		span := `[
					{
						"traceId":"1",
						"name":"test",
						"id":"2",
						"binaryAnnotations":[
							{
								"endpoint":{
									"serviceName":"frontend",
									"ipv4":"127.0.0.1"
								},
								"key":"double",
								"value":["hello","world"]
							}
						]
					}
				]`
		// make http request from sourceSpans
		req := httptest.NewRequest("POST", "/v1/trace", strings.NewReader(span))

		// parse the request and get the jaeger batch map
		_, err := ParseJaegerSpansFromRequest(req)
		assert.Error(t, err)
	})

	Convey("malformed payload should return a marshal error", t, func() {
		req := httptest.NewRequest("POST", "/v1/trace", strings.NewReader("{{},"))

		// parse the request and get the jaeger batch map
		_, err := ParseJaegerSpansFromRequest(req)
		assert.Error(t, err)
	})
}

func sortSpans(spans []*jaegerpb.Span) {
	sort.Slice(spans, func(i, j int) bool {
		return spans[i].SpanID < spans[j].SpanID
	})
}

func sortRefs(t []jaegerpb.SpanRef) {
	sort.Slice(t, func(i, j int) bool {
		return t[i].String() <= t[j].String()
	})
}

func sortLogs(t []jaegerpb.Log) {
	sort.Slice(t, func(i, j int) bool {
		return t[i].String() <= t[j].String()
	})
	for i := 0; i < len(t); i++ {
		sortTags(t[i].Fields)
	}
}

func sortTags(t []jaegerpb.KeyValue) {
	if t == nil {
		return
	}
	sort.Slice(t, func(i, j int) bool {
		return t[i].Key <= t[j].Key
	})
}

func sortedSpan(s *jaegerpb.Span) *jaegerpb.Span {
	sortLogs(s.Logs)
	sortTags(s.Tags)
	sortRefs(s.References)
	if s.Process != nil {
		sortTags(s.Process.Tags)
	}
	sort.Strings(s.Warnings)
	return s
}

func assertSpansAreEqual(t *testing.T, want, got []*jaegerpb.Span) {
	sortSpans(got)
	sortSpans(want)
	for i := 0; i < len(got); i++ {
		require.Equal(t, sortedSpan(want[i]), sortedSpan(got[i]))
	}
}

var benchmarkResults interface{}

func BenchmarkParseJaegerFromRequest(b *testing.B) {
	var batches []*jaegerpb.Span

	// test data taken from github.com/signalfx/golib/trace/translator tests
	var sourceSpans = []*trace.Span{
		{
			TraceID:  "a2969a8955571a3f",
			ParentID: pointer.String("000000000068c4e3"),
			ID:       "0000000000147d98",
			Name:     pointer.String("get"),
			Kind:     &ServerKind,
			LocalEndpoint: &trace.Endpoint{
				ServiceName: pointer.String("api1"),
				Ipv4:        pointer.String("10.53.69.61"),
			},
			RemoteEndpoint: &trace.Endpoint{
				ServiceName: pointer.String("rtapi"),
				Ipv4:        pointer.String("192.53.69.61"),
				Port:        pointer.Int32(53931),
			},
			Timestamp: pointer.Int64(1485467191639875),
			Duration:  pointer.Int64(22938),
			Debug:     nil,
			Shared:    nil,
			Annotations: []*trace.Annotation{
				{Timestamp: pointer.Int64(1485467191639875), Value: pointer.String("{\"key1\":\"value1\",\"key2\":\"value2\"}")},
				{Timestamp: pointer.Int64(1485467191639875), Value: pointer.String("nothing")},
				{Timestamp: pointer.Int64(1485467191639875), Value: nil}, // this nil annotation should be dropped by v2AnnotationsToJaegerLogs
			},
			Tags: map[string]string{
				"http.url":       "http://127.0.0.1:15598/client_transactions",
				"someBool":       "true",
				"someFalseBool":  "false",
				"someDouble":     "129.8",
				"hostname":       "api246-sjc1",
				"jaeger.version": "Python-3.1.0",
			},
		},
		{
			TraceID:  "0000000000000001b2969a8955571a3f",
			ParentID: pointer.String("000000000068c4e3"),
			ID:       "00000021d092272e",
			Name:     pointer.String("post"),
			Kind:     &ClientKind,
			LocalEndpoint: &trace.Endpoint{
				ServiceName: pointer.String("api1"),
				Ipv4:        pointer.String("10.53.69.61"),
			},
			RemoteEndpoint: &trace.Endpoint{
				Ipv4: pointer.String("10.0.0.1"),
				Port: pointer.Int32(53931),
			},
			Timestamp:   pointer.Int64(1485467191639875),
			Duration:    pointer.Int64(22938),
			Debug:       nil,
			Shared:      nil,
			Annotations: []*trace.Annotation{},
			Tags: map[string]string{
				"hostname":       "api246-sjc1",
				"jaeger.version": "Python-3.1.0",
			},
		},
		{
			TraceID:  "c2969a8955571a3f",
			ParentID: pointer.String("000000000068c4e3"),
			ID:       "0031d092272e",
			Name:     pointer.String("post"),
			Kind:     &ConsumerKind,
			LocalEndpoint: &trace.Endpoint{
				ServiceName: pointer.String("api1"),
				Ipv4:        pointer.String("10.53.69.61"),
			},
			RemoteEndpoint: nil,
			Timestamp:      pointer.Int64(1485467191639875),
			Duration:       pointer.Int64(22938),
			Debug:          nil,
			Shared:         nil,
			Annotations:    []*trace.Annotation{},
			Tags: map[string]string{
				"hostname":       "api246-sjc1",
				"jaeger.version": "Python-3.1.0",
			},
		},
		{
			TraceID:  "d2969a8955571a3f",
			ParentID: pointer.String("000000000068c4e3"),
			ID:       "A8DE7706B08C3A",
			Name:     pointer.String("post"),
			Kind:     &ProducerKind,
			LocalEndpoint: &trace.Endpoint{
				ServiceName: pointer.String("api1"),
				Ipv4:        pointer.String("10.53.69.61"),
			},
			RemoteEndpoint: &trace.Endpoint{
				Ipv6: pointer.String("::1"),
			},
			Timestamp:   pointer.Int64(1485467191639875),
			Duration:    pointer.Int64(22938),
			Debug:       pointer.Bool(true),
			Shared:      nil,
			Annotations: []*trace.Annotation{},
			Tags: map[string]string{
				"hostname":       "api246-sjc1",
				"jaeger.version": "Python-3.1.0",
			},
		},
		{
			TraceID:  "e2969a8955571a3f",
			ParentID: pointer.String("000000000078c4e3"),
			ID:       "B8DE7706B08C3A",
			Name:     pointer.String("post"),
			Kind:     &ProducerKind,
			LocalEndpoint: &trace.Endpoint{
				ServiceName: pointer.String("api1"),
				Ipv4:        pointer.String("10.53.69.61"),
			},
			RemoteEndpoint: &trace.Endpoint{
				Ipv6: pointer.String("::1"),
			},
			Timestamp:   pointer.Int64(1485467191639875),
			Duration:    pointer.Int64(22938),
			Debug:       pointer.Bool(true),
			Shared:      nil,
			Annotations: []*trace.Annotation{},
			Tags: map[string]string{
				"elements":       "100",
				"hostname":       "api246-sjc1",
				"jaeger.version": "Python-3.1.0",
			},
		},
		{
			TraceID:  "f2969a8955671a3f",
			ParentID: pointer.String("000000000035c4e2"),
			ID:       "CFEC5BE6328C3A",
			Name:     pointer.String("post"),
			Kind:     &ProducerKind,
			LocalEndpoint: &trace.Endpoint{
				ServiceName: pointer.String("api2"),
				Ipv4:        pointer.String("10.53.69.70"),
			},
			RemoteEndpoint: &trace.Endpoint{
				Ipv6: pointer.String("::1"),
			},
			Debug:     pointer.Bool(true),
			Timestamp: pointer.Int64(1485467191639875),
			Duration:  pointer.Int64(22938),
			Tags: map[string]string{
				"hostname":       "api2-233",
				"jaeger.version": "Python-3.6.0",
			},
		},
		{
			TraceID: "fa281a8955571a3a",
			ID:      "DFEC5BE6328C3A",
			Name:    pointer.String("get"),
			Kind:    &ClientKind,
			LocalEndpoint: &trace.Endpoint{
				ServiceName: pointer.String("api3"),
				Ipv4:        pointer.String("10.53.67.53"),
			},
			RemoteEndpoint: &trace.Endpoint{
				Ipv6: pointer.String("::1"),
			},
			Timestamp: pointer.Int64(1485467191639875),
			Duration:  pointer.Int64(22938),
			Tags: map[string]string{
				"hostname":       "api3-sjc1",
				"jaeger.version": "Python-3.6.0",
			},
		},
	}
	payload, err := json.Marshal(sourceSpans)
	assert.Nil(b, err)

	for n := 0; n < b.N; n++ {
		batches, _ = ParseJaegerSpansFromRequest(httptest.NewRequest("POST", "/v1/trace", bytes.NewReader(payload)))
	}

	// This is to prevent a compiler optimization during the benchmark.
	// See https://dave.cheney.net/2013/06/30/how-to-write-benchmarks-in-go
	benchmarkResults = batches
}

func Test(t *testing.T) {
	var sourceSpans = []*InputSpan{
		{
			Span: trace.Span{
				TraceID:  "a2969a8955571a3f",
				ParentID: pointer.String("000000000068c4e3"),
				ID:       "0000000000147d98",
				Name:     pointer.String("get"),
				Kind:     &ServerKind,
				LocalEndpoint: &trace.Endpoint{
					ServiceName: pointer.String("api1"),
					Ipv4:        pointer.String("10.53.69.61"),
				},
				RemoteEndpoint: &trace.Endpoint{
					ServiceName: pointer.String("rtapi"),
					Ipv4:        pointer.String("192.53.69.61"),
					Port:        pointer.Int32(53931),
				},
				Debug:  nil,
				Shared: nil,
				Tags: map[string]string{
					"http.url":       "http://127.0.0.1:15598/client_transactions",
					"someBool":       "true",
					"someFalseBool":  "false",
					"someDouble":     "129.8",
					"hostname":       "api246-sjc1",
					"jaeger.version": "Python-3.1.0",
				},
			},
			Timestamp: pointer.Float64(1485467191639875),
			Duration:  pointer.Float64(22938),
			Annotations: []*signalfxformat.InputAnnotation{
				{Timestamp: pointer.Float64(1485467191639875), Value: pointer.String("{\"key1\":\"value1\",\"key2\":\"value2\"}")},
				{Timestamp: pointer.Float64(1485467191639875), Value: pointer.String("nothing")},
			},
			BinaryAnnotations: []*signalfxformat.BinaryAnnotation{
				{Key: pointer.String("bool"), Value: interfaceAddr(true), Endpoint: &trace.Endpoint{
					ServiceName: pointer.String("frontend"),
					Ipv4:        pointer.String("127.0.0.1"),
				}},
			},
		},
	}
	bb, _ := easyjson.Marshal((*signalfxformat.InputSpan)(sourceSpans[0]))
	fmt.Println(string(bb))

}
