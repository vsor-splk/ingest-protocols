package signalfx

import (
	"bytes"
	"context"
	"net/http"

	"github.com/gogo/protobuf/proto"
	"github.com/gorilla/mux"
	"github.com/mailru/easyjson"
	sfxmodel "github.com/signalfx/com_signalfx_metrics_protobuf/model"
	"github.com/signalfx/golib/v3/datapoint/dpsink"
	"github.com/signalfx/golib/v3/event"
	"github.com/signalfx/golib/v3/log"
	"github.com/signalfx/golib/v3/pointer"
	"github.com/signalfx/golib/v3/sfxclient"
	"github.com/signalfx/golib/v3/web"
	"github.com/signalfx/ingest-protocols/protocol"
	signalfxformat "github.com/signalfx/ingest-protocols/protocol/signalfx/format"
)

// ProtobufEventDecoderV2 decodes protocol buffers in signalfx's v2 format and sends them to Sink
type ProtobufEventDecoderV2 struct {
	Sink   dpsink.ESink
	Logger log.Logger
}

func (decoder *ProtobufEventDecoderV2) Read(ctx context.Context, req *http.Request) (err error) {
	jeff := buffs.Get().(*bytes.Buffer)
	defer buffs.Put(jeff)
	jeff.Reset()
	if err = protocol.ReadFromRequest(jeff, req, decoder.Logger); err != nil {
		return err
	}
	var msg sfxmodel.EventUploadMessage
	if err = proto.Unmarshal(jeff.Bytes(), &msg); err != nil {
		return err
	}
	evts := make([]*event.Event, 0, len(msg.GetEvents()))
	for _, protoDb := range msg.GetEvents() {
		if e, err1 := NewProtobufEvent(protoDb); err1 == nil {
			evts = append(evts, e)
		}
	}
	if len(evts) > 0 {
		err = decoder.Sink.AddEvents(ctx, evts)
	}
	return err
}

// JSONEventDecoderV2 decodes v2 json data for signalfx events and sends it to Sink
type JSONEventDecoderV2 struct {
	Sink   dpsink.ESink
	Logger log.Logger
}

func (decoder *JSONEventDecoderV2) Read(ctx context.Context, req *http.Request) error {
	var e signalfxformat.JSONEventV2
	if err := easyjson.UnmarshalFromReader(req.Body, &e); err != nil {
		return err
	}
	evts := make([]*event.Event, 0, len(e))
	for _, jsonEvent := range e {
		if jsonEvent.Category == nil {
			jsonEvent.Category = pointer.String("USER_DEFINED")
		}
		if jsonEvent.Timestamp == nil {
			jsonEvent.Timestamp = pointer.Int64(0)
		}
		cat := event.USERDEFINED
		if pbcat, ok := sfxmodel.EventCategory_value[*jsonEvent.Category]; ok {
			cat = event.Category(pbcat)
		}
		evt := event.NewWithProperties(jsonEvent.EventType, cat, jsonEvent.Dimensions, jsonEvent.Properties, fromTs(*jsonEvent.Timestamp))
		evts = append(evts, evt)
	}
	return decoder.Sink.AddEvents(ctx, evts)
}

func setupJSONEventV2(ctx context.Context, r *mux.Router, sink Sink, logger log.Logger, debugContext *web.HeaderCtxFlag, httpChain web.NextConstructor, counter *dpsink.Counter) sfxclient.Collector {
	additionalConstructors := []web.Constructor{}
	if debugContext != nil {
		additionalConstructors = append(additionalConstructors, debugContext)
	}
	handler, st := SetupChain(ctx, sink, "json_event_v2", func(s Sink) ErrorReader {
		return &JSONEventDecoderV2{Sink: s, Logger: logger}
	}, httpChain, logger, counter, additionalConstructors...)
	SetupJSONV2EventPaths(r, handler)
	return st
}

// SetupJSONV2EventPaths tells the router which paths the given handler (which should handle v2 protobufs)
func SetupJSONV2EventPaths(r *mux.Router, handler http.Handler) {
	SetupJSONByPaths(r, handler, "/v2/event")
}

func setupProtobufEventV2(ctx context.Context, r *mux.Router, sink Sink, logger log.Logger, debugContext *web.HeaderCtxFlag, httpChain web.NextConstructor, counter *dpsink.Counter) sfxclient.Collector {
	additionalConstructors := []web.Constructor{}
	if debugContext != nil {
		additionalConstructors = append(additionalConstructors, debugContext)
	}
	handler, st := SetupChain(ctx, sink, "protobuf_event_v2", func(s Sink) ErrorReader {
		return &ProtobufEventDecoderV2{Sink: s, Logger: logger}
	}, httpChain, logger, counter, additionalConstructors...)
	SetupProtobufV2EventPaths(r, handler)

	return st
}

// SetupProtobufV2EventPaths tells the router which paths the given handler (which should handle v2 protobufs)
func SetupProtobufV2EventPaths(r *mux.Router, handler http.Handler) {
	SetupProtobufV2ByPaths(r, handler, "/v2/event")
}
