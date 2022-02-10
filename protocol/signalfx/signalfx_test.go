package signalfx

import (
	"errors"
	"fmt"
	"github.com/signalfx/golib/v3/event"
	"math"
	"testing"

	"github.com/gogo/protobuf/proto"
	sfxmodel "github.com/signalfx/com_signalfx_metrics_protobuf/model"
	"github.com/signalfx/golib/v3/datapoint"
	"github.com/signalfx/golib/v3/pointer"
	signalfxformat "github.com/signalfx/ingest-protocols/protocol/signalfx/format"
	signalfxlog "github.com/signalfx/ingest-protocols/protocol/signalfx/format/log"
	. "github.com/smartystreets/goconvey/convey"
)

func TestValueToValue(t *testing.T) {
	testVal := func(toSend interface{}, expected string) datapoint.Value {
		dv, err := ValueToValue(toSend)
		So(err, ShouldBeNil)
		So(expected, ShouldEqual, dv.String())
		return dv
	}
	Convey("v2v conversion", t, func() {
		Convey("test basic conversions", func() {
			testVal(int64(1), "1")
			testVal(float64(.2), "0.2")
			testVal(int(3), "3")
			testVal("4", "4")
			_, err := ValueToValue(errors.New("testing"))
			So(err, ShouldNotBeNil)
			_, err = ValueToValue(nil)
			So(err, ShouldNotBeNil)
		})
		Convey("show that maxfloat64 is too large to be a long", func() {
			dv := testVal(math.MaxFloat64, "179769313486231570000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")
			_, ok := dv.(datapoint.FloatValue)
			So(ok, ShouldBeTrue)

		})
		Convey("show that maxint32 will be a long", func() {
			dv := testVal(math.MaxInt32, "2147483647")
			_, ok := dv.(datapoint.IntValue)
			So(ok, ShouldBeTrue)
		})
		Convey("show that float(maxint64) will be a float due to edgyness of conversions", func() {
			dv := testVal(float64(math.MaxInt64), "9223372036854776000")
			_, ok := dv.(datapoint.FloatValue)
			So(ok, ShouldBeTrue)
		})
	})
}

func TestNewProtobufDataPointWithType(t *testing.T) {
	Convey("A nil datapoint value", t, func() {
		dp := sfxmodel.DataPoint{}
		Convey("should error when converted", func() {
			_, err := NewProtobufDataPointWithType(&dp, sfxmodel.MetricType_COUNTER)
			So(err, ShouldEqual, errDatapointValueNotSet)
		})
		Convey("with a value", func() {
			dp.Value = sfxmodel.Datum{
				IntValue: pointer.Int64(1),
			}
			Convey("source should set", func() {
				dp.Source = "hello"
				dp2, err := NewProtobufDataPointWithType(&dp, sfxmodel.MetricType_COUNTER)
				So(err, ShouldBeNil)
				So(dp2.Dimensions["sf_source"], ShouldEqual, "hello")
			})
		})
	})
}

func TestPropertyAsRawType(t *testing.T) {
	Convey("With value raw test PropertyAsRawType", t, func() {
		type testCase struct {
			v   *sfxmodel.PropertyValue
			exp interface{}
		}
		cases := []testCase{
			{
				v:   nil,
				exp: nil,
			},
			{
				v: &sfxmodel.PropertyValue{
					BoolValue: proto.Bool(false),
				},
				exp: false,
			},
			{
				v: &sfxmodel.PropertyValue{
					IntValue: proto.Int64(123),
				},
				exp: 123,
			},
			{
				v: &sfxmodel.PropertyValue{
					DoubleValue: proto.Float64(2.0),
				},
				exp: 2.0,
			},
			{
				v: &sfxmodel.PropertyValue{
					StrValue: proto.String("hello"),
				},
				exp: "hello",
			},
			{
				v:   &sfxmodel.PropertyValue{},
				exp: nil,
			},
		}
		for _, c := range cases {
			So(PropertyAsRawType(c.v), ShouldEqual, c.exp)
		}
	})
}

func TestBodySendFormatV2(t *testing.T) {
	Convey("BodySendFormatV2 should String()-ify", t, func() {
		x := signalfxformat.BodySendFormatV2{
			Metric: "hi",
		}
		So(x.String(), ShouldContainSubstring, "hi")
	})
}

func TestNewProtobufEvent(t *testing.T) {
	Convey("given a protobuf event with a nil property value", t, func() {
		protoEvent := &sfxmodel.Event{
			EventType:  "mwp.test2",
			Dimensions: []*sfxmodel.Dimension{},
			Properties: []*sfxmodel.Property{
				{
					Key:   "version",
					Value: &sfxmodel.PropertyValue{},
				},
			},
		}
		Convey("should error when converted", func() {
			_, err := NewProtobufEvent(protoEvent)
			So(err, ShouldEqual, errPropertyValueNotSet)
		})

	})
}

func TestFromMT(t *testing.T) {
	Convey("invalid fromMT types should panic", t, func() {
		So(func() {
			fromMT(sfxmodel.MetricType(1001))
		}, ShouldPanic)
	})
}

func TestNewDatumValue(t *testing.T) {
	Convey("datum values should convert", t, func() {
		Convey("string should convert", func() {
			s1 := "abc"
			So(s1, ShouldEqual, NewDatumValue(sfxmodel.Datum{StrValue: &s1}).(datapoint.StringValue).String())
		})
		Convey("floats should convert", func() {
			f1 := 1.2
			So(f1, ShouldEqual, NewDatumValue(sfxmodel.Datum{DoubleValue: &f1}).(datapoint.FloatValue).Float())
		})
		Convey("int should convert", func() {
			i1 := int64(3)
			So(i1, ShouldEqual, NewDatumValue(sfxmodel.Datum{IntValue: &i1}).(datapoint.IntValue).Int())
		})
	})
}

func TestResourceDimensions(t *testing.T) {
	Convey("given a resource map", t, func() {
			protoEvent := signalfxlog.KeyValueList{
				Values: []*signalfxlog.KeyValue{
					{
						Key:   "bool_key",
						Value: &signalfxlog.Value{Value: &signalfxlog.Value_BoolValue{BoolValue: true}},
					},
					{
						Key:   "int_key",
						Value: &signalfxlog.Value{Value: &signalfxlog.Value_IntValue{IntValue: 42}},
					},
					{
						Key:   "float_key",
						Value: &signalfxlog.Value{Value: &signalfxlog.Value_DoubleValue{DoubleValue: 42.24}},
					},
					{
						Key:   "string_key",
						Value: &signalfxlog.Value{Value: &signalfxlog.Value_StringValue{StringValue: "string value"}},
					},
					{
						Key: "array_key",
						Value: &signalfxlog.Value{
							Value: &signalfxlog.Value_ArrayValue{
								ArrayValue: &signalfxlog.ValueList{
									Values: []*signalfxlog.Value{
										{
											Value: &signalfxlog.Value_IntValue{IntValue: 12},
										},
									},
								},
							},
						},
					},
				},
			}

			res := createDimensionsFromResources(&protoEvent)
			fmt.Printf(res["array_values"])
			So(res["array_key"], ShouldEqual, "values:<int_value:12 > ")
			So(res["bool_key"], ShouldEqual, "true")
			So(res["int_key"], ShouldEqual, "42")
			So(res["float_key"], ShouldEqual, "42.24")
			So(res["string_key"], ShouldEqual, "string value")
			fmt.Printf("%v", res)

	})
}

func TestConvertTS(t *testing.T) {
	Convey("given an incoming timestamp", t, func(){
		Convey("empty timestamps should be rejected", func() {
			_, err := convertTs(nil)
			So(err, ShouldBeError, errTimestampNotSet)
		})

		Convey("zero timestamp should be rejected", func(){
			ts := &signalfxlog.TimeField{Value: &signalfxlog.TimeField_NumericValue{NumericValue: 0}}
			_, err := convertTs(ts)
			So(err, ShouldBeError, errTimestampNotSet)
		})

		Convey("empty string timestamp should be rejected", func(){
			ts := &signalfxlog.TimeField{Value: &signalfxlog.TimeField_StringValue{StringValue: ""}}
			_, err := convertTs(ts)
			So(err, ShouldBeError, errTimestampNotSet)
		})

		Convey("valid numerical timestamp should be accepted", func(){
			ts := &signalfxlog.TimeField{Value: &signalfxlog.TimeField_NumericValue{NumericValue: 1556793030000}}
			tsn, err := convertTs(ts)
			So(err, ShouldBeNil)
			So(tsn, ShouldEqual, 1556793030000)
		})

		Convey("valid string timestamp should be accepted", func(){
			ts := &signalfxlog.TimeField{Value: &signalfxlog.TimeField_StringValue{StringValue: "1556793030000"}}
			tsn, err := convertTs(ts)
			So(err, ShouldBeNil)
			So(tsn, ShouldEqual, 1556793030000)
		})

	})
}

func createTestResource(b bool, i int64, s string) *signalfxlog.KeyValueList {
	return &signalfxlog.KeyValueList{
		Values: []*signalfxlog.KeyValue{
			{
				Key:   "resource_bool_key",
				Value: &signalfxlog.Value{Value: &signalfxlog.Value_BoolValue{BoolValue: b}},
			},
			{
				Key:   "resource_int_key",
				Value: &signalfxlog.Value{Value: &signalfxlog.Value_IntValue{IntValue: i}},
			},
			{
				Key: "resource_string_key",
				Value: &signalfxlog.Value{Value: &signalfxlog.Value_StringValue{StringValue: s}},
			},
		},
	}
}

func createTestLogRecord(attrs *signalfxlog.KeyValueList) *signalfxlog.LogRecord {
	return &signalfxlog.LogRecord{
		Timestamp:            &signalfxlog.TimeField{
			Value: &signalfxlog.TimeField_NumericValue{
				NumericValue: 1556793030000,
			},
		},
		TraceID:              []byte("1234"),
		SpanID:               []byte("5678"),
		TraceFlags:           10,
		SeverityText:         "sev text",
		SeverityNumber:       8,
		Body:                 &signalfxlog.Value{
			Value: &signalfxlog.Value_StringValue{
				StringValue: "body value"},
		},
		Attributes: attrs,
	}

}

func TestCreateEventMeta(t *testing.T) {
	Convey("given a log record and resources", t, func(){
		res := createTestResource(true, 42, "string_value")
		lr := createTestLogRecord(createTestAttributes(16, true))
		meta := createEventMeta(lr, res)
		extras := meta["sfx_payload_extras"].(map[string]interface{})
		So(extras["TraceID"], ShouldResemble, lr.GetTraceID())
		So(extras["SpanID"], ShouldResemble, lr.GetSpanID())
		So(extras["TraceFlags"], ShouldEqual, lr.GetTraceFlags())
		So(extras["SeverityText"], ShouldEqual, lr.GetSeverityText())
		So(extras["SeverityNumber"], ShouldEqual, lr.GetSeverityNumber())
		So(extras["Body"], ShouldEqual, lr.GetBody())
		So(extras["Resource"], ShouldResemble, res.GetValues())
		So(meta["sfx_event_version"], ShouldEqual, "v3")
	})
}

func createTestAttributes(i int64, b bool) *signalfxlog.KeyValueList {
	return &signalfxlog.KeyValueList{
		Values: []*signalfxlog.KeyValue{
			{
				Key:   "bool_key",
				Value: &signalfxlog.Value{Value: &signalfxlog.Value_BoolValue{BoolValue: b}},
			},
			{
				Key:   "int_key",
				Value: &signalfxlog.Value{Value: &signalfxlog.Value_IntValue{IntValue: i}},
			},
		},
	}

}
func createTestAttributesWithEventInfo(et string, ec, i int64, b bool) *signalfxlog.KeyValueList{
	attrs := createTestAttributes(i, b)
	attrs.Values = append(attrs.GetValues(), &signalfxlog.KeyValue{
		Key: otelEventType,
		Value: &signalfxlog.Value{Value: &signalfxlog.Value_StringValue{StringValue: et}},
	})
	attrs.Values = append(attrs.GetValues(), &signalfxlog.KeyValue{
		 Key: otelCategory,
		 Value: &signalfxlog.Value{Value: &signalfxlog.Value_IntValue{IntValue: ec}},
	} )
	return attrs
}

func TestCreatePropertiesFromAttributes(t *testing.T) {
	Convey("given a list of attributes", t, func() {
		Convey("an property map with empty categories and type is created", func() {
		attrs :=           &signalfxlog.KeyValueList{
			Values: []*signalfxlog.KeyValue{
				{
					Key:   "bool_key",
					Value: &signalfxlog.Value{Value: &signalfxlog.Value_BoolValue{BoolValue: true}},
				},
				{
					Key:   "int_key",
					Value: &signalfxlog.Value{Value: &signalfxlog.Value_IntValue{IntValue: 42}},
				},
			},
		}
		props, tp, cat, err := createPropertiesFromAttributes(attrs)
		So(tp, ShouldEqual, "")
		So(cat, ShouldEqual, 0)
		So(err, ShouldBeNil)
		So(props["bool_key"], ShouldEqual, attrs.GetValues()[0].GetValue().GetBoolValue())
		So(props["int_key"], ShouldEqual, attrs.GetValues()[1].GetValue().GetIntValue())
		})

		Convey("a property map with nil values generates an error", func(){
			attrs :=           &signalfxlog.KeyValueList{
				Values: []*signalfxlog.KeyValue{
					{
						Key:   "bool_key",
						Value: nil,
					},
				},
			}
			_, _, _, err := createPropertiesFromAttributes(attrs)
			So(err, ShouldBeError, errPropertyValueNotSet)
		})

		Convey("a property map with type and category is created", func(){
			attrs := createTestAttributesWithEventInfo("event_test", 2, 42, true)
			props, tp, cat, err := createPropertiesFromAttributes(attrs)
			So(tp, ShouldEqual, "event_test")
			So(cat, ShouldEqual, 2)
			So(err, ShouldBeNil)
			So(props["bool_key"], ShouldEqual, attrs.GetValues()[0].GetValue().GetBoolValue())
			So(props["int_key"], ShouldEqual, attrs.GetValues()[1].GetValue().GetIntValue())
			// We shouldn't add the category and type to the properties map
			So(len(props), ShouldEqual, 2)
		})
	})
}

func TestNewProtobufEventV3(t *testing.T) {
	Convey("given a log record", t, func(){
		res1 := createTestResource(true, 42, "string_value")
		dims := createDimensionsFromResources(res1)
		//res2 := createTestResource(false, 24, "another_string")
		attrs1 := createTestAttributesWithEventInfo("event-type1", 2, 4, true)
		//attrs2 := createTestAttributesWithEventInfo("event-type2", 1, 10, false)
		Convey("accept a valid log record", func(){
			logRecord := createTestLogRecord(attrs1)
			e, err := NewProtobufEventV3(logRecord, res1, dims)
			So(err, ShouldBeNil)
			So(e.Category, ShouldEqual, event.USERDEFINED)
			So(len(e.Dimensions), ShouldEqual, 3)
			So(e.EventType, ShouldEqual, "event-type1")
			So(len(e.Properties), ShouldEqual, 2)

		})
		Convey("an invalid log record doesn't create an event", func(){
			logRecord := createTestLogRecord(attrs1)
			logRecord.Timestamp = nil
			e, err := NewProtobufEventV3(logRecord, res1, dims)
			So(err, ShouldBeError, errTimestampNotSet)
			So(e, ShouldBeNil)
		})
/*
		logRequest := signalfxlog.LogRequest{
			ResourceLogs:         []*signalfxlog.ResourceLogs{
				{
					Resource:             res1,
					LogRecords:           []*signalfxlog.LogRecord{
						createTestLogRecord(attrs1),
					},
				},
				{
					Resource: res2,
					LogRecords: []*signalfxlog.LogRecord{
						createTestLogRecord(attrs2),
					},
				},
			},
		}
 */

	})

		/*
		protoEvent := &sfxmodel.Event{
			EventType:  "mwp.test2",
			Dimensions: []*sfxmodel.Dimension{},
			Properties: []*sfxmodel.Property{
				{
					Key:   "version",
					Value: &sfxmodel.PropertyValue{},
				},
			},
		}
		Convey("should error when converted", func() {
			_, err := NewProtobufEvent(protoEvent)
			So(err, ShouldEqual, errPropertyValueNotSet)
		})


	})
		*/
}
