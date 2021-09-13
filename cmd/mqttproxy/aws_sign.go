package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iot"
)

const (
	port                  = 443
	protocol              = "wss"
	path                  = "/mqtt"
	endpointType          = "iot:Data-ATS"
	subTimeout            = 2000
	pubTimeout            = 2000
	qos1                  = 1
	defaultMQTTBufferSize = 1024
)

var (
	DefaultLogger   Logger = &GoLogger{}
	DefaultLogLevel        = "info"
)

type Ctx struct {
	context.Context
	Logger
	LogLevel string
	sess     *session.Session
}

func NewCtx(ctx context.Context, region string) (*Ctx, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	cfg := aws.Config{
		Region: aws.String(region),
	}

	sess, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            cfg,
	})
	if err != nil {
		return nil, err
	}

	return &Ctx{
		Context:  ctx,
		Logger:   DefaultLogger,
		LogLevel: DefaultLogLevel,
		sess:     sess,
	}, nil
}

func (c *Ctx) WithCancel() (*Ctx, func()) {
	ctx, cancel := context.WithCancel(c.Context)
	return &Ctx{
		Context:  ctx,
		Logger:   DefaultLogger,
		LogLevel: c.LogLevel,
	}, cancel
}

func (c *Ctx) WithTimeout(d time.Duration) (*Ctx, func()) {
	ctx, cancel := context.WithTimeout(c.Context, d)
	return &Ctx{
		Context:  ctx,
		Logger:   DefaultLogger,
		LogLevel: c.LogLevel,
	}, cancel
}

func (c *Ctx) SetLogLevel(level string) error {
	canonical := strings.ToLower(level)
	// No strings.TrimSpace.

	switch canonical {
	case "info", "debug", "none":
	default:
		return fmt.Errorf("Ctx.LogLevel '%s' isn't 'info', 'debug', or 'none'", canonical)
	}
	c.LogLevel = canonical
	return nil
}

// Indf emits a log line starting with a '|' when ctx.LogLevel isn't 'none'.
func (c *Ctx) Indf(format string, args ...interface{}) {
	switch c.LogLevel {
	case "none", "NONE":
	default:
		c.Printf("| "+format, args...)
	}
}

// Inddf emits a log line starting with a '|' when ctx.LogLevel is 'debug';
//
// The second 'd' is for "debug".
func (c *Ctx) Inddf(format string, args ...interface{}) {
	switch c.LogLevel {
	case "debug", "DEBUG":
		c.Printf("| "+format, args...)
	}
}

// Warnf emits a log  with a '!' prefix.
func (c *Ctx) Warnf(format string, args ...interface{}) {
	c.Printf("! "+format, args...)
}

// Logf emits a log line starting with a '>' when ctx.LogLevel isn't 'none'.
func (c *Ctx) Logf(format string, args ...interface{}) {
	switch c.LogLevel {
	case "none", "NONE":
	default:
		c.Printf("> "+format, args...)
	}
}

// Logdf emits a log line starting with a '>' when ctx.LogLevel is 'debug';
//
// The second 'd' is for "debug".
func (c *Ctx) Logdf(format string, args ...interface{}) {
	switch c.LogLevel {
	case "debug", "DEBUG":
		c.Printf("> "+format, args...)
	}
}

// Logger is an interface that allows for pluggabe loggers.
//
// Used in the Plax Lambda.
type Logger interface {
	Printf(format string, args ...interface{})
}

// GoLogger is just basic Go logging.
type GoLogger struct {
}

func (l *GoLogger) Printf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

// GetEndpoint retries the IoT endpoint
func getEndpoint(ctx *Ctx) (string, error) {
	svc := iot.New(ctx.sess)

	input := iot.DescribeEndpointInput{
		EndpointType: aws.String(endpointType),
	}

	output, err := svc.DescribeEndpointWithContext(ctx, &input)
	if err != nil {
		fmt.Printf("Could not get iotdata endpoint: %v", err)
		return "", err
	}

	return *output.EndpointAddress, nil
}

func GetWebsocketUrl() (*url.URL, error) {
	ctx, err := NewCtx(context.Background(), "us-west-2")
	if err != nil {
		panic(err)
	}

	endpoint, err := getEndpoint(ctx)
	if err != nil {
		return nil, err
	}

	creds, err := ctx.sess.Config.Credentials.Get()
	if err != nil {
		return nil, err
	}

	// according to docs, time must be within 5min of actual time (or at least according to AWS servers)
	now := time.Now().UTC()
	dateLong := now.Format("20060102T150405Z")
	dateShort := dateLong[:8]
	serviceName := "iotdevicegateway"
	scope := fmt.Sprintf("%s/%s/%s/aws4_request", dateShort, *ctx.sess.Config.Region, serviceName)
	alg := "AWS4-HMAC-SHA256"
	q := [][2]string{
		{"X-Amz-Algorithm", alg},
		{"X-Amz-Credential", creds.AccessKeyID + "/" + scope},
		{"X-Amz-Date", dateLong},
		{"X-Amz-SignedHeaders", "host"},
	}

	query := awsQueryParams(q)
	signKey := awsSignKey(creds.SecretAccessKey, dateShort, *ctx.sess.Config.Region, serviceName)
	stringToSign := awsSignString(creds.AccessKeyID, creds.SecretAccessKey, query, endpoint, dateLong, alg, scope)
	signature := fmt.Sprintf("%x", awsHmac(signKey, []byte(stringToSign)))
	wsURLStr := fmt.Sprintf("%s://%s%s?%s&X-Amz-Signature=%s", protocol, endpoint, path, query, signature)

	if creds.SessionToken != "" {
		wsURLStr = fmt.Sprintf("%s&X-Amz-Security-Token=%s", wsURLStr, url.QueryEscape(creds.SessionToken))
	}

	wsURL, err := url.Parse(wsURLStr)
	if err != nil {
		return nil, err
	}

	return wsURL, nil
}

func awsQueryParams(q [][2]string) string {
	var buff bytes.Buffer
	var i int
	for _, param := range q {
		if i != 0 {
			buff.WriteRune('&')
		}
		i++
		buff.WriteString(param[0])
		buff.WriteRune('=')
		buff.WriteString(url.QueryEscape(param[1]))
	}
	return buff.String()
}

func awsSignString(accessKey string, secretKey string, query string, host string, dateLongStr string, alg string, scopeStr string) string {
	emptyStringHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	req := strings.Join([]string{
		"GET",
		"/mqtt",
		query,
		"host:" + host,
		"", // separator
		"host",
		emptyStringHash,
	}, "\n")
	return strings.Join([]string{
		alg,
		dateLongStr,
		scopeStr,
		awsSha(req),
	}, "\n")
}

func awsHmac(key []byte, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func awsSignKey(secretKey string, dateShort string, region string, serviceName string) []byte {
	h := awsHmac([]byte("AWS4"+secretKey), []byte(dateShort))
	h = awsHmac(h, []byte(region))
	h = awsHmac(h, []byte(serviceName))
	h = awsHmac(h, []byte("aws4_request"))
	return h
}

func awsSha(in string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s", in)
	return fmt.Sprintf("%x", h.Sum(nil))
}
