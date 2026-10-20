package main
/*

 Krakend plugin to rewrite request body. Support endpoint parameter injection into the body

 Krakend configuration :
 {
   "endpoint": "/organizations/{org}",
   "backend": [{
	 "url_pattern": "/api/orgs",
	 "host": ["https://api.example.com"],
	 "extra_config": {
	   "plugin/req-resp-modifier": {
         "name": ["krakend-body-modifier-request"],
         "body_template": "{ \"schema\": \"PUBLIC\", \"query\": \"SELECT KEY,NAME FROM PUBLIC.ORGANIZATION WHERE NAME = '{{.Org}}'\"}"
       },
	 }
   }]
 }

 - body_template can use go templating to refer to endpoint args
 - generated body replace body provided by client

 Example : Request /organizations/Org%2011 -> Generate body : {"schema": "PUBLIC", "query": "SELECT KEY,NAME FROM PUBLIC.ORGANIZATION WHERE NAME = 'Org%2011'"}
 
 Compilation : docker run -it -v "$PWD:/app" -w /app krakend/builder:2.13.8-linux-generic go build -buildmode=plugin -buildvcs=false -o krakend-body-modifier-request.so
   - Replace 2.13.8 with krakend target version
   - Remove "-linux-generic" if building for docker (binary will be linked to musl)

*/

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"text/template"
)

func main() {}

const (
	gPluginName = "krakend-body-modifier-request"
)

var (
	ModifierRegisterer  = registerer(gPluginName)
	logger Logger       = nil
	ctx context.Context = context.Background()
)

type registerer string

func (r registerer) RegisterModifiers(f func(
	name string,
	factoryFunc func(map[string]interface{}) func(interface{}) (interface{}, error),
	appliesToRequest bool,
	appliesToResponse bool,
)) {
	f(string(r), r.requestModifierFactory, true, false)
}

func (registerer) RegisterLogger(in interface{}) {
	l, ok := in.(Logger)
	if !ok {
		return
	}
	logger = l
	logger.Debug(fmt.Sprintf("[PLUGIN: %s] Logger loaded", ModifierRegisterer))
}

func (registerer) RegisterContext(c context.Context) {
	ctx = c
	logger.Debug(fmt.Sprintf("[PLUGIN: %s] Context loaded", ModifierRegisterer))
}

func (r registerer) requestModifierFactory(cfg map[string]interface{}) func(interface{}) (interface{}, error) {
	bodyTemplateStr, _ := cfg["body_template"].(string)
	logger.Info(fmt.Sprintf("[PLUGIN: %s] body_template = %v", gPluginName, bodyTemplateStr))

	var tmpl *template.Template
	var err error
	if bodyTemplateStr != "" {
		tmpl, err = template.New("body").Parse(bodyTemplateStr)
		if err != nil {
			logger.Error(fmt.Sprintf("[PLUGIN: %s] Failed to parse body template: %v", ModifierRegisterer, err))
			return func(interface{}) (interface{}, error) {
				return nil, fmt.Errorf("failed to parse body template: %w", err)
			}
		}
	}

	logger.Debug(fmt.Sprintf("[PLUGIN: %s] Request modifier injected", ModifierRegisterer))
	return func(input interface{}) (interface{}, error) {
		req, ok := input.(RequestWrapper)
		if !ok {
			return nil, unkownTypeErr
		}

		params := req.Params()
		logger.Debug(fmt.Sprintf("[PLUGIN: %s] URL params: %v", ModifierRegisterer, params))

		wrapper := &requestWrapperWrapper{
			RequestWrapper: req,
		}

		if tmpl != nil {
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, params); err != nil {
				logger.Error(fmt.Sprintf("[PLUGIN: %s] Failed to execute body template: %v", ModifierRegisterer, err))
				return nil, fmt.Errorf("failed to execute body template: %w", err)
			}
			bodyContent := buf.String()
			logger.Debug(fmt.Sprintf("[PLUGIN: %s] Generated body: %s", ModifierRegisterer, bodyContent))
			wrapper.body = io.NopCloser(bytes.NewReader([]byte(bodyContent)))
		}

		return wrapper, nil
	}
}

var unkownTypeErr = errors.New("unknown request type")

type requestWrapperWrapper struct {
	RequestWrapper
	body io.ReadCloser
}

func (r *requestWrapperWrapper) Body() io.ReadCloser {
	if r.body != nil {
		return r.body
	}
	return r.RequestWrapper.Body()
}

// ResponseWrapper is an interface for passing proxy response between the krakend pipe
// and the loaded plugins
type ResponseWrapper interface {
    Data() map[string]interface{}
    Io() io.Reader
    IsComplete() bool
    StatusCode() int
    Headers() map[string][]string
}

// RequestWrapper is an interface for passing proxy request between the krakend pipe
// and the loaded plugins
type RequestWrapper interface {
    Params() map[string]string
    Headers() map[string][]string
    Body() io.ReadCloser
    Method() string
    URL() *url.URL
    Query() url.Values
    Path() string
}

type Logger interface {
	Debug(v ...interface{})
	Info(v ...interface{})
	Warning(v ...interface{})
	Error(v ...interface{})
	Critical(v ...interface{})
	Fatal(v ...interface{})
}