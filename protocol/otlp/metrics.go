// Copyright OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// This code is copied and modified directly from the OTEL Collector:
// https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/1d6309bb62264cc7e2dda076ed95385b1ddef28a/pkg/translator/signalfx/from_metrics.go

package otlp

import (
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/signalfx/golib/v3/datapoint"
	metricsservicev1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
)

var (
	// Some standard dimension keys.
	// upper bound dimension key for histogram buckets.
	upperBoundDimensionKey = "upper_bound"

	// infinity bound dimension value is used on all histograms.
	infinityBoundSFxDimValue = float64ToDimValue(math.Inf(1))
)

// FromOTLPMetricRequest converts the ResourceMetrics in an incoming request to SignalFx datapoints
func FromOTLPMetricRequest(md *metricsservicev1.ExportMetricsServiceRequest) []*datapoint.Datapoint {
	return FromOTLPResourceMetrics(md.GetResourceMetrics())
}

// FromMetrics converts OTLP ResourceMetrics to SignalFx datapoints.
func FromOTLPResourceMetrics(rms []*metricsv1.ResourceMetrics) []*datapoint.Datapoint {
	var sfxDps []*datapoint.Datapoint

	for _, rm := range rms {
		for _, ilm := range rm.GetInstrumentationLibraryMetrics() {
			for _, m := range ilm.GetMetrics() {
				sfxDps = append(sfxDps, FromMetric(m)...)
			}
		}

		extraDimensions := attributesToDimensions(rm.GetResource().GetAttributes())
		for i := range sfxDps {
			dpDims := sfxDps[i].Dimensions
			for k, v := range extraDimensions {
				if _, ok := dpDims[k]; !ok {
					dpDims[k] = v
				}
			}
		}
	}

	return sfxDps
}

// FromMetric converts a OTLP Metric to SignalFx datapoint(s).
func FromMetric(m *metricsv1.Metric) []*datapoint.Datapoint {
	var dps []*datapoint.Datapoint

	basePoint := &datapoint.Datapoint{
		Metric:     m.GetName(),
		MetricType: fromMetricTypeToMetricType(m),
	}

	data := m.GetData()
	switch data.(type) {
	case *metricsv1.Metric_Gauge:
		dps = convertNumberDataPoints(m.GetGauge().GetDataPoints(), basePoint)
	case *metricsv1.Metric_Sum:
		dps = convertNumberDataPoints(m.GetSum().GetDataPoints(), basePoint)
	case *metricsv1.Metric_Histogram:
		dps = convertHistogram(m.GetHistogram().GetDataPoints(), basePoint)
	case *metricsv1.Metric_ExponentialHistogram:
		// TODO: Add support for these
	case *metricsv1.Metric_Summary:
		dps = convertSummaryDataPoints(m.GetSummary().GetDataPoints(), m.GetName())
	}

	return dps
}

func fromMetricTypeToMetricType(m *metricsv1.Metric) datapoint.MetricType {
	data := m.GetData()
	switch data.(type) {
	case *metricsv1.Metric_Gauge:
		return datapoint.Gauge

	case *metricsv1.Metric_Sum:
		if !m.GetSum().GetIsMonotonic() {
			return datapoint.Gauge
		}
		if m.GetSum().GetAggregationTemporality() == metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA {
			return datapoint.Count
		}
		return datapoint.Counter

	case *metricsv1.Metric_Histogram:
		if m.GetHistogram().GetAggregationTemporality() == metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA {
			return datapoint.Count
		}
		return datapoint.Counter

	case *metricsv1.Metric_Summary:
		return datapoint.Counter
	}

	return datapoint.Gauge
}

func convertNumberDataPoints(in []*metricsv1.NumberDataPoint, basePoint *datapoint.Datapoint) []*datapoint.Datapoint {
	out := make([]*datapoint.Datapoint, 0, len(in))

	for _, inDp := range in {
		dp := *basePoint
		dp.Timestamp = time.Unix(0, int64(inDp.GetTimeUnixNano()))
		dp.Dimensions = attributesToDimensions(inDp.GetAttributes())

		dp.Value = numberToSignalFxValue(inDp)

		out = append(out, &dp)
	}
	return out
}

func convertHistogram(histDPs []*metricsv1.HistogramDataPoint, basePoint *datapoint.Datapoint) []*datapoint.Datapoint {
	var out []*datapoint.Datapoint

	for _, histDP := range histDPs {
		attrDims := attributesToDimensions(histDP.GetAttributes())
		ts := time.Unix(0, int64(histDP.GetTimeUnixNano()))

		countDP := *basePoint
		countDP.Metric = basePoint.Metric + "_count"
		countDP.Timestamp = ts
		countDP.Dimensions = attrDims
		count := int64(histDP.GetCount())
		countDP.Value = datapoint.NewIntValue(count)

		sumDP := *basePoint
		sumDP.Timestamp = ts
		sumDP.Dimensions = attrDims
		sum := histDP.GetSum()
		sumDP.Value = datapoint.NewFloatValue(sum)

		out = append(out, &countDP, &sumDP)

		bounds := histDP.GetExplicitBounds()
		counts := histDP.GetBucketCounts()

		// Spec says counts is optional but if present it must have one more
		// element than the bounds array.
		if len(counts) > 0 && len(counts) != len(bounds)+1 {
			continue
		}

		for j, c := range counts {
			bound := infinityBoundSFxDimValue
			if j < len(bounds) {
				bound = float64ToDimValue(bounds[j])
			}

			dp := *basePoint
			dp.Metric = basePoint.Metric + "_bucket"
			dp.Timestamp = ts
			dp.Dimensions = mergeStringMaps(attrDims, map[string]string{
				upperBoundDimensionKey: bound,
			})
			dp.Value = datapoint.NewIntValue(int64(c))

			out = append(out, &dp)
		}
	}

	return out
}

func convertSummaryDataPoints(
	in []*metricsv1.SummaryDataPoint,
	name string,
) []*datapoint.Datapoint {
	out := make([]*datapoint.Datapoint, 0, len(in))

	for _, inDp := range in {
		dims := attributesToDimensions(inDp.GetAttributes())
		ts := time.Unix(0, int64(inDp.GetTimeUnixNano()))

		countPt := datapoint.Datapoint{
			Metric:     name + "_count",
			Timestamp:  ts,
			Dimensions: dims,
			MetricType: datapoint.Counter,
		}
		c := int64(inDp.GetCount())
		countPt.Value = datapoint.NewIntValue(c)
		out = append(out, &countPt)

		sumPt := datapoint.Datapoint{
			Metric:     name,
			Timestamp:  ts,
			Dimensions: dims,
			MetricType: datapoint.Counter,
		}
		sum := inDp.GetSum()
		sumPt.Value = datapoint.NewFloatValue(sum)
		out = append(out, &sumPt)

		qvs := inDp.GetQuantileValues()
		for _, qv := range qvs {
			qPt := datapoint.Datapoint{
				Metric:    name + "_quantile",
				Timestamp: ts,
				Dimensions: mergeStringMaps(dims, map[string]string{
					"quantile": strconv.FormatFloat(qv.GetQuantile(), 'f', -1, 64),
				}),
				MetricType: datapoint.Gauge,
			}
			qPt.Value = datapoint.NewFloatValue(qv.GetValue())
			out = append(out, &qPt)
		}
	}
	return out
}

func attributesToDimensions(attributes []*commonv1.KeyValue) map[string]string {
	dimensions := make(map[string]string, len(attributes))
	if len(attributes) == 0 {
		return dimensions
	}
	for _, kv := range attributes {
		v := stringifyAnyValue(kv.GetValue())
		if v == "" {
			// Don't bother setting things that serialize to nothing
			continue
		}

		dimensions[kv.Key] = v
	}
	return dimensions
}

func stringifyAnyValue(a *commonv1.AnyValue) string {
	var v string
	if a == nil {
		return ""
	}
	switch a.GetValue().(type) {
	case *commonv1.AnyValue_StringValue:
		v = a.GetStringValue()

	case *commonv1.AnyValue_BoolValue:
		v = strconv.FormatBool(a.GetBoolValue())

	case *commonv1.AnyValue_DoubleValue:
		v = float64ToDimValue(a.GetDoubleValue())

	case *commonv1.AnyValue_IntValue:
		v = strconv.FormatInt(a.GetIntValue(), 10)

	case *commonv1.AnyValue_KvlistValue, *commonv1.AnyValue_ArrayValue:
		jsonStr, _ := json.Marshal(anyValueToRaw(a))
		v = string(jsonStr)
	}

	return v
}

func anyValueToRaw(a *commonv1.AnyValue) interface{} {
	var v interface{}
	if a == nil {
		return nil
	}
	switch a.GetValue().(type) {
	case *commonv1.AnyValue_StringValue:
		v = a.GetStringValue()

	case *commonv1.AnyValue_BoolValue:
		v = a.GetBoolValue()

	case *commonv1.AnyValue_DoubleValue:
		v = a.GetDoubleValue()

	case *commonv1.AnyValue_IntValue:
		v = a.GetIntValue()

	case *commonv1.AnyValue_KvlistValue:
		kvl := a.GetKvlistValue()
		tv := make(map[string]interface{}, len(kvl.Values))
		for _, kv := range kvl.Values {
			tv[kv.Key] = anyValueToRaw(kv.Value)
		}
		v = tv

	case *commonv1.AnyValue_ArrayValue:
		av := a.GetArrayValue()
		tv := make([]interface{}, len(av.Values))
		for i := range av.Values {
			tv[i] = anyValueToRaw(av.Values[i])
		}
		v = tv
	}
	return v
}

// Is equivalent to strconv.FormatFloat(f, 'g', -1, 64), but hardcodes a few common cases for increased efficiency.
func float64ToDimValue(f float64) string {
	// Parameters below are the same used by Prometheus
	// see https://github.com/prometheus/common/blob/b5fe7d854c42dc7842e48d1ca58f60feae09d77b/expfmt/text_create.go#L450
	// SignalFx agent uses a different pattern
	// https://github.com/signalfx/signalfx-agent/blob/5779a3de0c9861fa07316fd11b3c4ff38c0d78f0/internal/monitors/prometheusexporter/conversion.go#L77
	// The important issue here is consistency with the exporter, opting for the
	// more common one used by Prometheus.
	switch {
	case f == 0:
		return "0"
	case f == 1:
		return "1"
	case math.IsInf(f, +1):
		return "+Inf"
	default:
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
}

func numberToSignalFxValue(in *metricsv1.NumberDataPoint) datapoint.Value {
	v := in.GetValue()
	switch n := v.(type) {
	case *metricsv1.NumberDataPoint_AsDouble:
		return datapoint.NewFloatValue(n.AsDouble)
	case *metricsv1.NumberDataPoint_AsInt:
		return datapoint.NewIntValue(n.AsInt)
	}
	return nil
}

func mergeStringMaps(ms ...map[string]string) map[string]string {
	out := make(map[string]string)
	for _, m := range ms {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}
