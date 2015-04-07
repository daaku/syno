// Package syno provides a client to access the Synology NAS APIs.
//
// The APIs are documented in various PDFs here:
// https://global.download.synology.com/ftp/Document/DeveloperGuide/
package syno

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var (
	errURLMisconfigured = errors.New("syno: client URL misconfigured")
)

// Error is the integer error code returned by the Synology API.
type Error int

// Error returns a human readable error string for the error if one is known.
func (e Error) Error() string {
	if s, ok := errStrings[e]; ok {
		return fmt.Sprint("syno: ", s, " (", int(e), ")")
	}
	return fmt.Sprintf("syno: error code %d", int(e))
}

const (
	ErrorUnknown                          = Error(100)
	ErrorInvalidParameter                 = Error(101)
	ErrorInvalidAPI                       = Error(102)
	ErrorInvalidMethod                    = Error(103)
	ErrorUnsupportedVersion               = Error(104)
	ErrorPermissionDenied                 = Error(105)
	ErrorSessionTimeout                   = Error(106)
	ErrorSessionInterruptedDuplicateLogin = Error(107)
)

var errStrings = map[Error]string{
	ErrorUnknown:                          "unknown API error",
	ErrorInvalidParameter:                 "invalid parameter",
	ErrorInvalidAPI:                       "invalid API",
	ErrorInvalidMethod:                    "invalid method",
	ErrorUnsupportedVersion:               "unsupported version",
	ErrorPermissionDenied:                 "permission denined",
	ErrorSessionTimeout:                   "session timeout error",
	ErrorSessionInterruptedDuplicateLogin: "session interrupted with duplicated login",
}

func dropEmpty(p url.Values) url.Values {
	for k, v := range p {
		if len(v) == 1 && v[0] == "" {
			p.Del(k)
		}
	}
	return p
}

// Request represents an API request to Synology.
type Request struct {
	Path    string
	API     string
	Version string
	Method  string
	Params  url.Values
	SID     string
}

// MarshalRequest can be implemented by a type that can be serialized to a
// Request.
type MarshalRequest interface {
	MarshalRequest() (*Request, error)
}

// Client provides access to the Synology API.
type Client struct {
	url       *url.URL
	transport http.RoundTripper
	sid       string
}

// Call makes a request obtained from marshaling the given argument and calls
// Do with it.
func (c *Client) Call(r MarshalRequest, data interface{}) error {
	req, err := r.MarshalRequest()
	if err != nil {
		return err
	}
	return c.Do(req, data)
}

// Do performs an API request and unmarshals the "Data" into the passed in
// argument. If data is nil, it is ignored.
func (c *Client) Do(r *Request, data interface{}) error {
	v := make(url.Values)
	v.Add("api", r.API)
	v.Add("version", r.Version)
	v.Add("method", r.Method)

	if r.SID != "" {
		v.Add("_sid", r.SID)
	} else if c.sid != "" {
		v.Add("_sid", c.sid)
	}

	for k, l := range r.Params {
		for _, e := range l {
			v.Add(k, e)
		}
	}

	hreq := &http.Request{
		Method: "GET",
		URL: c.url.ResolveReference(&url.URL{
			Path:     r.Path,
			RawQuery: v.Encode(),
		}),
		Header: make(http.Header),
	}
	hres, err := c.transport.RoundTrip(hreq)
	if err != nil {
		return err
	}
	defer hres.Body.Close()

	var synologyResponse struct {
		Success bool
		Error   struct{ Code Error }
		Data    json.RawMessage
	}
	if err := json.NewDecoder(hres.Body).Decode(&synologyResponse); err != nil {
		return err
	}
	if !synologyResponse.Success {
		return synologyResponse.Error.Code
	}
	if data != nil {
		if err := json.Unmarshal(synologyResponse.Data, data); err != nil {
			return err
		}
	}
	return nil
}

// ClientOption allows configuring various aspects of the Client.
type ClientOption func(*Client) error

// ClientURL configures the base Client URL. All API requests are made relative
// to this URL.
func ClientURL(u *url.URL) ClientOption {
	return func(c *Client) error {
		c.url = u
		return nil
	}
}

// ClientRawURL configures the base Client URL. All API requests are made
// relative to this URL.
func ClientRawURL(u string) ClientOption {
	return func(c *Client) error {
		var err error
		c.url, err = url.Parse(u)
		return err
	}
}

// ClientTransport configures the Transport for the Client. If not specified
// http.DefaultTransport is used.
func ClientTransport(t http.RoundTripper) ClientOption {
	return func(c *Client) error {
		c.transport = t
		return nil
	}
}

// ClientSID configures a default "sid" to include for authenticating an
// account.
func ClientSID(sid string) ClientOption {
	return func(c *Client) error {
		c.sid = sid
		return nil
	}
}

// ClientLogin configures the Client with a "sid" from the given credentials.
// It does so when the client is being initialized, so the ordering of this
// option should typically be after all the other options have been specified.
func ClientLogin(l AuthLogin) ClientOption {
	return func(c *Client) error {
		var res AuthLoginResponse
		l.Format = "sid"
		if err := c.Call(l, &res); err != nil {
			return err
		}
		c.sid = res.SID
		return nil
	}
}

// NewClient creates a new client with the given options.
func NewClient(options ...ClientOption) (*Client, error) {
	c := Client{transport: http.DefaultTransport}
	for _, o := range options {
		if err := o(&c); err != nil {
			return nil, err
		}
	}
	if c.url == nil || !c.url.IsAbs() {
		return nil, errURLMisconfigured
	}
	return &c, nil
}

const (
	authLoginPath    = "/webapi/auth.cgi"
	authLoginAPI     = "SYNO.API.Auth"
	authLoginVersion = "3"
)

// AuthLogin logs in an account. The response is AuthLoginResponse.
type AuthLogin struct {
	Account  string
	Password string
	Session  string
	Format   string
	OTPCode  string
}

// MarshalRequest serializes the instance to a Request.
func (a AuthLogin) MarshalRequest() (*Request, error) {
	return &Request{
		Path:    authLoginPath,
		API:     authLoginAPI,
		Version: authLoginVersion,
		Method:  "login",
		Params: dropEmpty(url.Values{
			"account":  []string{a.Account},
			"passwd":   []string{a.Password},
			"session":  []string{a.Session},
			"format":   []string{a.Format},
			"otp_code": []string{a.OTPCode},
		}),
	}, nil
}

// AuthLoginResponse is the response from an AuthLogin request.
type AuthLoginResponse struct {
	SID    string
	Cookie string
}

const (
	downloadTaskPath    = "/webapi/DownloadStation/task.cgi"
	downloadTaskAPI     = "SYNO.DownloadStation.Task"
	downloadTaskVersion = "1"
)

// DownloadTaskList perfoms a list call for download tasks.
type DownloadTaskList struct {
	Offset     int
	Limit      int
	Additional []string
}

// MarshalRequest serializes the instance to a Request.
func (d DownloadTaskList) MarshalRequest() (*Request, error) {
	v := url.Values{}
	if d.Offset != 0 {
		v.Add("offset", strconv.Itoa(d.Offset))
	}
	if d.Limit != 0 {
		v.Add("limit", strconv.Itoa(d.Limit))
	}
	if len(d.Additional) > 0 {
		v.Add("additional", strings.Join(d.Additional, ","))
	}

	return &Request{
		Path:    downloadTaskPath,
		API:     downloadTaskAPI,
		Version: downloadTaskVersion,
		Method:  "list",
		Params:  v,
	}, nil
}

// DownloadTaskCreate creates a new download task. It does not have a response.
type DownloadTaskCreate struct {
	URI           string
	Username      string
	Password      string
	UnzipPassword string
	Destination   string
}

// MarshalRequest serializes the instance to a Request.
func (d DownloadTaskCreate) MarshalRequest() (*Request, error) {
	return &Request{
		Path:    downloadTaskPath,
		API:     downloadTaskAPI,
		Version: downloadTaskVersion,
		Method:  "create",
		Params: dropEmpty(url.Values{
			"uri":            []string{d.URI},
			"username":       []string{d.Username},
			"password":       []string{d.Password},
			"unzip_password": []string{d.UnzipPassword},
			"destination":    []string{d.Destination},
		}),
	}, nil
}
