package signalfx

import (
	"fmt"

	"time"

	"errors"

	sfxmodel "github.com/signalfx/com_signalfx_metrics_protobuf/model"
	"github.com/signalfx/golib/v3/datapoint"
	"github.com/signalfx/golib/v3/event"
	signalfxformat "github.com/signalfx/ingest-protocols/protocol/signalfx/format"
)

// ValueToSend is an alias
type ValueToSend signalfxformat.ValueToSend

// NewDatumValue creates new datapoint value referenced from a value of the datum protobuf
func NewDatumValue(val sfxmodel.Datum) datapoint.Value {
	if val.DoubleValue != nil {
		return datapoint.NewFloatValue(val.GetDoubleValue())
	}
	if val.IntValue != nil {
		return datapoint.NewIntValue(val.GetIntValue())
	}
	return datapoint.NewStringValue(val.GetStrValue())
}

// ValueToValue converts the v2 JSON value to a core api Value
func ValueToValue(v ValueToSend) (datapoint.Value, error) {
	if v != nil {
		f, ok := v.(float64)
		if ok {
			if f == float64(int64(f)) {
				return datapoint.NewIntValue(int64(f)), nil
			}
			return datapoint.NewFloatValue(f), nil
		}
		i, ok := v.(int64)
		if ok {
			return datapoint.NewIntValue(i), nil
		}
		i2, ok := v.(int)
		if ok {
			return datapoint.NewIntValue(int64(i2)), nil
		}
		s, ok := v.(string)
		if ok {
			return datapoint.NewStringValue(s), nil
		}
	}
	return nil, fmt.Errorf("unable to convert value: %s", v)
}

// MetricCreationStruct is the API format for /v1/metric POST
type MetricCreationStruct struct {
	MetricName string `json:"sf_metric"`
	MetricType string `json:"sf_metricType"`
}

// MetricCreationResponse is the API response for /v1/metric POST
type MetricCreationResponse struct {
	Code    int    `json:"code,omitempty"`
	Error   bool   `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

var fromMTMap = map[sfxmodel.MetricType]datapoint.MetricType{
	sfxmodel.MetricType_CUMULATIVE_COUNTER: datapoint.Counter,
	sfxmodel.MetricType_GAUGE:              datapoint.Gauge,
	sfxmodel.MetricType_COUNTER:            datapoint.Count,
}

func fromMT(mt sfxmodel.MetricType) datapoint.MetricType {
	ret, exists := fromMTMap[mt]
	if exists {
		return ret
	}
	panic(fmt.Sprintf("Unknown metric type: %v\n", mt))
}

func fromTs(ts int64) time.Time {
	if ts > 0 {
		return time.Unix(0, ts*time.Millisecond.Nanoseconds())
	}
	return time.Now().Add(-time.Duration(time.Millisecond.Nanoseconds() * ts))
}

var errDatapointValueNotSet = errors.New("datapoint value not set")

// NewProtobufDataPointWithType creates a new datapoint from SignalFx's protobuf definition (backwards compatable with old API)
func NewProtobufDataPointWithType(dp *sfxmodel.DataPoint, mType sfxmodel.MetricType) (*datapoint.Datapoint, error) {
	var mt sfxmodel.MetricType

	// TODO make this a method?
	if dp.GetValue().StrValue == nil && dp.GetValue().IntValue == nil && dp.GetValue().DoubleValue == nil {
		return nil, errDatapointValueNotSet
	}
	if dp.MetricType != nil {
		mt = dp.GetMetricType()
	} else {
		mt = mType
	}

	dims := make(map[string]string, len(dp.GetDimensions())+1)
	if dp.GetSource() != "" {
		dims["sf_source"] = dp.GetSource()
	}

	dpdims := dp.GetDimensions()
	for _, dpdim := range dpdims {
		dims[dpdim.GetKey()] = dpdim.GetValue()
	}

	dpToRet := datapoint.New(dp.GetMetric(), dims, NewDatumValue(dp.GetValue()), fromMT(mt), fromTs(dp.GetTimestamp()))
	return dpToRet, nil
}

// PropertyAsRawType converts a protobuf property to a native Go type
func PropertyAsRawType(p *sfxmodel.PropertyValue) interface{} {
	if p == nil {
		return nil
	}
	if p.BoolValue != nil {
		return *p.BoolValue
	}
	if p.DoubleValue != nil {
		return *p.DoubleValue
	}
	if p.StrValue != nil {
		return *p.StrValue
	}
	if p.IntValue != nil {
		return *p.IntValue
	}
	return nil
}

var errPropertyValueNotSet = errors.New("property value not set")

// NewProtobufEvent creates a new event from SignalFx's protobuf definition
func NewProtobufEvent(e *sfxmodel.Event) (*event.Event, error) {
	dims := make(map[string]string, len(e.GetDimensions())+1)
	edims := e.GetDimensions()
	for _, dpdim := range edims {
		dims[dpdim.GetKey()] = dpdim.GetValue()
	}

	// sure wish protobuf has something eqiv of interface{} instead of only union structs...
	// <sigh>
	props := make(map[string]interface{}, len(e.GetDimensions())+1)
	for _, dpdim := range e.GetProperties() {
		pval := dpdim.GetValue()
		pkey := dpdim.GetKey()
		if pval.StrValue != nil {
			props[pkey] = pval.GetStrValue()
		} else if pval.BoolValue != nil {
			props[pkey] = pval.GetBoolValue()
		} else if pval.DoubleValue != nil {
			props[pkey] = pval.GetDoubleValue()
		} else if pval.IntValue != nil {
			props[pkey] = pval.GetIntValue()
		} else {
			return nil, errPropertyValueNotSet
		}
	}

	cat := event.ToProtoEC(e.GetCategory())
	return event.NewWithProperties(e.GetEventType(), cat, dims, props, fromTs(e.GetTimestamp())), nil
}
