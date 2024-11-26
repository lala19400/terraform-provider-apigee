package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/go-http-utils/headers"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

const (
	FormEncoded          = "application/x-www-form-urlencoded"
	ApplicationJson      = "application/json"
	ApplicationXml       = "application/xml"
	IdSeparator          = ":"
	Basic                = "Basic"
	Bearer               = "Bearer"
	SSOClientCredentials = "ZWRnZWNsaTplZGdlY2xpc2VjcmV0"
	PublicApigeeServer   = "api.enterprise.apigee.com"
	GoogleApigeeServer   = "apigee.googleapis.com"
	ServerPath           = "v1"
	HTTP                 = "http"
	HTTPS                = "https"
)

type Client struct {
	username        string
	password        string
	accessToken     string
	useSSL          bool
	server          string
	serverPath      string
	port            int
	oauthServer     string
	oauthServerPath string
	oauthPort       int
	Organization    string
	httpClient      *http.Client
}
type FormData struct {
	Filename string
	Text     string
}

func NewClient(username string, password string, accessToken string, useSSL bool, server string, serverPath string, port int, oauthServer string, oauthServerPath string, oauthPort int, organization string) (client *Client, err error) {
	c := &Client{
		username:        username,
		password:        password,
		accessToken:     accessToken,
		useSSL:          useSSL,
		server:          server,
		serverPath:      serverPath,
		port:            port,
		oauthServer:     oauthServer,
		oauthServerPath: oauthServerPath,
		oauthPort:       oauthPort,
		Organization:    organization,
		httpClient:      &http.Client{},
	}
	//Check for oauth authentication and try to get access token
	if c.oauthServer != "" {
		log.Print("Apigee Management API: Obtaining access token...")
		var requestURL string
		var scheme string
		if c.useSSL {
			scheme = HTTPS
		} else {
			scheme = HTTP
		}
		if c.oauthServerPath != "" {
			requestURL = fmt.Sprintf("%s://%s:%d/%s/%s", scheme, c.oauthServer, c.oauthPort, c.oauthServerPath, OauthTokenPath)
		} else {
			requestURL = fmt.Sprintf("%s://%s:%d/%s", scheme, c.oauthServer, c.oauthPort, OauthTokenPath)
		}
		requestForm := url.Values{
			"grant_type": []string{"password"},
			"username":   []string{c.username},
			"password":   []string{c.password},
		}
		req, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewBufferString(requestForm.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set(headers.Authorization, Basic+" "+SSOClientCredentials)
		req.Header.Set(headers.ContentType, FormEncoded)
		requestDump, err := httputil.DumpRequest(req, true)
		if err != nil {
			log.Print("Apigee Management API:")
			log.Print(err)
		} else {
			log.Print("Apigee Management API: " + string(requestDump))
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, &RequestError{StatusCode: http.StatusInternalServerError, Err: err}
		}
		if (resp.StatusCode < http.StatusOK) || (resp.StatusCode >= http.StatusMultipleChoices) {
			respBody := new(bytes.Buffer)
			_, err := respBody.ReadFrom(resp.Body)
			if err != nil {
				return nil, &RequestError{StatusCode: resp.StatusCode, Err: err}
			}
			return nil, &RequestError{StatusCode: resp.StatusCode, Err: fmt.Errorf("%s", respBody.String())}
		}
		//Parse body to extract access_token
		token := &OauthToken{}
		err = json.NewDecoder(resp.Body).Decode(token)
		if err != nil {
			return nil, err
		}
		log.Print("Apigee Management API: Received access token: " + token.AccessToken)
		//Inject token as access_token for client for all future calls
		c.accessToken = token.AccessToken
	}
	return c, nil
}

func (c *Client) IsPublic() bool {
	return (c.server == PublicApigeeServer) || strings.Contains(c.server, GoogleApigeeServer)
}

func (c *Client) IsGoogle() bool {
    return strings.Contains(c.server, GoogleApigeeServer)
}

func (c *Client) HttpRequest(method string, path string, query url.Values, headerMap http.Header, body *bytes.Buffer) (closer io.ReadCloser, err error) {
	req, err := http.NewRequest(method, c.requestPath(path), body)
	if err != nil {
		return nil, &RequestError{StatusCode: http.StatusInternalServerError, Err: err}
	}
	//Handle query values
	if query != nil {
		requestQuery := req.URL.Query()
		for key, values := range query {
			for _, value := range values {
				requestQuery.Add(key, value)
			}
		}
		req.URL.RawQuery = requestQuery.Encode()
	}
	//Handle header values
	if headerMap != nil {
		for key, values := range headerMap {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	}
	//Handle authentication
	if c.accessToken != "" {
		req.Header.Set(headers.Authorization, Bearer+" "+c.accessToken)
	} else {
		req.SetBasicAuth(c.username, c.password)
	}
	requestDump, err := httputil.DumpRequest(req, true)
	if err != nil {
		log.Print("Apigee Management API:")
		log.Print(err)
	} else {
		log.Print("Apigee Management API: " + string(requestDump))
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &RequestError{StatusCode: http.StatusInternalServerError, Err: err}
	}
	if (resp.StatusCode < http.StatusOK) || (resp.StatusCode >= http.StatusMultipleChoices) {
		respBody := new(bytes.Buffer)
		_, err := respBody.ReadFrom(resp.Body)
		if err != nil {
			return nil, &RequestError{StatusCode: resp.StatusCode, Err: err}
		}
		return nil, &RequestError{StatusCode: resp.StatusCode, Err: fmt.Errorf("%s", respBody.String())}
	}
	return resp.Body, nil
}

func (c *Client) requestPath(path string) string {
	var scheme string
	if c.useSSL {
		scheme = HTTPS
	} else {
		scheme = HTTP
	}
	return fmt.Sprintf("%s://%s:%d/%s/%s", scheme, c.server, c.port, c.serverPath, path)
}

func GetBuffer(filename string) (*bytes.Buffer, error) {
	buf := bytes.Buffer{}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	_, err = buf.ReadFrom(file)
	if err != nil {
		return nil, err
	}
	return &buf, nil
}

func GetMultiPartBuffer(formData map[string]FormData) (*multipart.Writer, *bytes.Buffer, error) {
	buf := bytes.Buffer{}
	mp := multipart.NewWriter(&buf)
	for key, fd := range formData {
		if fd.Filename != "" {
			//Handle files
			file, err := os.Open(fd.Filename)
			if err != nil {
				return nil, nil, err
			}
			defer file.Close()
			fw, err := mp.CreateFormFile(key, fd.Filename)
			if err != nil {
				return nil, nil, err
			}
			_, err = io.Copy(fw, file)
			if err != nil {
				return nil, nil, err
			}
		} else {
			//Handle strings
			fw, err := mp.CreateFormField(key)
			if err != nil {
				return nil, nil, err
			}
			_, err = io.Copy(fw, strings.NewReader(fd.Text))
			if err != nil {
				return nil, nil, err
			}
		}
	}
	mp.Close()
	return mp, &buf, nil
}
