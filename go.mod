module github.com/signalfx/ingest-protocols

go 1.13

replace git.apache.org/thrift.git => github.com/signalfx/thrift v0.0.0-20181211001559-3838fa316492

require (
	github.com/apache/thrift v0.0.0-20180411174621-858809fad01d
	github.com/bmizerany/perks v0.0.0-20141205001514-d9a9656a3a4b // indirect
	github.com/codahale/hdrhistogram v0.0.0-20161010025455-3a0bb77429bd // indirect
	github.com/coreos/etcd v3.3.18+incompatible
	github.com/gobwas/glob v0.2.3
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.3.2
	github.com/golang/snappy v0.0.1
	github.com/google/uuid v1.1.1 // indirect
	github.com/gorilla/mux v1.7.3
	github.com/jaegertracing/jaeger v1.16.0
	github.com/juju/loggo v0.0.0-20190526231331-6e530bcce5d8 // indirect
	github.com/juju/testing v0.0.0-20191001232224-ce9dec17d28b // indirect
	github.com/mailru/easyjson v0.7.0
	github.com/opentracing/opentracing-go v1.1.0
	github.com/prashantv/protectmem v0.0.0-20171002184600-e20412882b3a // indirect
	github.com/prometheus/common v0.9.1
	github.com/prometheus/prometheus v2.5.0+incompatible
	github.com/signalfx/com_signalfx_metrics_protobuf v0.0.0-20190530013331-054be550cb49
	github.com/signalfx/embetcd v0.0.9
	github.com/signalfx/gohelpers v0.0.0-20200319205814-a00d270ff0d6
	github.com/signalfx/golib/v3 v3.2.1
	github.com/signalfx/sapm-proto v0.4.0
	github.com/signalfx/xdgbasedir v0.0.0-20160106035722-cd6a71c07e4e
	github.com/smartystreets/assertions v1.0.1
	github.com/smartystreets/goconvey v1.6.4
	github.com/streadway/quantile v0.0.0-20150917103942-b0c588724d25 // indirect
	github.com/stretchr/testify v1.4.0
	github.com/uber/jaeger-client-go v2.22.1+incompatible // indirect
	github.com/uber/jaeger-lib v2.2.0+incompatible // indirect
	github.com/uber/tchannel-go v1.16.0 // indirect
	go.etcd.io/bbolt v1.3.3 // indirect
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22 // indirect
	sigs.k8s.io/yaml v1.1.0 // indirect
)
