package syno

import (
	"context"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/facebookgo/ensure"
	"github.com/facebookgo/jsonpipe"
)

func TestCustomErrorString(t *testing.T) {
	ensure.DeepEqual(t, ErrorUnknown.Error(), "syno: unknown API error (100)")
}

func TestGenericErrorString(t *testing.T) {
	ensure.DeepEqual(t, Error(42).Error(), "syno: error code 42")
}

func TestDropEmpty(t *testing.T) {
	ensure.DeepEqual(
		t,
		dropEmpty(url.Values{
			"keep": []string{"foo"},
			"drop": []string{""},
		}),
		url.Values{
			"keep": []string{"foo"},
		},
	)
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestClientDoSuccess(t *testing.T) {
	const data = "data"
	c, err := NewClient(
		ClientRawURL("http://foo.com/"),
		ClientTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
			ensure.DeepEqual(t, r.URL.Host, "foo.com")
			ensure.DeepEqual(t, r.URL.Scheme, "http")
			v, err := url.ParseQuery(r.URL.RawQuery)
			ensure.Nil(t, err)
			ensure.DeepEqual(t, v, url.Values{
				"api":     []string{"api"},
				"method":  []string{"method"},
				"version": []string{"version"},
				"foo":     []string{"foo"},
			})
			return &http.Response{
				Body: ioutil.NopCloser(jsonpipe.Encode(map[string]interface{}{
					"success": true,
					"data":    data,
				})),
			}, nil
		})),
	)
	ensure.Nil(t, err)
	var res string
	err = c.Do(context.Background(), &Request{
		API:     "api",
		Method:  "method",
		Version: "version",
		Params: url.Values{
			"foo": []string{"foo"},
		},
	}, &res)
	ensure.Nil(t, err)
	ensure.DeepEqual(t, res, data)
}

func TestClientDoRequestSID(t *testing.T) {
	const reqSID = "reqSID"
	c, err := NewClient(
		ClientRawURL("http://foo.com/"),
		ClientSID("clientSID"),
		ClientTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
			v, err := url.ParseQuery(r.URL.RawQuery)
			ensure.Nil(t, err)
			ensure.DeepEqual(t, v["_sid"], []string{reqSID})
			return &http.Response{
				Body: ioutil.NopCloser(jsonpipe.Encode(map[string]interface{}{
					"success": true,
				})),
			}, nil
		})),
	)
	ensure.Nil(t, err)
	err = c.Do(context.Background(), &Request{SID: reqSID}, nil)
	ensure.Nil(t, err)
}

func TestClientDoClientSID(t *testing.T) {
	const clientSID = "reqSID"
	c, err := NewClient(
		ClientRawURL("http://foo.com/"),
		ClientSID(clientSID),
		ClientTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
			v, err := url.ParseQuery(r.URL.RawQuery)
			ensure.Nil(t, err)
			ensure.DeepEqual(t, v["_sid"], []string{clientSID})
			return &http.Response{
				Body: ioutil.NopCloser(jsonpipe.Encode(map[string]interface{}{
					"success": true,
				})),
			}, nil
		})),
	)
	ensure.Nil(t, err)
	err = c.Do(context.Background(), &Request{}, nil)
	ensure.Nil(t, err)
}

func TestClientDoTransportError(t *testing.T) {
	givenErr := errors.New("")
	c, err := NewClient(
		ClientRawURL("http://foo.com/"),
		ClientTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
			return nil, givenErr
		})),
	)
	ensure.Nil(t, err)
	err = c.Do(context.Background(), &Request{}, nil)
	ensure.DeepEqual(t, err, givenErr)
}

func TestClientDoNonJSON(t *testing.T) {
	c, err := NewClient(
		ClientRawURL("http://foo.com/"),
		ClientTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				Body: ioutil.NopCloser(strings.NewReader(".")),
			}, nil
		})),
	)
	ensure.Nil(t, err)
	err = c.Do(context.Background(), &Request{}, nil)
	ensure.Err(t, err, regexp.MustCompile("invalid character"))
}

func TestClientDoDataJSONError(t *testing.T) {
	c, err := NewClient(
		ClientRawURL("http://foo.com/"),
		ClientTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				Body: ioutil.NopCloser(jsonpipe.Encode(map[string]interface{}{
					"success": true,
					"data":    true,
				})),
			}, nil
		})),
	)
	ensure.Nil(t, err)
	var res string
	err = c.Do(context.Background(), &Request{}, &res)
	ensure.Err(t, err, regexp.MustCompile("cannot unmarshal bool into Go value of type string"))
}

func TestClientAPIError(t *testing.T) {
	c, err := NewClient(
		ClientRawURL("http://foo.com/"),
		ClientTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				Body: ioutil.NopCloser(jsonpipe.Encode(map[string]interface{}{
					"error": map[string]interface{}{
						"code": ErrorUnknown,
					},
				})),
			}, nil
		})),
	)
	ensure.Nil(t, err)
	err = c.Do(context.Background(), &Request{}, nil)
	ensure.DeepEqual(t, err, ErrorUnknown)
}

func TestClientLogin(t *testing.T) {
	c, err := NewClient(
		ClientRawURL("http://foo.com/"),
		ClientTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
			v, err := url.ParseQuery(r.URL.RawQuery)
			ensure.Nil(t, err)
			ensure.Subset(t, v, url.Values{
				"account": []string{"account"},
				"passwd":  []string{"password"},
			})
			return &http.Response{
				Body: ioutil.NopCloser(jsonpipe.Encode(map[string]interface{}{
					"success": true,
					"data": map[string]string{
						"sid": "sid",
					},
				})),
			}, nil
		})),
		ClientLogin(AuthLogin{
			Account:  "account",
			Password: "password",
		}),
	)
	ensure.Nil(t, err)
	ensure.DeepEqual(t, c.sid, "sid")
}

func TestClientLoginError(t *testing.T) {
	givenErr := errors.New("")
	c, err := NewClient(
		ClientRawURL("http://foo.com/"),
		ClientTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
			return nil, givenErr
		})),
		ClientLogin(AuthLogin{
			Account:  "account",
			Password: "password",
		}),
	)
	ensure.True(t, c == nil)
	ensure.DeepEqual(t, err, givenErr)
}

type funcMarshalRequest func() (*Request, error)

func (f funcMarshalRequest) MarshalRequest() (*Request, error) { return f() }

func TestCallMarshalError(t *testing.T) {
	c, err := NewClient(ClientRawURL("http://foo.com/"))
	ensure.Nil(t, err)
	givenErr := errors.New("")
	err = c.Call(
		context.Background(),
		funcMarshalRequest(func() (*Request, error) { return nil, givenErr }),
		nil,
	)
	ensure.DeepEqual(t, err, givenErr)
}

func TestCallTransportError(t *testing.T) {
	givenErr := errors.New("")
	c, err := NewClient(
		ClientRawURL("http://foo.com/"),
		ClientTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
			return nil, givenErr
		})),
	)
	ensure.Nil(t, err)
	err = c.Call(
		context.Background(),
		funcMarshalRequest(func() (*Request, error) { return &Request{}, nil }),
		nil,
	)
	ensure.DeepEqual(t, err, givenErr)
}

func TestNewClientURLNil(t *testing.T) {
	c, err := NewClient()
	ensure.True(t, c == nil)
	ensure.DeepEqual(t, err, errURLMisconfigured)
}

func TestNewClientURLNotAbsolute(t *testing.T) {
	c, err := NewClient(ClientURL(&url.URL{}))
	ensure.True(t, c == nil)
	ensure.DeepEqual(t, err, errURLMisconfigured)
}

func TestNewClientDefaultTransport(t *testing.T) {
	c, err := NewClient(
		ClientURL(&url.URL{
			Scheme: "http",
			Host:   "foo.com",
		}),
	)
	ensure.Nil(t, err)
	ensure.DeepEqual(t, c.transport, http.DefaultTransport)
}

func TestNewClientOptionError(t *testing.T) {
	givenErr := errors.New("")
	c, err := NewClient(func(*Client) error { return givenErr })
	ensure.True(t, c == nil)
	ensure.DeepEqual(t, err, givenErr)
}

func TestAuthLoginMarshal(t *testing.T) {
	cases := []struct {
		AuthLogin AuthLogin
		Request   *Request
	}{
		{
			AuthLogin: AuthLogin{
				Account:  "a",
				Password: "b",
			},
			Request: &Request{
				Path:    authLoginPath,
				API:     authLoginAPI,
				Version: authLoginVersion,
				Method:  "login",
				Params: url.Values{
					"account": []string{"a"},
					"passwd":  []string{"b"},
				},
			},
		},
		{
			AuthLogin: AuthLogin{},
			Request: &Request{
				Path:    authLoginPath,
				API:     authLoginAPI,
				Version: authLoginVersion,
				Method:  "login",
				Params:  url.Values{},
			},
		},
	}

	for _, c := range cases {
		r, err := c.AuthLogin.MarshalRequest()
		ensure.Nil(t, err)
		ensure.DeepEqual(t, r, c.Request)
	}
}

func TestDownloadTaskListMarshal(t *testing.T) {
	cases := []struct {
		DownloadTaskList DownloadTaskList
		Request          *Request
	}{
		{
			DownloadTaskList: DownloadTaskList{
				Offset:     1,
				Limit:      2,
				Additional: []string{"a", "b"},
			},
			Request: &Request{
				Path:    downloadTaskPath,
				API:     downloadTaskAPI,
				Version: downloadTaskVersion,
				Method:  "list",
				Params: url.Values{
					"offset":     []string{"1"},
					"limit":      []string{"2"},
					"additional": []string{"a,b"},
				},
			},
		},
		{
			DownloadTaskList: DownloadTaskList{},
			Request: &Request{
				Path:    downloadTaskPath,
				API:     downloadTaskAPI,
				Version: downloadTaskVersion,
				Method:  "list",
				Params:  url.Values{},
			},
		},
	}

	for _, c := range cases {
		r, err := c.DownloadTaskList.MarshalRequest()
		ensure.Nil(t, err)
		ensure.DeepEqual(t, r, c.Request)
	}
}

func TestDownloadTaskCreateMarshal(t *testing.T) {
	cases := []struct {
		DownloadTaskCreate DownloadTaskCreate
		Request            *Request
	}{
		{
			DownloadTaskCreate: DownloadTaskCreate{
				URI:           "a",
				Username:      "b",
				Password:      "c",
				UnzipPassword: "d",
				Destination:   "e",
			},
			Request: &Request{
				Path:    downloadTaskPath,
				API:     downloadTaskAPI,
				Version: downloadTaskVersion,
				Method:  "create",
				Params: url.Values{
					"uri":            []string{"a"},
					"username":       []string{"b"},
					"password":       []string{"c"},
					"unzip_password": []string{"d"},
					"destination":    []string{"e"},
				},
			},
		},
		{
			DownloadTaskCreate: DownloadTaskCreate{
				URI: "foo",
			},
			Request: &Request{
				Path:    downloadTaskPath,
				API:     downloadTaskAPI,
				Version: downloadTaskVersion,
				Method:  "create",
				Params:  url.Values{"uri": []string{"foo"}},
			},
		},
	}

	for _, c := range cases {
		r, err := c.DownloadTaskCreate.MarshalRequest()
		ensure.Nil(t, err)
		ensure.DeepEqual(t, r, c.Request)
	}
}
