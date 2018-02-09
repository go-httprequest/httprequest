// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package httprequest_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"gopkg.in/httprequest.v1"
)

type marshalSuite struct{}

var _ = gc.Suite(&marshalSuite{})

type embedded struct {
	F1 string  `json:"name"`
	F2 int     `json:"age"`
	F3 *string `json:"address"`
}

var marshalTests = []struct {
	about           string
	urlString       string
	method          string
	val             interface{}
	expectURLString string
	expectBody      *string
	expectHeader    http.Header
	expectError     string
}{{
	about:     "struct with simple fields",
	urlString: "http://localhost:8081/:F01",
	val: &struct {
		F01 int        `httprequest:",path"`
		F02 string     `httprequest:",form"`
		F03 string     `httprequest:",form,omitempty"`
		F04 string     `httprequest:",form,omitempty"`
		F06 time.Time  `httprequest:",form,omitempty"`
		F07 time.Time  `httprequest:",form,omitempty"`
		F08 *time.Time `httprequest:",form,omitempty"`
		F09 *time.Time `httprequest:",form,omitempty"`
		F10 stringer   `httprequest:",form,omitempty"`
		F11 stringer   `httprequest:",form,omitempty"`
		F12 *stringer  `httprequest:",form,omitempty"`
		F13 *stringer  `httprequest:",form,omitempty"`
		F14 *stringer  `httprequest:",form,omitempty"`
		F15 time.Time  `httprequest:",form"`
		// Note that this gets omitted anyway because it's nil.
		F16 *time.Time `httprequest:",form"`
	}{
		F01: 99,
		F02: "some text",
		F03: "",
		F04: "something",
		F07: time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC),
		F09: func() *time.Time {
			t := time.Date(2011, 2, 3, 4, 5, 6, 0, time.UTC)
			return &t
		}(),
		F11: stringer(99),
		F13: func() *stringer {
			s := stringer(99)
			return &s
		}(),
		F14: new(stringer),
	},
	expectURLString: "http://localhost:8081/99" +
		"?F02=some+text" +
		"&F04=something" +
		"&F07=2001-02-03T04%3A05%3A06Z" +
		"&F09=2011-02-03T04%3A05%3A06Z" +
		"&F11=str99" +
		"&F13=str99" +
		"&F15=0001-01-01T00%3A00%3A00Z",
}, {
	about:     "struct with renamed fields",
	urlString: "http://localhost:8081/:name",
	val: &struct {
		F1 string `httprequest:"name,path"`
		F2 int    `httprequest:"age,form"`
	}{
		F1: "some random user",
		F2: 42,
	},
	expectURLString: "http://localhost:8081/some%20random%20user?age=42",
}, {
	about:     "fields without httprequest tags are ignored",
	urlString: "http://localhost:8081/:name",
	val: &struct {
		F1 string `httprequest:"name,path"`
		F2 int    `httprequest:"age,form"`
		F3 string
	}{
		F1: "some random user",
		F2: 42,
		F3: "some more random text",
	},
	expectURLString: "http://localhost:8081/some%20random%20user?age=42",
}, {
	about:     "pointer fields are correctly handled",
	urlString: "http://localhost:8081/:name",
	val: &struct {
		F1 *string `httprequest:"name,path"`
		F2 *string `httprequest:"age,form"`
		F3 *string `httprequest:"address,form"`
	}{
		F1: newString("some random user"),
		F2: newString("42"),
	},
	expectURLString: "http://localhost:8081/some%20random%20user?age=42",
}, {
	about:     "MarshalText called on TextMarshalers",
	urlString: "http://localhost:8081/:param1/:param2",
	val: &struct {
		F1 testMarshaler  `httprequest:"param1,path"`
		F2 *testMarshaler `httprequest:"param2,path"`
		F3 testMarshaler  `httprequest:"param3,form"`
		F4 *testMarshaler `httprequest:"param4,form"`
	}{
		F1: "test1",
		F2: (*testMarshaler)(newString("test2")),
		F3: "test3",
		F4: (*testMarshaler)(newString("test4")),
	},
	expectURLString: "http://localhost:8081/test_test1/test_test2?param3=test_test3&param4=test_test4",
}, {
	about:     "MarshalText not called on values that do not implement TextMarshaler",
	urlString: "http://localhost:8081/user/:name/:surname",
	val: &struct {
		F1 notTextMarshaler  `httprequest:"name,path"`
		F2 *notTextMarshaler `httprequest:"surname,path"`
	}{
		F1: "name",
		F2: (*notTextMarshaler)(newString("surname")),
	},
	expectURLString: "http://localhost:8081/user/name/surname",
}, {
	about:     "MarshalText returns an error",
	urlString: "http://localhost:8081/user/:name/:surname",
	val: &struct {
		F1 testMarshaler  `httprequest:"name,path"`
		F2 *testMarshaler `httprequest:"surname,path"`
	}{
		F1: "",
		F2: (*testMarshaler)(newString("surname")),
	},
	expectError: "cannot marshal field: empty string",
}, {
	about:     "[]string field form value",
	urlString: "http://localhost:8081/user",
	val: &struct {
		F1 []string `httprequest:"users,form"`
	}{
		F1: []string{"user1", "user2", "user3"},
	},
	expectURLString: "http://localhost:8081/user?users=user1&users=user2&users=user3",
}, {
	about:     "nil []string field form value",
	urlString: "http://localhost:8081/user",
	val: &struct {
		F1 *[]string `httprequest:"users,form"`
	}{
		F1: nil,
	},
	expectURLString: "http://localhost:8081/user",
}, {
	about:     "form values in body",
	urlString: "http://localhost:8081",
	val: &struct {
		F1 string   `httprequest:"f1,form,inbody"`
		F2 []string `httprequest:"f2,form,inbody"`
		F3 string   `httprequest:"f3,form"`
	}{
		F1: "f1",
		F2: []string{"f2.1", "f2.2"},
		F3: "f3",
	},
	expectURLString: "http://localhost:8081?f3=f3",
	expectHeader: http.Header{
		"Content-Type": {"application/x-www-form-urlencoded"},
	},
	expectBody: newString(`f1=f1&f2=f2.1&f2=f2.2`),
}, {
	about:     "form inbody values with explicit body",
	urlString: "http://localhost:8081",
	val: &struct {
		F1 string `httprequest:"f1,form,inbody"`
		F2 string `httprequest:"f3,body"`
	}{},
	expectError: `bad type .*: cannot specify inbody field with a body field`,
}, {
	about:     "cannot marshal []string field to path",
	urlString: "http://localhost:8081/:users",
	val: &struct {
		F1 []string `httprequest:"users,path"`
	}{
		F1: []string{"user1", "user2"},
	},
	expectError: `bad type \*struct { F1 \[\]string "httprequest:\\"users,path\\"" }: invalid target type \[\]string for path parameter`,
}, {
	about:     "[]string field fails to marshal to path",
	urlString: "http://localhost:8081/user/:users",
	val: &struct {
		F1 []string `httprequest:"users,path"`
	}{
		F1: []string{"user1", "user2", "user3"},
	},
	expectError: "bad type .*: invalid target type.*",
}, {
	about:     "omitempty on body",
	urlString: "http://localhost:8081/:users",
	val: &struct {
		Body string `httprequest:",body,omitempty"`
	}{},
	expectError: `bad type \*struct { Body string "httprequest:\\",body,omitempty\\"" }: bad tag "httprequest:\\",body,omitempty\\"" in field Body: can only use omitempty with form or header fields`,
}, {
	about:     "omitempty on path",
	urlString: "http://localhost:8081/:Users",
	val: &struct {
		Users string `httprequest:",path,omitempty"`
	}{},
	expectError: `bad type \*struct { Users string "httprequest:\\",path,omitempty\\"" }: bad tag "httprequest:\\",path,omitempty\\"" in field Users: can only use omitempty with form or header fields`,
}, {
	about:     "more than one field with body tag",
	urlString: "http://localhost:8081/user",
	method:    "POST",
	val: &struct {
		F1 string `httprequest:"user,body"`
		F2 int    `httprequest:"age,body"`
	}{
		F1: "test user",
		F2: 42,
	},
	expectError: "bad type .*: more than one body field specified",
}, {
	about:     "required path parameter, but not specified",
	urlString: "http://localhost:8081/u/:username",
	method:    "POST",
	val: &struct {
		F1 string `httprequest:"user,body"`
	}{
		F1: "test user",
	},
	expectError: `missing value for path parameter "username"`,
}, {
	about:     "marshal to body",
	urlString: "http://localhost:8081/u",
	method:    "POST",
	val: &struct {
		F1 embedded `httprequest:"info,body"`
	}{
		F1: embedded{
			F1: "test user",
			F2: 42,
			F3: newString("test address"),
		},
	},
	expectBody: newString(`{"name":"test user","age":42,"address":"test address"}`),
}, {
	about:     "empty path wildcard",
	urlString: "http://localhost:8081/u/:",
	method:    "POST",
	val: &struct {
		F1 string `httprequest:"user,body"`
	}{
		F1: "test user",
	},
	expectError: "empty path parameter",
}, {
	about:     "nil field to form",
	urlString: "http://localhost:8081/u",
	val: &struct {
		F1 *string `httprequest:"user,form"`
	}{},
	expectURLString: "http://localhost:8081/u",
}, {
	about:     "nil field to path",
	urlString: "http://localhost:8081/u",
	val: &struct {
		F1 *string `httprequest:"user,path"`
	}{},
	expectURLString: "http://localhost:8081/u",
}, {
	about:     "marshal to body of a GET request",
	urlString: "http://localhost:8081/u",
	val: &struct {
		F1 string `httprequest:",body"`
	}{
		F1: "hello test",
	},
	// Providing a body to a GET request is unusual but
	// some people do it anyway.

	expectBody: newString(`"hello test"`),
}, {
	about:     "marshal to nil value to body",
	urlString: "http://localhost:8081/u",
	val: &struct {
		F1 *string `httprequest:",body"`
	}{
		F1: nil,
	},
	expectBody: newString(""),
}, {
	about:     "nil TextMarshaler",
	urlString: "http://localhost:8081/u",
	val: &struct {
		F1 *testMarshaler `httprequest:"surname,form"`
	}{
		F1: (*testMarshaler)(nil),
	},
	expectURLString: "http://localhost:8081/u",
}, {
	about:     "marshal nil with Sprint",
	urlString: "http://localhost:8081/u",
	val: &struct {
		F1 *int `httprequest:"surname,form"`
	}{
		F1: (*int)(nil),
	},
	expectURLString: "http://localhost:8081/u",
}, {
	about:     "marshal to path with * placeholder",
	urlString: "http://localhost:8081/u/*name",
	val: &struct {
		F1 string `httprequest:"name,path"`
	}{
		F1: "/test",
	},
	expectURLString: "http://localhost:8081/u/test",
}, {
	about:     "marshal to path with * placeholder, but the marshaled value does not start with /",
	urlString: "http://localhost:8081/u/*name",
	val: &struct {
		F1 string `httprequest:"name,path"`
	}{
		F1: "test",
	},
	expectError: `value \"test\" for path parameter \"\*name\" does not start with required /`,
}, {
	about:     "* placeholder allowed only at the end",
	urlString: "http://localhost:8081/u/*name/document",
	val: &struct {
		F1 string `httprequest:"name,path"`
	}{
		F1: "test",
	},
	expectError: "star path parameter is not at end of path",
}, {
	about:     "unparsable base url string",
	urlString: "%%",
	val: &struct {
		F1 string `httprequest:"name,form"`
	}{
		F1: "test",
	},
	expectError: `parse %%: invalid URL escape \"%%\"`,
}, {
	about:     "value cannot be marshaled to json",
	urlString: "http://localhost",
	method:    "POST",
	val: &struct {
		F1 failJSONMarshaler `httprequest:"field,body"`
	}{
		F1: "test",
	},
	expectError: `cannot marshal field: cannot marshal request body: json: error calling MarshalJSON for type \*httprequest_test.failJSONMarshaler: marshal error`,
}, {
	about:     "url with query parameters",
	urlString: "http://localhost?a=b",
	method:    "POST",
	val: &struct {
		F1 failJSONMarshaler `httprequest:"f1,form"`
	}{
		F1: "test",
	},
	expectURLString: "http://localhost?a=b&f1=test",
}, {
	about:           "url with query parameters no form",
	urlString:       "http://localhost?a=b",
	method:          "POST",
	val:             &struct{}{},
	expectURLString: "http://localhost?a=b",
}, {
	about:     "struct with headers",
	urlString: "http://localhost:8081/",
	val: &struct {
		F01 string     `httprequest:",header"`
		F02 int        `httprequest:",header"`
		F03 bool       `httprequest:",header"`
		F04 string     `httprequest:",header,omitempty"`
		F05 string     `httprequest:",header,omitempty"`
		F06 time.Time  `httprequest:",header,omitempty"`
		F07 time.Time  `httprequest:",header,omitempty"`
		F08 *time.Time `httprequest:",header,omitempty"`
		F09 *time.Time `httprequest:",header,omitempty"`
		F10 stringer   `httprequest:",header,omitempty"`
		F11 stringer   `httprequest:",header,omitempty"`
		F12 *stringer  `httprequest:",header,omitempty"`
		F13 *stringer  `httprequest:",header,omitempty"`
		F14 *stringer  `httprequest:",header,omitempty"`
		F15 time.Time  `httprequest:",header"`
		// Note that this gets omitted anyway because it's nil.
		F16 *time.Time `httprequest:",header"`
	}{
		F01: "some text",
		F02: 99,
		F03: true,
		F04: "",
		F05: "something",
		F07: time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC),
		F09: func() *time.Time {
			t := time.Date(2011, 2, 3, 4, 5, 6, 0, time.UTC)
			return &t
		}(),
		F11: stringer(99),
		F13: func() *stringer {
			s := stringer(99)
			return &s
		}(),
		F14: new(stringer),
	},
	expectURLString: "http://localhost:8081/",
	expectHeader: http.Header{
		"F01": {"some text"},
		"F02": {"99"},
		"F03": {"true"},
		"F05": {"something"},
		"F07": {"2001-02-03T04:05:06Z"},
		"F09": {"2011-02-03T04:05:06Z"},
		"F11": {"str99"},
		"F13": {"str99"},
		"F15": {"0001-01-01T00:00:00Z"},
	},
}, {
	about:     "struct with header slice",
	urlString: "http://localhost:8081/:F1",
	val: &struct {
		F1 int      `httprequest:",path"`
		F2 string   `httprequest:",form"`
		F3 []string `httprequest:",header"`
	}{
		F1: 99,
		F2: "some text",
		F3: []string{"A", "B", "C"},
	},
	expectURLString: "http://localhost:8081/99?F2=some+text",
	expectHeader:    http.Header{"F3": []string{"A", "B", "C"}},
}, {
	about:     "SetHeader called after marshaling",
	urlString: "http://localhost:8081/",
	val: &httprequest.CustomHeader{
		Body: &struct {
			F1 string `httprequest:",header"`
			F2 int    `httprequest:",header"`
			F3 bool   `httprequest:",header"`
		}{
			F1: "some text",
			F2: 99,
			F3: false,
		},
		SetHeaderFunc: func(h http.Header) {
			h.Set("F2", "some other text")
		},
	},
	expectURLString: "http://localhost:8081/",
	expectHeader: http.Header{
		"F1": {"some text"},
		"F2": {"some other text"},
		"F3": {"false"},
	},
}}

func getStruct() interface{} {
	return &struct {
		F1 string
	}{
		F1: "hello",
	}
}

func (*marshalSuite) TestMarshal(c *gc.C) {
	for i, test := range marshalTests {
		c.Logf("%d: %s", i, test.about)
		method := "GET"
		if test.method != "" {
			method = test.method
		}
		req, err := httprequest.Marshal(test.urlString, method, test.val)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			continue
		}
		c.Assert(err, gc.IsNil)
		if test.expectURLString != "" {
			c.Assert(req.URL.String(), gc.DeepEquals, test.expectURLString)
		}
		if test.expectBody != nil {
			data, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gc.IsNil)
			if *test.expectBody != "" && test.expectHeader["Content-Type"] == nil {
				c.Assert(req.Header.Get("Content-Type"), gc.Equals, "application/json")
			}
			c.Assert(string(data), gc.DeepEquals, *test.expectBody)
		}
		for k, v := range test.expectHeader {
			c.Assert(req.Header[k], gc.DeepEquals, v, gc.Commentf("key %q", k))
		}
	}
}

type testMarshaler string

func (t *testMarshaler) MarshalText() ([]byte, error) {
	if len(*t) == 0 {
		return nil, errgo.New("empty string")
	}
	return []byte("test_" + *t), nil
}

type notTextMarshaler string

// MarshalText does *not* implement encoding.TextMarshaler
func (t *notTextMarshaler) MarshalText() {
	panic("unexpected call")
}

type failJSONMarshaler string

func (*failJSONMarshaler) MarshalJSON() ([]byte, error) {
	return nil, errgo.New("marshal error")
}

type stringer int

func (s stringer) String() string {
	return fmt.Sprintf("str%d", int(s))
}
