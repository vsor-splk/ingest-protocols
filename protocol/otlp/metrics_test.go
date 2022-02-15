// Copyright OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package otlp

import (
	"math"
	"testing"
	"time"

	"github.com/signalfx/golib/v3/datapoint"
	. "github.com/smartystreets/goconvey/convey"
	metricsservicev1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
)

const (
	unixSecs  = int64(1574092046)
	unixNSecs = int64(11 * time.Millisecond)
	tsMSecs   = unixSecs*1e3 + unixNSecs/1e6
)

var ts = time.Unix(unixSecs, unixNSecs)

func Test_FromMetrics(t *testing.T) {
	labelMap := map[string]string{
		"k0": "v0",
		"k1": "v1",
	}

	const doubleVal = 1234.5678
	makeDoublePt := func() *metricsv1.DoubleDataPoint {
		return &metricsv1.DoubleDataPoint{
			TimeUnixNano: uint64(ts.UnixNano()),
			Value:        doubleVal,
		}
	}

	makeDoublePtWithLabels := func() *metricsv1.DoubleDataPoint {
		pt := makeDoublePt()
		pt.Labels = stringMapToAttributeMap(labelMap)
		return pt
	}

	const int64Val = int64(123)
	makeInt64Pt := func() *metricsv1.IntDataPoint {
		return &metricsv1.IntDataPoint{
			TimeUnixNano: uint64(ts.UnixNano()),
			Value:        int64Val,
		}
	}

	makeInt64PtWithLabels := func() *metricsv1.IntDataPoint {
		pt := makeInt64Pt()
		pt.Labels = stringMapToAttributeMap(labelMap)
		return pt
	}

	histBounds := []float64{1, 2, 4}
	histCounts := []uint64{4, 2, 3, 7}

	makeDoubleHistDP := func() *metricsv1.DoubleHistogramDataPoint {
		return &metricsv1.DoubleHistogramDataPoint{
			TimeUnixNano:   uint64(ts.UnixNano()),
			Count:          16,
			Sum:            100.0,
			ExplicitBounds: histBounds,
			BucketCounts:   histCounts,
			Labels:         stringMapToAttributeMap(labelMap),
		}
	}
	doubleHistDP := makeDoubleHistDP()

	makeDoubleHistDPBadCounts := func() *metricsv1.DoubleHistogramDataPoint {
		return &metricsv1.DoubleHistogramDataPoint{
			TimeUnixNano:   uint64(ts.UnixNano()),
			Count:          16,
			Sum:            100.0,
			ExplicitBounds: histBounds,
			BucketCounts:   []uint64{4},
			Labels:         stringMapToAttributeMap(labelMap),
		}
	}

	makeIntHistDP := func() *metricsv1.IntHistogramDataPoint {
		return &metricsv1.IntHistogramDataPoint{
			TimeUnixNano:   uint64(ts.UnixNano()),
			Count:          16,
			Sum:            100,
			ExplicitBounds: histBounds,
			BucketCounts:   histCounts,
			Labels:         stringMapToAttributeMap(labelMap),
		}
	}
	intHistDP := makeIntHistDP()

	makeIntHistDPBadCounts := func() *metricsv1.IntHistogramDataPoint {
		return &metricsv1.IntHistogramDataPoint{
			TimeUnixNano:   uint64(ts.UnixNano()),
			Count:          16,
			Sum:            100,
			ExplicitBounds: histBounds,
			BucketCounts:   []uint64{4},
			Labels:         stringMapToAttributeMap(labelMap),
		}
	}

	makeHistDPNoBuckets := func() *metricsv1.IntHistogramDataPoint {
		return &metricsv1.IntHistogramDataPoint{
			Count:        2,
			Sum:          10,
			TimeUnixNano: uint64(ts.UnixNano()),
			Labels:       stringMapToAttributeMap(labelMap),
		}
	}
	histDPNoBuckets := makeHistDPNoBuckets()

	const summarySumVal = 123.4
	const summaryCountVal = 111

	makeSummaryDP := func() *metricsv1.DoubleSummaryDataPoint {
		summaryDP := &metricsv1.DoubleSummaryDataPoint{
			TimeUnixNano: uint64(ts.UnixNano()),
			Sum:          summarySumVal,
			Count:        summaryCountVal,
			Labels:       stringMapToAttributeMap(labelMap),
		}
		for i := 0; i < 4; i++ {
			summaryDP.QuantileValues = append(summaryDP.QuantileValues, &metricsv1.DoubleSummaryDataPoint_ValueAtQuantile{
				Quantile: 0.25 * float64(i+1),
				Value:    float64(i),
			})
		}
		return summaryDP
	}

	makeEmptySummaryDP := func() *metricsv1.DoubleSummaryDataPoint {
		return &metricsv1.DoubleSummaryDataPoint{
			TimeUnixNano: uint64(ts.UnixNano()),
			Sum:          summarySumVal,
			Count:        summaryCountVal,
			Labels:       stringMapToAttributeMap(labelMap),
		}
	}

	tests := []struct {
		name              string
		metricsFn         func() []*metricsv1.ResourceMetrics
		wantSfxDataPoints []*datapoint.Datapoint
	}{
		{
			name: "nil_node_nil_resources_no_dims",
			metricsFn: func() []*metricsv1.ResourceMetrics {
				out := &metricsv1.ResourceMetrics{}
				ilm := &metricsv1.InstrumentationLibraryMetrics{}
				out.InstrumentationLibraryMetrics = append(out.InstrumentationLibraryMetrics, ilm)

				ilm.Metrics = []*metricsv1.Metric{
					{
						Name: "gauge_double_with_no_dims",
						Data: &metricsv1.Metric_DoubleGauge{
							DoubleGauge: &metricsv1.DoubleGauge{
								DataPoints: []*metricsv1.DoubleDataPoint{
									makeDoublePt(),
								},
							},
						},
					},
					{
						Name: "gauge_int_with_no_dims",
						Data: &metricsv1.Metric_IntGauge{
							IntGauge: &metricsv1.IntGauge{
								DataPoints: []*metricsv1.IntDataPoint{
									makeInt64Pt(),
								},
							},
						},
					},
					{
						Name: "cumulative_double_with_no_dims",
						Data: &metricsv1.Metric_DoubleSum{
							DoubleSum: &metricsv1.DoubleSum{
								IsMonotonic:            true,
								AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
								DataPoints: []*metricsv1.DoubleDataPoint{
									makeDoublePt(),
								},
							},
						},
					},
					{
						Name: "cumulative_int_with_no_dims",
						Data: &metricsv1.Metric_IntSum{
							IntSum: &metricsv1.IntSum{
								IsMonotonic:            true,
								AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
								DataPoints: []*metricsv1.IntDataPoint{
									makeInt64Pt(),
								},
							},
						},
					},
					{
						Name: "delta_double_with_no_dims",
						Data: &metricsv1.Metric_DoubleSum{
							DoubleSum: &metricsv1.DoubleSum{
								IsMonotonic:            true,
								AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
								DataPoints: []*metricsv1.DoubleDataPoint{
									makeDoublePt(),
								},
							},
						},
					},
					{
						Name: "delta_int_with_no_dims",
						Data: &metricsv1.Metric_IntSum{
							IntSum: &metricsv1.IntSum{
								IsMonotonic:            true,
								AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
								DataPoints: []*metricsv1.IntDataPoint{
									makeInt64Pt(),
								},
							},
						},
					},
					{
						Name: "gauge_sum_double_with_no_dims",
						Data: &metricsv1.Metric_DoubleSum{
							DoubleSum: &metricsv1.DoubleSum{
								IsMonotonic: false,
								DataPoints: []*metricsv1.DoubleDataPoint{
									makeDoublePt(),
								},
							},
						},
					},
					{
						Name: "gauge_sum_int_with_no_dims",
						Data: &metricsv1.Metric_IntSum{
							IntSum: &metricsv1.IntSum{
								IsMonotonic: false,
								DataPoints: []*metricsv1.IntDataPoint{
									makeInt64Pt(),
								},
							},
						},
					},
				}

				return []*metricsv1.ResourceMetrics{out}
			},
			wantSfxDataPoints: []*datapoint.Datapoint{
				doubleSFxDataPoint("gauge_double_with_no_dims", datapoint.Gauge, nil, doubleVal),
				int64SFxDataPoint("gauge_int_with_no_dims", datapoint.Gauge, nil, int64Val),
				doubleSFxDataPoint("cumulative_double_with_no_dims", datapoint.Counter, nil, doubleVal),
				int64SFxDataPoint("cumulative_int_with_no_dims", datapoint.Counter, nil, int64Val),
				doubleSFxDataPoint("delta_double_with_no_dims", datapoint.Count, nil, doubleVal),
				int64SFxDataPoint("delta_int_with_no_dims", datapoint.Count, nil, int64Val),
				doubleSFxDataPoint("gauge_sum_double_with_no_dims", datapoint.Gauge, nil, doubleVal),
				int64SFxDataPoint("gauge_sum_int_with_no_dims", datapoint.Gauge, nil, int64Val),
			},
		},
		{
			name: "nil_node_and_resources_with_dims",
			metricsFn: func() []*metricsv1.ResourceMetrics {
				out := &metricsv1.ResourceMetrics{}
				ilm := &metricsv1.InstrumentationLibraryMetrics{}
				out.InstrumentationLibraryMetrics = append(out.InstrumentationLibraryMetrics, ilm)

				ilm.Metrics = []*metricsv1.Metric{
					{
						Name: "gauge_double_with_dims",
						Data: &metricsv1.Metric_DoubleGauge{
							DoubleGauge: &metricsv1.DoubleGauge{
								DataPoints: []*metricsv1.DoubleDataPoint{
									makeDoublePtWithLabels(),
								},
							},
						},
					},
					{
						Name: "gauge_int_with_dims",
						Data: &metricsv1.Metric_IntGauge{
							IntGauge: &metricsv1.IntGauge{
								DataPoints: []*metricsv1.IntDataPoint{
									makeInt64PtWithLabels(),
								},
							},
						},
					},
					{
						Name: "cumulative_double_with_dims",
						Data: &metricsv1.Metric_DoubleSum{
							DoubleSum: &metricsv1.DoubleSum{
								IsMonotonic:            true,
								AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
								DataPoints: []*metricsv1.DoubleDataPoint{
									makeDoublePtWithLabels(),
								},
							},
						},
					},
					{
						Name: "cumulative_int_with_dims",
						Data: &metricsv1.Metric_IntSum{
							IntSum: &metricsv1.IntSum{
								IsMonotonic:            true,
								AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
								DataPoints: []*metricsv1.IntDataPoint{
									makeInt64PtWithLabels(),
								},
							},
						},
					},
				}

				return []*metricsv1.ResourceMetrics{out}
			},
			wantSfxDataPoints: []*datapoint.Datapoint{
				doubleSFxDataPoint("gauge_double_with_dims", datapoint.Gauge, labelMap, doubleVal),
				int64SFxDataPoint("gauge_int_with_dims", datapoint.Gauge, labelMap, int64Val),
				doubleSFxDataPoint("cumulative_double_with_dims", datapoint.Counter, labelMap, doubleVal),
				int64SFxDataPoint("cumulative_int_with_dims", datapoint.Counter, labelMap, int64Val),
			},
		},
		{
			name: "with_node_resources_dims",
			metricsFn: func() []*metricsv1.ResourceMetrics {
				out := &metricsv1.ResourceMetrics{
					Resource: &resourcev1.Resource{
						Attributes: []*commonv1.KeyValue{
							{
								Key:   "k_r0",
								Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v_r0"}},
							},
							{
								Key:   "k_r1",
								Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v_r1"}},
							},
							{
								Key:   "k_n0",
								Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v_n0"}},
							},
							{
								Key:   "k_n1",
								Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v_n1"}},
							},
						},
					},
				}
				ilm := &metricsv1.InstrumentationLibraryMetrics{}
				out.InstrumentationLibraryMetrics = append(out.InstrumentationLibraryMetrics, ilm)

				ilm.Metrics = []*metricsv1.Metric{
					{
						Name: "gauge_double_with_dims",
						Data: &metricsv1.Metric_DoubleGauge{
							DoubleGauge: &metricsv1.DoubleGauge{
								DataPoints: []*metricsv1.DoubleDataPoint{
									makeDoublePtWithLabels(),
								},
							},
						},
					},
					{
						Name: "gauge_int_with_dims",
						Data: &metricsv1.Metric_IntGauge{
							IntGauge: &metricsv1.IntGauge{
								DataPoints: []*metricsv1.IntDataPoint{
									makeInt64PtWithLabels(),
								},
							},
						},
					},
				}
				return []*metricsv1.ResourceMetrics{out}
			},
			wantSfxDataPoints: []*datapoint.Datapoint{
				doubleSFxDataPoint(
					"gauge_double_with_dims",
					datapoint.Gauge,
					mergeStringMaps(map[string]string{
						"k_n0": "v_n0",
						"k_n1": "v_n1",
						"k_r0": "v_r0",
						"k_r1": "v_r1",
					}, labelMap),
					doubleVal),
				int64SFxDataPoint(
					"gauge_int_with_dims",
					datapoint.Gauge,
					mergeStringMaps(map[string]string{
						"k_n0": "v_n0",
						"k_n1": "v_n1",
						"k_r0": "v_r0",
						"k_r1": "v_r1",
					}, labelMap),
					int64Val),
			},
		},
		{
			name: "histograms",
			metricsFn: func() []*metricsv1.ResourceMetrics {
				out := &metricsv1.ResourceMetrics{}
				ilm := &metricsv1.InstrumentationLibraryMetrics{}
				out.InstrumentationLibraryMetrics = append(out.InstrumentationLibraryMetrics, ilm)

				ilm.Metrics = []*metricsv1.Metric{
					{
						Name: "int_histo",
						Data: &metricsv1.Metric_IntHistogram{
							IntHistogram: &metricsv1.IntHistogram{
								DataPoints: []*metricsv1.IntHistogramDataPoint{
									makeIntHistDP(),
								},
							},
						},
					},
					{
						Name: "int_delta_histo",
						Data: &metricsv1.Metric_IntHistogram{
							IntHistogram: &metricsv1.IntHistogram{
								AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
								DataPoints: []*metricsv1.IntHistogramDataPoint{
									makeIntHistDP(),
								},
							},
						},
					},
					{
						Name: "double_histo",
						Data: &metricsv1.Metric_DoubleHistogram{
							DoubleHistogram: &metricsv1.DoubleHistogram{
								DataPoints: []*metricsv1.DoubleHistogramDataPoint{
									makeDoubleHistDP(),
								},
							},
						},
					},
					{
						Name: "double_delta_histo",
						Data: &metricsv1.Metric_DoubleHistogram{
							DoubleHistogram: &metricsv1.DoubleHistogram{
								AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
								DataPoints: []*metricsv1.DoubleHistogramDataPoint{
									makeDoubleHistDP(),
								},
							},
						},
					},
					{
						Name: "double_histo_bad_counts",
						Data: &metricsv1.Metric_DoubleHistogram{
							DoubleHistogram: &metricsv1.DoubleHistogram{
								DataPoints: []*metricsv1.DoubleHistogramDataPoint{
									makeDoubleHistDPBadCounts(),
								},
							},
						},
					},
					{
						Name: "int_histo_bad_counts",
						Data: &metricsv1.Metric_IntHistogram{
							IntHistogram: &metricsv1.IntHistogram{
								DataPoints: []*metricsv1.IntHistogramDataPoint{
									makeIntHistDPBadCounts(),
								},
							},
						},
					},
				}
				return []*metricsv1.ResourceMetrics{out}
			},
			wantSfxDataPoints: mergeDPs(
				expectedFromIntHistogram("int_histo", labelMap, *intHistDP, false),
				expectedFromIntHistogram("int_delta_histo", labelMap, *intHistDP, true),
				expectedFromDoubleHistogram("double_histo", labelMap, *doubleHistDP, false),
				expectedFromDoubleHistogram("double_delta_histo", labelMap, *doubleHistDP, true),
				[]*datapoint.Datapoint{
					int64SFxDataPoint("double_histo_bad_counts_count", datapoint.Counter, labelMap, int64(doubleHistDP.Count)),
					doubleSFxDataPoint("double_histo_bad_counts", datapoint.Counter, labelMap, doubleHistDP.Sum),
				},
				[]*datapoint.Datapoint{
					int64SFxDataPoint("int_histo_bad_counts_count", datapoint.Counter, labelMap, int64(intHistDP.Count)),
					int64SFxDataPoint("int_histo_bad_counts", datapoint.Counter, labelMap, intHistDP.Sum),
				},
			),
		},
		{
			name: "distribution_no_buckets",
			metricsFn: func() []*metricsv1.ResourceMetrics {
				out := &metricsv1.ResourceMetrics{}
				ilm := &metricsv1.InstrumentationLibraryMetrics{}
				out.InstrumentationLibraryMetrics = append(out.InstrumentationLibraryMetrics, ilm)

				ilm.Metrics = []*metricsv1.Metric{
					{
						Name: "no_bucket_histo",
						Data: &metricsv1.Metric_IntHistogram{
							IntHistogram: &metricsv1.IntHistogram{
								DataPoints: []*metricsv1.IntHistogramDataPoint{
									makeHistDPNoBuckets(),
								},
							},
						},
					},
				}
				return []*metricsv1.ResourceMetrics{out}
			},
			wantSfxDataPoints: expectedFromIntHistogram("no_bucket_histo", labelMap, *histDPNoBuckets, false),
		},
		{
			name: "summaries",
			metricsFn: func() []*metricsv1.ResourceMetrics {
				out := &metricsv1.ResourceMetrics{}
				ilm := &metricsv1.InstrumentationLibraryMetrics{}
				out.InstrumentationLibraryMetrics = append(out.InstrumentationLibraryMetrics, ilm)

				ilm.Metrics = []*metricsv1.Metric{
					{
						Name: "summary",
						Data: &metricsv1.Metric_DoubleSummary{
							DoubleSummary: &metricsv1.DoubleSummary{
								DataPoints: []*metricsv1.DoubleSummaryDataPoint{
									makeSummaryDP(),
								},
							},
						},
					},
				}
				return []*metricsv1.ResourceMetrics{out}
			},
			wantSfxDataPoints: expectedFromSummary("summary", labelMap, summaryCountVal, summarySumVal),
		},
		{
			name: "empty_summary",
			metricsFn: func() []*metricsv1.ResourceMetrics {
				out := &metricsv1.ResourceMetrics{}
				ilm := &metricsv1.InstrumentationLibraryMetrics{}
				out.InstrumentationLibraryMetrics = append(out.InstrumentationLibraryMetrics, ilm)

				ilm.Metrics = []*metricsv1.Metric{
					{
						Name: "empty_summary",
						Data: &metricsv1.Metric_DoubleSummary{
							DoubleSummary: &metricsv1.DoubleSummary{
								DataPoints: []*metricsv1.DoubleSummaryDataPoint{
									makeEmptySummaryDP(),
								},
							},
						},
					},
				}
				return []*metricsv1.ResourceMetrics{out}
			},
			wantSfxDataPoints: expectedFromEmptySummary("empty_summary", labelMap, summaryCountVal, summarySumVal),
		},
	}
	for _, tt := range tests {
		Convey(tt.name, t, func() {
			rms := tt.metricsFn()
			gotSfxDataPoints, err := FromOTLPMetricRequest(&metricsservicev1.ExportMetricsServiceRequest{ResourceMetrics: rms})
			So(err, ShouldBeNil)
			So(tt.wantSfxDataPoints, ShouldResemble, gotSfxDataPoints)
		})
	}
}

func TestAttributesToDimensions(t *testing.T) {
	Convey("attributesToDimensions", t, func() {
		attrs := []*commonv1.KeyValue{
			{
				Key:   "a",
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "s"}},
			},
			{
				Key:   "b",
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: ""}},
			},
			{
				Key:   "c",
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: true}},
			},
			{
				Key:   "d",
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 44}},
			},
			{
				Key:   "e",
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: 45.1}},
			},
			{
				Key: "f",
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_ArrayValue{ArrayValue: &commonv1.ArrayValue{
					Values: []*commonv1.AnyValue{
						{Value: &commonv1.AnyValue_StringValue{StringValue: "n1"}},
						{Value: &commonv1.AnyValue_StringValue{StringValue: "n2"}},
					}}}},
			},
			{
				Key: "g",
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_KvlistValue{KvlistValue: &commonv1.KeyValueList{
					Values: []*commonv1.KeyValue{
						{Key: "k1", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "n1"}}},
						{Key: "k2", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: false}}},
						{Key: "k3", Value: nil},
						{Key: "k4", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: 40.3}}},
						{Key: "k5", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 41}}},
					}}}},
			},
			{
				Key:   "h",
				Value: nil,
			},
			{
				Key:   "i",
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: 0}},
			},
		}

		dims := attributesToDimensions(attrs)
		So(dims, ShouldResemble, map[string]string{
			"a": "s",
			"c": "true",
			"d": "44",
			"e": "45.1",
			"f": `["n1","n2"]`,
			"g": `{"k1":"n1","k2":false,"k3":null,"k4":40.3,"k5":41}`,
			"i": "0",
		})
	})
}

func doubleSFxDataPoint(
	metric string,
	metricType datapoint.MetricType,
	dims map[string]string,
	val float64,
) *datapoint.Datapoint {
	return &datapoint.Datapoint{
		Metric:     metric,
		Timestamp:  ts,
		Value:      datapoint.NewFloatValue(val),
		MetricType: metricType,
		Dimensions: cloneStringMap(dims),
	}
}

func int64SFxDataPoint(
	metric string,
	metricType datapoint.MetricType,
	dims map[string]string,
	val int64,
) *datapoint.Datapoint {
	return &datapoint.Datapoint{
		Metric:     metric,
		Timestamp:  ts,
		Value:      datapoint.NewIntValue(val),
		MetricType: metricType,
		Dimensions: cloneStringMap(dims),
	}
}

func expectedFromDoubleHistogram(
	metricName string,
	dims map[string]string,
	histDP metricsv1.DoubleHistogramDataPoint,
	isDelta bool,
) []*datapoint.Datapoint {
	buckets := histDP.GetBucketCounts()

	dps := make([]*datapoint.Datapoint, 0)

	typ := datapoint.Counter
	if isDelta {
		typ = datapoint.Count
	}

	dps = append(dps,
		int64SFxDataPoint(metricName+"_count", typ, dims, int64(histDP.GetCount())),
		doubleSFxDataPoint(metricName, typ, dims, histDP.GetSum()))

	explicitBounds := histDP.GetExplicitBounds()
	if explicitBounds == nil {
		return dps
	}
	for i := 0; i < len(explicitBounds); i++ {
		dimsCopy := cloneStringMap(dims)
		dimsCopy[upperBoundDimensionKey] = float64ToDimValue(explicitBounds[i])
		dps = append(dps, int64SFxDataPoint(metricName+"_bucket", typ, dimsCopy, int64(buckets[i])))
	}
	dimsCopy := cloneStringMap(dims)
	dimsCopy[upperBoundDimensionKey] = float64ToDimValue(math.Inf(1))
	dps = append(dps, int64SFxDataPoint(metricName+"_bucket", typ, dimsCopy, int64(buckets[len(buckets)-1])))
	return dps
}

func expectedFromIntHistogram(
	metricName string,
	dims map[string]string,
	histDP metricsv1.IntHistogramDataPoint,
	isDelta bool,
) []*datapoint.Datapoint {
	buckets := histDP.GetBucketCounts()

	dps := make([]*datapoint.Datapoint, 0)

	typ := datapoint.Counter
	if isDelta {
		typ = datapoint.Count
	}

	dps = append(dps,
		int64SFxDataPoint(metricName+"_count", typ, dims, int64(histDP.GetCount())),
		int64SFxDataPoint(metricName, typ, dims, histDP.GetSum()))

	explicitBounds := histDP.GetExplicitBounds()
	if explicitBounds == nil {
		return dps
	}
	for i := 0; i < len(explicitBounds); i++ {
		dimsCopy := cloneStringMap(dims)
		dimsCopy[upperBoundDimensionKey] = float64ToDimValue(explicitBounds[i])
		dps = append(dps, int64SFxDataPoint(metricName+"_bucket", typ, dimsCopy, int64(buckets[i])))
	}
	dimsCopy := cloneStringMap(dims)
	dimsCopy[upperBoundDimensionKey] = float64ToDimValue(math.Inf(1))
	dps = append(dps, int64SFxDataPoint(metricName+"_bucket", typ, dimsCopy, int64(buckets[len(buckets)-1])))
	return dps
}

func expectedFromSummary(name string, labelMap map[string]string, count int64, sumVal float64) []*datapoint.Datapoint {
	countName := name + "_count"
	countPt := int64SFxDataPoint(countName, datapoint.Counter, labelMap, count)
	sumPt := doubleSFxDataPoint(name, datapoint.Counter, labelMap, sumVal)
	out := []*datapoint.Datapoint{countPt, sumPt}
	quantileDimVals := []string{"0.25", "0.5", "0.75", "1"}
	for i := 0; i < 4; i++ {
		qDims := map[string]string{"quantile": quantileDimVals[i]}
		qPt := doubleSFxDataPoint(
			name+"_quantile",
			datapoint.Gauge,
			mergeStringMaps(labelMap, qDims),
			float64(i),
		)
		out = append(out, qPt)
	}
	return out
}

func expectedFromEmptySummary(name string, labelMap map[string]string, count int64, sumVal float64) []*datapoint.Datapoint {
	countName := name + "_count"
	countPt := int64SFxDataPoint(countName, datapoint.Counter, labelMap, count)
	sumPt := doubleSFxDataPoint(name, datapoint.Counter, labelMap, sumVal)
	return []*datapoint.Datapoint{countPt, sumPt}
}

func mergeDPs(dps ...[]*datapoint.Datapoint) []*datapoint.Datapoint {
	var out []*datapoint.Datapoint
	for i := range dps {
		out = append(out, dps[i]...)
	}
	return out
}

func cloneStringMap(m map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func stringMapToAttributeMap(m map[string]string) []*commonv1.StringKeyValue {
	ret := make([]*commonv1.StringKeyValue, 0, len(m))
	for k, v := range m {
		ret = append(ret, &commonv1.StringKeyValue{
			Key:   k,
			Value: v,
		})
	}
	return ret
}
