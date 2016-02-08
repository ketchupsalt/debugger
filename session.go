package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type session struct {
	session string
	URL     string
}

// this is slurped out of our unit test framework and janky as hell
type response struct {
	path    string
	method  string
	code    int
	body    []byte
	redir   string
	err     error
	cookies map[string]string
}

func (self *response) HTTPOK(err error) bool {
	if err != nil {
		logError(self.path, err)
		return false
	}
	return self.code == 200
}

func (self *response) OK(err error) bool {
	type Ok struct {
		Ok    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err != nil {
		logError(self.path, err)
		return false
	}

	if self.code == 200 {
		ok := &Ok{}
		if err := json.Unmarshal(self.body, ok); err == nil {
			if ok.Ok {
				return true
			} else {
				logf("%s error: %s", self.path, ok.Error)
			}
		} else {
			logf("%s bad json: %s (%s)", self.path, err, self.body)
		}
	} else {
		logf("%s got HTTP %d", self.path, self.code)
	}
	return false
}

func (self *session) stamp(req *http.Request) {
	if self.session != "" {
		req.AddCookie(&http.Cookie{
			Name:  "api_key",
			Value: self.session,
		})
	}
}

func (self *session) reset() {
	self.session = ""
}

func (self *session) login(user, pass string) error {
	res, err := http.PostForm(strings.Replace(self.URL, "/trainer", "/ui/login", 1),
		url.Values{"session[username]": {user}, "session[password]": {pass}})
	if err != nil {
		return err
	}

	type LoginMsg struct {
		Status []string `json:"status"`
		Token  string   `json:"token"`
	}
	lm := &LoginMsg{}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", string(body))

	if err = json.Unmarshal(body, lm); err != nil {
		return err
	}

	if lm.Token == "" {
		return fmt.Errorf("login failed")
	}

	self.session = lm.Token
	return nil
}

func (self *session) post(path, data string) (*response, error) {
	req, err := http.NewRequest("POST", self.URL+path,
		strings.NewReader(data))
	if err != nil {
		return &response{path: path, method: "POST", err: err}, err
	}

	self.stamp(req)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return &response{path: path, method: "POST", err: err}, err
	}

	r, err := self.processResponse(res)
	r.path = path
	r.method = "POST"
	r.err = err
	if err != nil {
		panic(err.Error())
	}
	return r, err
}

func (self *session) put(path, data string) (*response, error) {
	req, err := http.NewRequest("PUT", self.URL+path,
		strings.NewReader(data))
	if err != nil {
		return &response{path: path, method: "PUT", err: err}, err
	}

	self.stamp(req)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return &response{path: path, method: "PUT", err: err}, err
	}

	r, err := self.processResponse(res)
	r.path = path
	r.method = "PUT"
	r.err = err
	if err != nil {
		panic(err.Error())
	}
	return r, err
}

func (self *session) get(path string) (*response, error) {
	req, err := http.NewRequest("GET", self.URL+path, nil)
	if err != nil {
		return &response{path: path, method: "GET", err: err}, err
	}

	self.stamp(req)

	client := http.Client{Timeout: (10000 * time.Millisecond)}

	res, err := client.Do(req)
	if err != nil {
		return &response{path: path, method: "GET", err: err}, err
	}

	r, err := self.processResponse(res)
	r.path = path
	r.method = "GET"
	r.err = err
	return r, err
}

func (self *session) del(path string) (*response, error) {
	req, err := http.NewRequest("DELETE", self.URL+path, nil)
	if err != nil {
		return &response{path: path, method: "DELETE", err: err}, err
	}

	self.stamp(req)

	client := http.Client{Timeout: (10000 * time.Millisecond)}

	res, err := client.Do(req)
	if err != nil {
		return &response{path: path, method: "DELETE", err: err}, err
	}

	r, err := self.processResponse(res)
	r.path = path
	r.method = "DELETE"
	r.err = err
	return r, err
}

func (self *session) processResponse(res *http.Response) (r *response, err error) {
	ret := &response{
		code: res.StatusCode,
	}

	if h, ok := res.Header["Location"]; ok {
		ret.redir = h[0]
	}

	ret.body, err = ioutil.ReadAll(res.Body)
	if err != nil {
		return ret, err
	}

	ret.cookies = map[string]string{}

	for _, cookie := range res.Cookies() {
		ret.cookies[cookie.Name] = cookie.Value
		if cookie.Name == "SESSION" {
			if self.session == "" {
				self.session = cookie.Value
			}
		}
	}

	return ret, nil
}
