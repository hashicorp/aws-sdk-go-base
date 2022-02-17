package test

import (
	"crypto/tls"
	"net/http"
	"testing"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
)

func HTTPClientConfigurationTest_basic(t *testing.T, transport *http.Transport) {
	if a, e := transport.MaxIdleConns, awshttp.DefaultHTTPTransportMaxIdleConns; a != e {
		t.Errorf("expected MaxIdleConns to be %d, got %d", e, a)
	}
	if a, e := transport.MaxIdleConnsPerHost, awshttp.DefaultHTTPTransportMaxIdleConnsPerHost; a != e {
		t.Errorf("expected MaxIdleConnsPerHost to be %d, got %d", e, a)
	}
	if a, e := transport.IdleConnTimeout, awshttp.DefaultHTTPTransportIdleConnTimeout; a != e {
		t.Errorf("expected IdleConnTimeout to be %s, got %s", e, a)
	}
	if a, e := transport.TLSHandshakeTimeout, awshttp.DefaultHTTPTransportTLSHandleshakeTimeout; a != e {
		t.Errorf("expected TLSHandshakeTimeout to be %s, got %s", e, a)
	}
	if a, e := transport.ExpectContinueTimeout, awshttp.DefaultHTTPTransportExpectContinueTimeout; a != e {
		t.Errorf("expected ExpectContinueTimeout to be %s, got %s", e, a)
	}
	if !transport.ForceAttemptHTTP2 {
		t.Error("expected ForceAttemptHTTP2 to be true, got false")
	}
	if transport.DisableKeepAlives {
		t.Error("expected DisableKeepAlives to be false, got true")
	}

	tlsConfig := transport.TLSClientConfig
	if a, e := int(tlsConfig.MinVersion), tls.VersionTLS12; a != e {
		t.Errorf("expected tlsConfig.MinVersion to be %d, got %d", e, a)
	}
	if tlsConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be false, got true")
	}
}

func HTTPClientConfigurationTest_insecureHTTPS(t *testing.T, transport *http.Transport) {
	tlsConfig := transport.TLSClientConfig
	if !tlsConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be true, got false")
	}
}
