package check

import (
	"crypto/dsa"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	log "github.com/Sirupsen/logrus"
	"github.com/racker/rackspace-monitoring-poller/metric"
	"github.com/racker/rackspace-monitoring-poller/utils"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"
)

var (
	MaxHttpResponseBodyLength = int64(512 * 1024)
)

type HTTPCheck struct {
	CheckBase
	Details struct {
		AuthPassword    string              `json:"auth_password"`
		AuthUser        string              `json:"auth_user"`
		Body            string              `json:"body"`
		BodyMatches     []map[string]string `json:"body_matches"`
		FollowRedirects bool                `json:"follow_redirects"`
		Headers         map[string]string   `json:"headers"`
		IncludeBody     bool                `json:"include_body"`
		Method          string              `json:"method"`
		Url             string              `json:"url"`
	}
}

func NewHTTPCheck(base *CheckBase) Check {
	check := &HTTPCheck{CheckBase: *base}
	err := json.Unmarshal(*base.Details, &check.Details)
	if err != nil {
		log.Error("Error unmarshalling checkbase")
		return nil
	}
	check.PrintDefaults()
	return check
}

func (ch *HTTPCheck) ParseTLS(cr *CheckResult, resp *http.Response) {
	cert := resp.TLS.PeerCertificates[0]
	cr.AddMetric(metric.NewMetric("cert_serial", "", metric.MetricNumber, cert.SerialNumber, ""))
	if len(cert.OCSPServer) > 0 {
		cr.AddMetric(metric.NewMetric("cert_ocsp", "", metric.MetricNumber, cert.OCSPServer[0], ""))
	}
	switch cert.PublicKeyAlgorithm {
	case x509.RSA:
		publicKey := cert.PublicKey.(*rsa.PublicKey)
		cr.AddMetric(metric.NewMetric("cert_bits", "", metric.MetricNumber, publicKey.N.BitLen(), ""))
		cr.AddMetric(metric.NewMetric("cert_type", "", metric.MetricNumber, "rsa", ""))
	case x509.DSA:
		publicKey := cert.PublicKey.(*dsa.PublicKey)
		cr.AddMetric(metric.NewMetric("cert_bits", "", metric.MetricNumber, publicKey.Q.BitLen(), ""))
		cr.AddMetric(metric.NewMetric("cert_type", "", metric.MetricNumber, "dsa", ""))
	case x509.ECDSA:
		publicKey := cert.PublicKey.(*ecdsa.PublicKey)
		cr.AddMetric(metric.NewMetric("cert_bits", "", metric.MetricNumber, publicKey.Params().BitSize, ""))
		cr.AddMetric(metric.NewMetric("cert_type", "", metric.MetricNumber, "ecdsa", ""))
	default:
		cr.AddMetric(metric.NewMetric("cert_bits", "", metric.MetricNumber, "0", ""))
		cr.AddMetric(metric.NewMetric("cert_type", "", metric.MetricNumber, "-", ""))
	}
	cr.AddMetric(metric.NewMetric("cert_sig_algo", "", metric.MetricNumber, strings.ToLower(cert.SignatureAlgorithm.String()), ""))
	var sslVersion string
	switch resp.TLS.Version {
	case tls.VersionSSL30:
		sslVersion = "ssl3"
	case tls.VersionTLS10:
		sslVersion = "tls1.0"
	case tls.VersionTLS11:
		sslVersion = "tls1.1"
	case tls.VersionTLS12:
		sslVersion = "tls1.2"
	}
	cr.AddMetric(metric.NewMetric("ssl_session_version", "", metric.MetricNumber, sslVersion, ""))
	var cipherSuite string
	switch resp.TLS.CipherSuite {
	case tls.TLS_RSA_WITH_RC4_128_SHA:
		cipherSuite = "TLS_RSA_WITH_RC4_128_SHA"
	case tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA:
		cipherSuite = "TLS_RSA_WITH_3DES_EDE_CBC_SHA"
	case tls.TLS_RSA_WITH_AES_128_CBC_SHA:
		cipherSuite = "TLS_RSA_WITH_AES_128_CBC_SHA"
	case tls.TLS_RSA_WITH_AES_256_CBC_SHA:
		cipherSuite = "TLS_RSA_WITH_AES_256_CBC_SHA"
	case tls.TLS_RSA_WITH_AES_128_GCM_SHA256:
		cipherSuite = "TLS_RSA_WITH_AES_128_GCM_SHA256"
	case tls.TLS_RSA_WITH_AES_256_GCM_SHA384:
		cipherSuite = "TLS_RSA_WITH_AES_256_GCM_SHA384"
	case tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA:
		cipherSuite = "TLS_ECDHE_ECDSA_WITH_RC4_128_SHA"
	case tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA:
		cipherSuite = "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA"
	case tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA:
		cipherSuite = "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA"
	case tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA:
		cipherSuite = "TLS_ECDHE_RSA_WITH_RC4_128_SHA"
	case tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA:
		cipherSuite = "TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA"
	case tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA:
		cipherSuite = "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA"
	case tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA:
		cipherSuite = "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA"
	case tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256:
		cipherSuite = "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"
	case tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256:

		cipherSuite = "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"
	case tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:
		cipherSuite = "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"
	case tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384:
		cipherSuite = "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384"
	default:
		cipherSuite = "-"
	}
	cr.AddMetric(metric.NewMetric("ssl_session_cipher", "", metric.MetricNumber, cipherSuite, ""))
	//TODO Fill in the rest of the SSL info
}

func DisableRedirects(req *http.Request, via []*http.Request) error {
	return http.ErrUseLastResponse
}

func (ch *HTTPCheck) Run() (*CheckResultSet, error) {
	log.WithFields(log.Fields{
		"type": ch.CheckType,
		"id":   ch.Id,
	}).Info("Running HTTP Check")
	starttime := utils.NowTimestampMillis()
	timeout := time.Duration(ch.Timeout) * time.Millisecond
	netTransport := &http.Transport{
		Dial:                (&net.Dialer{Timeout: timeout}).Dial,
		TLSHandshakeTimeout: timeout,
	}
	netClient := &http.Client{
		Timeout:   timeout,
		Transport: netTransport,
	}
	// Setup Redirects
	if !ch.Details.FollowRedirects {
		netClient.CheckRedirect = DisableRedirects
	}
	// Setup Method
	method := strings.ToUpper(ch.Details.Method)
	// Setup Request
	req, err := http.NewRequest(method, ch.Details.Url, nil)
	if err != nil {
		return nil, err
	}
	// Add Headers
	for key, value := range ch.Details.Headers {
		req.Header.Add(key, value)
	}
	// Perform Request
	resp, err := netClient.Do(req)
	if err != nil {
		log.Errorf("%s: HTTP: Got Error: %v", ch.GetId(), err)
		return nil, err
	}
	limitReader := io.LimitReader(resp.Body, MaxHttpResponseBodyLength)
	body, err := ioutil.ReadAll(limitReader)
	if err != nil {
		log.Errorf("%s: Received error in body read: %v", ch.GetId(), err)
		return nil, err
	}
	endtime := utils.NowTimestampMillis()
	resp.Body.Close()
	// METRICS
	cr := NewCheckResult(
		metric.NewMetric("code", "", metric.MetricNumber, resp.StatusCode, ""),
		metric.NewMetric("duration", "", metric.MetricNumber, endtime-starttime, "milliseconds"),
		metric.NewMetric("bytes", "", metric.MetricNumber, len(body), "bytes"),
	)
	// BODY MATCHES
	// BODY
	if ch.Details.IncludeBody {
		cr.AddMetric(metric.NewMetric("body", "", metric.MetricNumber, string(body), ""))
	}
	// TLS
	if resp.TLS != nil {
		ch.ParseTLS(cr, resp)
	}
	return NewCheckResultSet(ch, cr), nil
}
