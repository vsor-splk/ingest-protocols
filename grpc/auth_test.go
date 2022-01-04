package grpc

import (
	"context"
	"testing"

	"github.com/signalfx/golib/v3/sfxclient"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGRPCAuth(t *testing.T) {
	Convey("grpc auth", t, func() {
		a := &SignalFxTokenAuth{
			Token: "test",
			DisableTransportSecurity: false,
		}

		So(a.RequireTransportSecurity(), ShouldBeTrue)

		md, err := a.GetRequestMetadata(context.Background(), "")
		So(err, ShouldBeNil)
		So(md[sfxclient.TokenHeaderName], ShouldEqual, "test")

		a.DisableTransportSecurity = true
		So(a.RequireTransportSecurity(), ShouldBeFalse)
	})
}
