package signalfx

import (
	"bytes"
	"sync"

	"github.com/signalfx/golib/v3/errors"
)

const (
	// DefaultLogPathV1 is the default listener endpoint path
	DefaultLogPathV1 = "/v1/log"
	// JaegerV1 binary thrift protocol
	JaegerV1 = "jaeger_thrift_v1"
	// DefaultTracePathV1 is the default listen path
	DefaultTracePathV1 = "/v1/trace"
	// ZipkinTracePathV1 adds /api/v1/spans endpoint
	ZipkinTracePathV1 = "/api/v1/spans"
	// ZipkinTracePathV2 adds /api/vw/spans endpoint
	ZipkinTracePathV2 = "/api/v2/spans"
	// ZipkinV1 is a constant used for protocol naming
	ZipkinV1 = "zipkin_json_v1"
)

// LogProtocol is the context type used to set what is the log protocol
type LogProtocol string

const (
	// HECV1Protocol is used to set LogProtocol value when the incoming data is hecv1
	HECV1Protocol LogProtocol = "hecv1"
	// LogProtocolType is used to set log protocol type
	LogProtocolType LogProtocol = "protocol"
)

// Constants as variables so it is easy to get a pointer to them
var (
	trueVar = true

	ClientKind   = "CLIENT"
	ServerKind   = "SERVER"
	ProducerKind = "PRODUCER"
	ConsumerKind = "CONSUMER"

	pads = []string{
		"",
		"0",
		"00",
		"000",
		"0000",
		"00000",
		"000000",
		"0000000",
		"00000000",
		"000000000",
		"0000000000",
		"00000000000",
		"000000000000",
		"0000000000000",
		"00000000000000",
		"000000000000000",
	}

	buffs = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	// ErrInvalidJaegerTraceFormat is an error returned when the payload cannot be parsed into jaeger thrift
	ErrInvalidJaegerTraceFormat = errors.New("invalid Jaeger format")
	// ErrUnableToReadRequest is an error returned when the request payload can't be read
	ErrUnableToReadRequest = errors.New("could not read request body")

	errInvalidJSONFormat = errors.New("invalid JSON format; please see correct format at https://developers.signalfx.com/ingest_data_reference.html")
)
