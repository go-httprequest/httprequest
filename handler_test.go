// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package httprequest_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/qthttptest"
	"github.com/julienschmidt/httprouter"
	"gopkg.in/errgo.v1"

	"gopkg.in/httprequest.v1"
)

type customError struct {
	httprequest.RemoteError
}

var handleTests = []struct {
	about        string
	f            func(c *qt.C) interface{}
	req          *http.Request
	pathVar      httprouter.Params
	expectMethod string
	expectPath   string
	expectBody   interface{}
	expectStatus int
}{{
	about: "function with no return",
	f: func(c *qt.C) interface{} {
		type testStruct struct {
			A string         `httprequest:"a,path"`
			B map[string]int `httprequest:",body"`
			C int            `httprequest:"c,form"`
		}
		return func(p httprequest.Params, s *testStruct) {
			c.Assert(s, qt.DeepEquals, &testStruct{
				A: "A",
				B: map[string]int{"hello": 99},
				C: 43,
			})
			c.Assert(p.PathVar, qt.DeepEquals, httprouter.Params{{
				Key:   "a",
				Value: "A",
			}})
			c.Assert(p.Request.Form, qt.DeepEquals, url.Values{
				"c": {"43"},
			})
			c.Assert(p.PathPattern, qt.Equals, "")
			p.Response.Header().Set("Content-Type", "application/json")
			p.Response.Write([]byte("true"))
		}
	},
	req: &http.Request{
		Header: http.Header{"Content-Type": {"application/json"}},
		Form: url.Values{
			"c": {"43"},
		},
		Body: body(`{"hello": 99}`),
	},
	pathVar: httprouter.Params{{
		Key:   "a",
		Value: "A",
	}},
	expectBody: true,
}, {
	about: "function with error return that returns no error",
	f: func(c *qt.C) interface{} {
		type testStruct struct {
			A int `httprequest:"a,path"`
		}
		return func(p httprequest.Params, s *testStruct) error {
			c.Assert(s, qt.DeepEquals, &testStruct{123})
			c.Assert(p.PathPattern, qt.Equals, "")
			p.Response.Header().Set("Content-Type", "application/json")
			p.Response.Write([]byte("true"))
			return nil
		}
	},
	req: &http.Request{},
	pathVar: httprouter.Params{{
		Key:   "a",
		Value: "123",
	}},
	expectBody: true,
}, {
	about: "function with error return that returns an error",
	f: func(c *qt.C) interface{} {
		type testStruct struct {
			A int `httprequest:"a,path"`
		}
		return func(p httprequest.Params, s *testStruct) error {
			c.Assert(p.PathPattern, qt.Equals, "")
			c.Assert(s, qt.DeepEquals, &testStruct{123})
			return errUnauth
		}
	},
	req: &http.Request{},
	pathVar: httprouter.Params{{
		Key:   "a",
		Value: "123",
	}},
	expectBody: httprequest.RemoteError{
		Message: errUnauth.Error(),
		Code:    "unauthorized",
	},
	expectStatus: http.StatusUnauthorized,
}, {
	about: "function with value return that returns a value",
	f: func(c *qt.C) interface{} {
		type testStruct struct {
			A int `httprequest:"a,path"`
		}
		return func(p httprequest.Params, s *testStruct) (int, error) {
			c.Assert(p.PathPattern, qt.Equals, "")
			c.Assert(s, qt.DeepEquals, &testStruct{123})
			return 1234, nil
		}
	},
	req: &http.Request{},
	pathVar: httprouter.Params{{
		Key:   "a",
		Value: "123",
	}},
	expectBody: 1234,
}, {
	about: "function with value return that returns an error",
	f: func(c *qt.C) interface{} {
		type testStruct struct {
			A int `httprequest:"a,path"`
		}
		return func(p httprequest.Params, s *testStruct) (int, error) {
			c.Assert(p.PathPattern, qt.Equals, "")
			c.Assert(s, qt.DeepEquals, &testStruct{123})
			return 0, errUnauth
		}
	},
	req: &http.Request{},
	pathVar: httprouter.Params{{
		Key:   "a",
		Value: "123",
	}},
	expectBody: httprequest.RemoteError{
		Message: errUnauth.Error(),
		Code:    "unauthorized",
	},
	expectStatus: http.StatusUnauthorized,
}, {
	about: "function with value return that writes to p.Response",
	f: func(c *qt.C) interface{} {
		type testStruct struct {
			A int `httprequest:"a,path"`
		}
		return func(p httprequest.Params, s *testStruct) (int, error) {
			c.Assert(p.PathPattern, qt.Equals, "")
			_, err := p.Response.Write(nil)
			c.Assert(err, qt.ErrorMatches, "inappropriate call to ResponseWriter.Write in JSON-returning handler")
			p.Response.WriteHeader(http.StatusTeapot)
			c.Assert(s, qt.DeepEquals, &testStruct{123})
			return 1234, nil
		}
	},
	req: &http.Request{},
	pathVar: httprouter.Params{{
		Key:   "a",
		Value: "123",
	}},
	expectBody: 1234,
}, {
	about: "function with no Params and no return",
	f: func(c *qt.C) interface{} {
		type testStruct struct {
			A string         `httprequest:"a,path"`
			B map[string]int `httprequest:",body"`
			C int            `httprequest:"c,form"`
		}
		return func(s *testStruct) {
			c.Assert(s, qt.DeepEquals, &testStruct{
				A: "A",
				B: map[string]int{"hello": 99},
				C: 43,
			})
		}
	},
	req: &http.Request{
		Header: http.Header{"Content-Type": {"application/json"}},
		Form: url.Values{
			"c": {"43"},
		},
		Body: body(`{"hello": 99}`),
	},
	pathVar: httprouter.Params{{
		Key:   "a",
		Value: "A",
	}},
}, {
	about: "function with no Params with error return that returns no error",
	f: func(c *qt.C) interface{} {
		type testStruct struct {
			A int `httprequest:"a,path"`
		}
		return func(s *testStruct) error {
			c.Assert(s, qt.DeepEquals, &testStruct{123})
			return nil
		}
	},
	req: &http.Request{},
	pathVar: httprouter.Params{{
		Key:   "a",
		Value: "123",
	}},
}, {
	about: "function with no Params with error return that returns an error",
	f: func(c *qt.C) interface{} {
		type testStruct struct {
			A int `httprequest:"a,path"`
		}
		return func(s *testStruct) error {
			c.Assert(s, qt.DeepEquals, &testStruct{123})
			return errUnauth
		}
	},
	req: &http.Request{},
	pathVar: httprouter.Params{{
		Key:   "a",
		Value: "123",
	}},
	expectBody: httprequest.RemoteError{
		Message: errUnauth.Error(),
		Code:    "unauthorized",
	},
	expectStatus: http.StatusUnauthorized,
}, {
	about: "function with no Params with value return that returns a value",
	f: func(c *qt.C) interface{} {
		type testStruct struct {
			A int `httprequest:"a,path"`
		}
		return func(s *testStruct) (int, error) {
			c.Assert(s, qt.DeepEquals, &testStruct{123})
			return 1234, nil
		}
	},
	req: &http.Request{},
	pathVar: httprouter.Params{{
		Key:   "a",
		Value: "123",
	}},
	expectBody: 1234,
}, {
	about: "function with no Params with value return that returns an error",
	f: func(c *qt.C) interface{} {
		type testStruct struct {
			A int `httprequest:"a,path"`
		}
		return func(s *testStruct) (int, error) {
			c.Assert(s, qt.DeepEquals, &testStruct{123})
			return 0, errUnauth
		}
	},
	req: &http.Request{},
	pathVar: httprouter.Params{{
		Key:   "a",
		Value: "123",
	}},
	expectBody: httprequest.RemoteError{
		Message: errUnauth.Error(),
		Code:    "unauthorized",
	},
	expectStatus: http.StatusUnauthorized,
}, {
	about: "error when unmarshaling",
	f: func(c *qt.C) interface{} {
		type testStruct struct {
			A int `httprequest:"a,path"`
		}
		return func(p httprequest.Params, s *testStruct) (int, error) {
			c.Errorf("function should not have been called")
			return 0, nil
		}
	},
	req: &http.Request{},
	pathVar: httprouter.Params{{
		Key:   "a",
		Value: "not a number",
	}},
	expectBody: httprequest.RemoteError{
		Message: `cannot unmarshal parameters: cannot unmarshal into field A: cannot parse "not a number" into int: expected integer`,
		Code:    "bad request",
	},
	expectStatus: http.StatusBadRequest,
}, {
	about: "error when unmarshaling, no Params",
	f: func(c *qt.C) interface{} {
		type testStruct struct {
			A int `httprequest:"a,path"`
		}
		return func(s *testStruct) (int, error) {
			c.Errorf("function should not have been called")
			return 0, nil
		}
	},
	req: &http.Request{},
	pathVar: httprouter.Params{{
		Key:   "a",
		Value: "not a number",
	}},
	expectBody: httprequest.RemoteError{
		Message: `cannot unmarshal parameters: cannot unmarshal into field A: cannot parse "not a number" into int: expected integer`,
		Code:    "bad request",
	},
	expectStatus: http.StatusBadRequest,
}, {
	about: "error when unmarshaling single value return",
	f: func(c *qt.C) interface{} {
		type testStruct struct {
			A int `httprequest:"a,path"`
		}
		return func(p httprequest.Params, s *testStruct) error {
			c.Errorf("function should not have been called")
			return nil
		}
	},
	req: &http.Request{},
	pathVar: httprouter.Params{{
		Key:   "a",
		Value: "not a number",
	}},
	expectBody: httprequest.RemoteError{
		Message: `cannot unmarshal parameters: cannot unmarshal into field A: cannot parse "not a number" into int: expected integer`,
		Code:    "bad request",
	},
	expectStatus: http.StatusBadRequest,
}, {
	about: "return type that can't be marshaled as JSON",
	f: func(c *qt.C) interface{} {
		return func(p httprequest.Params, s *struct{}) (chan int, error) {
			return make(chan int), nil
		}
	},
	req:     &http.Request{},
	pathVar: httprouter.Params{},
	expectBody: httprequest.RemoteError{
		Message: "json: unsupported type: chan int",
	},
	expectStatus: http.StatusInternalServerError,
}, {
	about: "argument with route",
	f: func(c *qt.C) interface{} {
		type testStruct struct {
			httprequest.Route `httprequest:"GET /foo/:bar"`
			A                 string `httprequest:"bar,path"`
		}
		return func(p httprequest.Params, s *testStruct) {
			c.Check(s.A, qt.Equals, "val")
			c.Assert(p.PathPattern, qt.Equals, "/foo/:bar")
		}
	},
	req: &http.Request{},
	pathVar: httprouter.Params{{
		Key:   "bar",
		Value: "val",
	}},
	expectMethod: "GET",
	expectPath:   "/foo/:bar",
}}

func TestHandle(t *testing.T) {
	c := qt.New(t)

	for _, test := range handleTests {
		test := test
		c.Run(test.about, func(c *qt.C) {
			h := testServer.Handle(test.f(c))
			c.Assert(h.Method, qt.Equals, test.expectMethod)
			c.Assert(h.Path, qt.Equals, test.expectPath)
			rec := httptest.NewRecorder()
			h.Handle(rec, test.req, test.pathVar)
			if test.expectStatus == 0 {
				test.expectStatus = http.StatusOK
			}
			qthttptest.AssertJSONResponse(c, rec, test.expectStatus, test.expectBody)
		})
	}
}

var handlePanicTests = []struct {
	name   string
	f      interface{}
	expect string
}{{
	name:   "not-a-function",
	f:      42,
	expect: "bad handler function: not a function",
}, {
	name:   "no-argument",
	f:      func(httprequest.Params) {},
	expect: "bad handler function: no argument parameter after Params argument",
}, {
	name:   "too-many-parameters",
	f:      func(httprequest.Params, *struct{}, struct{}) {},
	expect: "bad handler function: has 3 parameters, need 1 or 2",
}, {
	name:   "bad-return-type",
	f:      func(httprequest.Params, *struct{}) struct{} { return struct{}{} },
	expect: "bad handler function: final result parameter is struct {}, need error",
}, {
	name: "bad-first-parameter",
	f: func(http.ResponseWriter, httprequest.Params) (struct{}, error) {
		return struct{}{}, nil
	},
	expect: "bad handler function: first argument is http.ResponseWriter, need httprequest.Params",
}, {
	name: "bad-final-return-type",
	f: func(httprequest.Params, *struct{}) (struct{}, struct{}) {
		return struct{}{}, struct{}{}
	},
	expect: "bad handler function: final result parameter is struct {}, need error",
}, {
	name:   "bad-first-parameter2",
	f:      func(*http.Request, *struct{}) {},
	expect: `bad handler function: first argument is \*http.Request, need httprequest.Params`,
}, {
	name:   "parameter-not-pointer",
	f:      func(httprequest.Params, struct{}) {},
	expect: "bad handler function: last argument cannot be used for Unmarshal: type is not pointer to struct",
}, {
	name: "invalid-tag",
	f: func(httprequest.Params, *struct {
		A int `httprequest:"a,the-ether"`
	}) {
	},
	expect: `bad handler function: last argument cannot be used for Unmarshal: bad tag "httprequest:\\"a,the-ether\\"" in field A: unknown tag flag "the-ether"`,
}, {
	name:   "too-many-results",
	f:      func(httprequest.Params, *struct{}) (a, b, c struct{}) { return },
	expect: `bad handler function: has 3 result parameters, need 0, 1 or 2`,
}, {
	name: "no-route-tag",
	f: func(*struct {
		httprequest.Route
	}) {
	},
	expect: `bad handler function: last argument cannot be used for Unmarshal: bad route tag "": no httprequest tag`,
}, {
	name: "invalid-route-tag",
	f: func(*struct {
		httprequest.Route `othertag:"foo"`
	}) {
	},
	expect: `bad handler function: last argument cannot be used for Unmarshal: bad route tag "othertag:\\"foo\\"": no httprequest tag`,
}, {
	name: "invalid-route-tag-value",
	f: func(*struct {
		httprequest.Route `httprequest:""`
	}) {
	},
	expect: `bad handler function: last argument cannot be used for Unmarshal: bad route tag "httprequest:\\"\\"": no httprequest tag`,
}, {
	name: "invalid-route-tag-too-many-fields",
	f: func(*struct {
		httprequest.Route `httprequest:"GET /foo /bar"`
	}) {
	},
	expect: `bad handler function: last argument cannot be used for Unmarshal: bad route tag "httprequest:\\"GET /foo /bar\\"": wrong field count`,
}, {
	name: "invalid-route-tag-invalid-method",
	f: func(*struct {
		httprequest.Route `httprequest:"BAD /foo"`
	}) {
	},
	expect: `bad handler function: last argument cannot be used for Unmarshal: bad route tag "httprequest:\\"BAD /foo\\"": invalid method`,
}}

func TestHandlePanicsWithBadFunctions(t *testing.T) {
	c := qt.New(t)

	for _, test := range handlePanicTests {
		c.Run(test.name, func(c *qt.C) {
			c.Check(func() {
				testServer.Handle(test.f)
			}, qt.PanicMatches, test.expect)
		})
	}
}

var handlersTests = []struct {
	calledMethod      string
	callParams        qthttptest.JSONCallParams
	expectPathPattern string
}{{
	calledMethod: "M1",
	callParams: qthttptest.JSONCallParams{
		URL: "/m1/99",
	},
	expectPathPattern: "/m1/:p",
}, {
	calledMethod: "M2",
	callParams: qthttptest.JSONCallParams{
		URL:        "/m2/99",
		ExpectBody: 999,
	},
	expectPathPattern: "/m2/:p",
}, {
	calledMethod: "M3",
	callParams: qthttptest.JSONCallParams{
		URL: "/m3/99",
		ExpectBody: &httprequest.RemoteError{
			Message: "m3 error",
		},
		ExpectStatus: http.StatusInternalServerError,
	},
	expectPathPattern: "/m3/:p",
}, {
	calledMethod: "M3Post",
	callParams: qthttptest.JSONCallParams{
		Method:   "POST",
		URL:      "/m3/99",
		JSONBody: make(map[string]interface{}),
	},
	expectPathPattern: "/m3/:p",
}}

func TestHandlers(t *testing.T) {
	c := qt.New(t)

	handleVal := testHandlers{
		c: c,
	}
	f := func(p httprequest.Params) (*testHandlers, context.Context, error) {
		handleVal.p = p
		return &handleVal, p.Context, nil
	}
	handlers := testServer.Handlers(f)
	handlers1 := make([]httprequest.Handler, len(handlers))
	copy(handlers1, handlers)
	for i := range handlers1 {
		handlers1[i].Handle = nil
	}
	expectHandlers := []httprequest.Handler{{
		Method: "GET",
		Path:   "/m1/:p",
	}, {
		Method: "GET",
		Path:   "/m2/:p",
	}, {
		Method: "GET",
		Path:   "/m3/:p",
	}, {
		Method: "POST",
		Path:   "/m3/:p",
	}}
	c.Assert(handlers1, qt.DeepEquals, expectHandlers)
	c.Assert(handlersTests, qt.HasLen, len(expectHandlers))

	router := httprouter.New()
	for _, h := range handlers {
		c.Logf("adding %s %s", h.Method, h.Path)
		router.Handle(h.Method, h.Path, h.Handle)
	}
	for _, test := range handlersTests {
		test := test
		c.Run(test.calledMethod, func(c *qt.C) {
			handleVal = testHandlers{
				c: c,
			}
			test.callParams.Handler = router
			qthttptest.AssertJSONCall(c, test.callParams)
			c.Assert(handleVal.calledMethod, qt.Equals, test.calledMethod)
			c.Assert(handleVal.p.PathPattern, qt.Equals, test.expectPathPattern)
		})
	}
}

type testHandlers struct {
	calledMethod  string
	calledContext context.Context
	c             *qt.C
	p             httprequest.Params
}

func (h *testHandlers) M1(p httprequest.Params, arg *struct {
	httprequest.Route `httprequest:"GET /m1/:p"`
	P                 int `httprequest:"p,path"`
}) {
	h.calledMethod = "M1"
	h.calledContext = p.Context
	h.c.Check(arg.P, qt.Equals, 99)
	h.c.Check(p.Response, qt.Equals, h.p.Response)
	h.c.Check(p.Request, qt.Equals, h.p.Request)
	h.c.Check(p.PathVar, qt.DeepEquals, h.p.PathVar)
	h.c.Check(p.PathPattern, qt.Equals, "/m1/:p")
	h.c.Check(p.Context, qt.Not(qt.IsNil))
}

type m2Request struct {
	httprequest.Route `httprequest:"GET /m2/:p"`
	P                 int `httprequest:"p,path"`
}

func (h *testHandlers) M2(arg *m2Request) (int, error) {
	h.calledMethod = "M2"
	h.c.Check(arg.P, qt.Equals, 99)
	return 999, nil
}

func (h *testHandlers) unexported() {
}

func (h *testHandlers) M3(arg *struct {
	httprequest.Route `httprequest:"GET /m3/:p"`
	P                 int `httprequest:"p,path"`
}) (int, error) {
	h.calledMethod = "M3"
	h.c.Check(arg.P, qt.Equals, 99)
	return 0, errgo.New("m3 error")
}

func (h *testHandlers) M3Post(arg *struct {
	httprequest.Route `httprequest:"POST /m3/:p"`
	P                 int `httprequest:"p,path"`
}) {
	h.calledMethod = "M3Post"
	h.c.Check(arg.P, qt.Equals, 99)
}

func TestHandlersRootFuncWithRequestArg(t *testing.T) {
	c := qt.New(t)

	handleVal := testHandlers{
		c: c,
	}
	var gotArg interface{}
	f := func(p httprequest.Params, arg interface{}) (*testHandlers, context.Context, error) {
		gotArg = arg
		return &handleVal, p.Context, nil
	}
	router := httprouter.New()
	for _, h := range testServer.Handlers(f) {
		router.Handle(h.Method, h.Path, h.Handle)
	}
	qthttptest.AssertJSONCall(c, qthttptest.JSONCallParams{
		Handler:    router,
		URL:        "/m2/99",
		ExpectBody: 999,
	})
	c.Assert(gotArg, qt.DeepEquals, &m2Request{
		P: 99,
	})
}

func TestHandlersRootFuncReturningInterface(t *testing.T) {
	c := qt.New(t)

	handleVal := testHandlers{
		c: c,
	}
	type testHandlersI interface {
		M2(arg *m2Request) (int, error)
	}
	f := func(p httprequest.Params) (testHandlersI, context.Context, error) {
		return &handleVal, p.Context, nil
	}
	router := httprouter.New()
	for _, h := range testServer.Handlers(f) {
		router.Handle(h.Method, h.Path, h.Handle)
	}
	qthttptest.AssertJSONCall(c, qthttptest.JSONCallParams{
		Handler:    router,
		URL:        "/m2/99",
		ExpectBody: 999,
	})
}

func TestHandlersRootFuncWithIncompatibleRequestArg(t *testing.T) {
	c := qt.New(t)

	handleVal := testHandlers{
		c: c,
	}
	f := func(p httprequest.Params, arg interface {
		Foo()
	}) (*testHandlers, context.Context, error) {
		return &handleVal, p.Context, nil
	}
	c.Assert(func() {
		testServer.Handlers(f)
	}, qt.PanicMatches, `bad type for method M1: argument of type \*struct {.*} does not implement interface required by root handler interface \{ Foo\(\) \}`)
}

func TestHandlersRootFuncWithNonEmptyInterfaceRequestArg(t *testing.T) {
	c := qt.New(t)

	type tester interface {
		Test() string
	}
	var argResult string
	f := func(p httprequest.Params, arg tester) (*handlersWithRequestMethod, context.Context, error) {
		argResult = arg.Test()
		return &handlersWithRequestMethod{}, p.Context, nil
	}
	router := httprouter.New()
	for _, h := range testServer.Handlers(f) {
		router.Handle(h.Method, h.Path, h.Handle)
	}
	qthttptest.AssertJSONCall(c, qthttptest.JSONCallParams{
		Handler:    router,
		URL:        "/x1/something",
		ExpectBody: "something",
	})
	c.Assert(argResult, qt.DeepEquals, "test something")
}

var badHandlersFuncTests = []struct {
	about       string
	f           interface{}
	expectPanic string
}{{
	about:       "not a function",
	f:           123,
	expectPanic: "bad handler function: expected function, got int",
}, {
	about:       "nil function",
	f:           (func())(nil),
	expectPanic: "bad handler function: function is nil",
}, {
	about:       "no arguments",
	f:           func() {},
	expectPanic: "bad handler function: got 0 arguments, want 1 or 2",
}, {
	about:       "more than two argument",
	f:           func(http.ResponseWriter, *http.Request, int) {},
	expectPanic: "bad handler function: got 3 arguments, want 1 or 2",
}, {
	about:       "no return values",
	f:           func(httprequest.Params) {},
	expectPanic: `bad handler function: function returns 0 values, want \(<T>, context.Context, error\)`,
}, {
	about:       "only one return value",
	f:           func(httprequest.Params) string { return "" },
	expectPanic: `bad handler function: function returns 1 values, want \(<T>, context.Context, error\)`,
}, {
	about:       "only two return values",
	f:           func(httprequest.Params) (_ arithHandler, _ error) { return },
	expectPanic: `bad handler function: function returns 2 values, want \(<T>, context.Context, error\)`,
}, {
	about:       "too many return values",
	f:           func(httprequest.Params) (_ string, _ error, _ error, _ error) { return },
	expectPanic: `bad handler function: function returns 4 values, want \(<T>, context.Context, error\)`,
}, {
	about:       "invalid first argument",
	f:           func(string) (_ string, _ context.Context, _ error) { return },
	expectPanic: `bad handler function: invalid first argument, want httprequest.Params, got string`,
}, {
	about:       "second argument not an interface",
	f:           func(httprequest.Params, *http.Request) (_ string, _ context.Context, _ error) { return },
	expectPanic: `bad handler function: invalid second argument, want interface type, got \*http.Request`,
}, {
	about:       "non-error return",
	f:           func(httprequest.Params) (_ string, _ context.Context, _ string) { return },
	expectPanic: `bad handler function: invalid third return parameter, want error, got string`,
}, {
	about:       "non-context return",
	f:           func(httprequest.Params) (_ arithHandler, _ string, _ error) { return },
	expectPanic: `bad handler function: second return parameter of type string does not implement context.Context`,
}, {
	about:       "no methods on return type",
	f:           func(httprequest.Params) (_ string, _ context.Context, _ error) { return },
	expectPanic: `no exported methods defined on string`,
}, {
	about:       "method with invalid parameter count",
	f:           func(httprequest.Params) (_ badHandlersType1, _ context.Context, _ error) { return },
	expectPanic: `bad type for method M: has 3 parameters, need 1 or 2`,
}, {
	about:       "method with invalid route",
	f:           func(httprequest.Params) (_ badHandlersType2, _ context.Context, _ error) { return },
	expectPanic: `method M does not specify route method and path`,
}, {
	about:       "bad type for close method",
	f:           func(httprequest.Params) (_ badHandlersType3, _ context.Context, _ error) { return },
	expectPanic: `bad type for Close method \(got func\(httprequest_test\.badHandlersType3\) want func\(httprequest_test.badHandlersType3\) error`,
}}

type badHandlersType1 struct{}

func (badHandlersType1) M(a, b, c int) {
}

type badHandlersType2 struct{}

func (badHandlersType2) M(*struct {
	P int `httprequest:",path"`
}) {
}

type badHandlersType3 struct{}

func (badHandlersType3) M(arg *struct {
	httprequest.Route `httprequest:"GET /m1/:P"`
	P                 int `httprequest:",path"`
}) {
}

func (badHandlersType3) Close() {
}

func TestBadHandlersFunc(t *testing.T) {
	c := qt.New(t)

	for _, test := range badHandlersFuncTests {
		test := test
		c.Run(test.about, func(c *qt.C) {
			c.Check(func() {
				testServer.Handlers(test.f)
			}, qt.PanicMatches, test.expectPanic)
		})
	}
}

func TestHandlersFuncReturningError(t *testing.T) {
	c := qt.New(t)

	handlers := testServer.Handlers(func(p httprequest.Params) (*testHandlers, context.Context, error) {
		return nil, p.Context, errgo.WithCausef(errgo.New("failure"), errUnauth, "something")
	})
	router := httprouter.New()
	for _, h := range handlers {
		router.Handle(h.Method, h.Path, h.Handle)
	}
	qthttptest.AssertJSONCall(c, qthttptest.JSONCallParams{
		URL:          "/m1/99",
		Handler:      router,
		ExpectStatus: http.StatusUnauthorized,
		ExpectBody: &httprequest.RemoteError{
			Message: "something: failure",
			Code:    "unauthorized",
		},
	})
}

func TestHandlersFuncReturningCustomContext(t *testing.T) {
	c := qt.New(t)

	handleVal := testHandlers{
		c: c,
	}
	handlers := testServer.Handlers(func(p httprequest.Params) (*testHandlers, context.Context, error) {
		handleVal.p = p
		ctx := context.WithValue(p.Context, "some key", "some value")
		return &handleVal, ctx, nil
	})
	router := httprouter.New()
	for _, h := range handlers {
		router.Handle(h.Method, h.Path, h.Handle)
	}
	qthttptest.AssertJSONCall(c, qthttptest.JSONCallParams{
		URL:     "/m1/99",
		Handler: router,
	})
	c.Assert(handleVal.calledContext, qt.Not(qt.IsNil))
	c.Assert(handleVal.calledContext.Value("some key"), qt.Equals, "some value")
}

type closeHandlersType struct {
	p      int
	closed bool
}

func (h *closeHandlersType) M(arg *struct {
	httprequest.Route `httprequest:"GET /m1/:P"`
	P                 int `httprequest:",path"`
}) {
	h.p = arg.P
}

func (h *closeHandlersType) Close() error {
	h.closed = true
	return nil
}

func TestHandlersWithTypeThatImplementsIOCloser(t *testing.T) {
	c := qt.New(t)

	var v closeHandlersType
	handlers := testServer.Handlers(func(p httprequest.Params) (*closeHandlersType, context.Context, error) {
		return &v, p.Context, nil
	})
	router := httprouter.New()
	for _, h := range handlers {
		router.Handle(h.Method, h.Path, h.Handle)
	}
	qthttptest.AssertJSONCall(c, qthttptest.JSONCallParams{
		URL:     "/m1/99",
		Handler: router,
	})
	c.Assert(v.closed, qt.Equals, true)
	c.Assert(v.p, qt.Equals, 99)
}

func TestBadForm(t *testing.T) {
	c := qt.New(t)

	h := testServer.Handle(func(p httprequest.Params, _ *struct{}) {
		c.Fatalf("shouldn't be called")
	})
	testBadForm(c, h.Handle)
}

func TestBadFormNoParams(t *testing.T) {
	c := qt.New(t)

	h := testServer.Handle(func(_ *struct{}) {
		c.Fatalf("shouldn't be called")
	})
	testBadForm(c, h.Handle)
}

func testBadForm(c *qt.C, h httprouter.Handle) {
	rec := httptest.NewRecorder()
	req := &http.Request{
		Method: "POST",
		Header: http.Header{
			"Content-Type": {"application/x-www-form-urlencoded"},
		},
		Body: body("%6"),
	}
	h(rec, req, httprouter.Params{})
	qthttptest.AssertJSONResponse(c, rec, http.StatusBadRequest, httprequest.RemoteError{
		Message: `cannot parse HTTP request form: invalid URL escape "%6"`,
		Code:    "bad request",
	})
}

func TestToHTTP(t *testing.T) {
	c := qt.New(t)

	var h http.Handler
	h = httprequest.ToHTTP(testServer.Handle(func(p httprequest.Params, s *struct{}) {
		c.Assert(p.PathVar, qt.IsNil)
		p.Response.WriteHeader(http.StatusOK)
	}).Handle)
	rec := httptest.NewRecorder()
	req := &http.Request{
		Body: body(""),
	}
	h.ServeHTTP(rec, req)
	c.Assert(rec.Code, qt.Equals, http.StatusOK)
}

func TestWriteJSON(t *testing.T) {
	c := qt.New(t)

	rec := httptest.NewRecorder()
	type Number struct {
		N int
	}
	err := httprequest.WriteJSON(rec, http.StatusTeapot, Number{1234})
	c.Assert(err, qt.IsNil)
	c.Assert(rec.Code, qt.Equals, http.StatusTeapot)
	c.Assert(rec.Body.String(), qt.Equals, `{"N":1234}`)
	c.Assert(rec.Header().Get("content-type"), qt.Equals, "application/json")
}

var (
	errUnauth             = errors.New("unauth")
	errBadReq             = errors.New("bad request")
	errOther              = errors.New("other")
	errCustomHeaders      = errors.New("custom headers")
	errUnmarshalableError = errors.New("unmarshalable error")
	errNil                = errors.New("nil result")
)

type HeaderNumber struct {
	N int
}

func (HeaderNumber) SetHeader(h http.Header) {
	h.Add("some-custom-header", "yes")
}

func TestSetHeader(t *testing.T) {
	c := qt.New(t)

	rec := httptest.NewRecorder()
	err := httprequest.WriteJSON(rec, http.StatusTeapot, HeaderNumber{1234})
	c.Assert(err, qt.Equals, nil)
	c.Assert(rec.Code, qt.Equals, http.StatusTeapot)
	c.Assert(rec.Body.String(), qt.Equals, `{"N":1234}`)
	c.Assert(rec.Header().Get("content-type"), qt.Equals, "application/json")
	c.Assert(rec.Header().Get("some-custom-header"), qt.Equals, "yes")
}

var testServer = httprequest.Server{
	ErrorMapper: testErrorMapper,
}

func testErrorMapper(_ context.Context, err error) (int, interface{}) {
	resp := &httprequest.RemoteError{
		Message: err.Error(),
	}
	status := http.StatusInternalServerError
	switch errgo.Cause(err) {
	case errUnauth:
		status = http.StatusUnauthorized
		resp.Code = "unauthorized"
	case errBadReq, httprequest.ErrUnmarshal:
		status = http.StatusBadRequest
		resp.Code = "bad request"
	case errCustomHeaders:
		return http.StatusNotAcceptable, httprequest.CustomHeader{
			Body: resp,
			SetHeaderFunc: func(h http.Header) {
				h.Set("Acceptability", "not at all")
			},
		}
	case errUnmarshalableError:
		return http.StatusTeapot, make(chan int)
	case errNil:
		return status, nil
	}
	return status, &resp
}

var writeErrorTests = []struct {
	about          string
	err            error
	srv            httprequest.Server
	assertResponse func(c *qt.C, rec *httptest.ResponseRecorder)
	expectStatus   int
	expectResp     *httprequest.RemoteError
	expectHeader   http.Header
}{{
	about: "unauthorized error",
	err:   errUnauth,
	srv:   testServer,
	assertResponse: assertErrorResponse(
		http.StatusUnauthorized,
		&httprequest.RemoteError{
			Message: errUnauth.Error(),
			Code:    "unauthorized",
		},
		nil,
	),
}, {
	about: "bad request error",
	err:   errBadReq,
	srv:   testServer,
	assertResponse: assertErrorResponse(
		http.StatusBadRequest,
		&httprequest.RemoteError{
			Message: errBadReq.Error(),
			Code:    "bad request",
		},
		nil,
	),
}, {
	about: "unclassified error",
	err:   errOther,
	srv:   testServer,
	assertResponse: assertErrorResponse(
		http.StatusInternalServerError,
		&httprequest.RemoteError{
			Message: errOther.Error(),
		},
		nil,
	),
}, {
	about: "nil body",
	err:   errNil,
	srv:   testServer,
	assertResponse: assertErrorResponse(
		http.StatusInternalServerError,
		(*httprequest.RemoteError)(nil),
		nil,
	),
}, {
	about: "custom headers",
	err:   errCustomHeaders,
	srv:   testServer,
	assertResponse: assertErrorResponse(
		http.StatusNotAcceptable,
		&httprequest.RemoteError{
			Message: errCustomHeaders.Error(),
		},
		http.Header{
			"Acceptability": {"not at all"},
		},
	),
}, {
	about: "unmarshalable error",
	err:   errUnmarshalableError,
	srv:   testServer,
	assertResponse: assertErrorResponse(
		http.StatusInternalServerError,
		&httprequest.RemoteError{
			Message: `cannot marshal error response "unmarshalable error": json: unsupported type: chan int`,
		},
		nil,
	),
}, {
	about: "error with default error mapper",
	err:   errgo.Newf("some error"),
	srv:   httprequest.Server{},
	assertResponse: assertErrorResponse(
		http.StatusInternalServerError,
		&httprequest.RemoteError{
			Message: "some error",
		},
		nil,
	),
}, {
	about: "default error mapper with specific error code",
	err:   httprequest.Errorf(httprequest.CodeBadRequest, "some bad request %d", 99),
	srv:   httprequest.Server{},
	assertResponse: assertErrorResponse(
		http.StatusBadRequest,
		&httprequest.RemoteError{
			Message: "some bad request 99",
			Code:    httprequest.CodeBadRequest,
		},
		nil,
	),
}, {
	about: "edefault error mapper with specific error code with wrapped error",
	err:   errgo.NoteMask(httprequest.Errorf(httprequest.CodeBadRequest, "some bad request %d", 99), "wrap", errgo.Any),
	srv:   httprequest.Server{},
	assertResponse: assertErrorResponse(
		http.StatusBadRequest,
		&httprequest.RemoteError{
			Message: "wrap: some bad request 99",
			Code:    httprequest.CodeBadRequest,
		},
		nil,
	),
}, {
	about: "default error mapper with specific error code with wrapped error",
	err:   errgo.NoteMask(httprequest.Errorf(httprequest.CodeBadRequest, "some bad request %d", 99), "wrap", errgo.Any),
	srv:   httprequest.Server{},
	assertResponse: assertErrorResponse(
		http.StatusBadRequest,
		&httprequest.RemoteError{
			Message: "wrap: some bad request 99",
			Code:    httprequest.CodeBadRequest,
		},
		nil,
	),
}, {
	about: "default error mapper with custom error with ErrorCode implementation",
	err: &customError{
		RemoteError: httprequest.RemoteError{
			Code:    httprequest.CodeNotFound,
			Message: "bar",
		},
	},
	srv: httprequest.Server{},
	assertResponse: assertErrorResponse(
		http.StatusNotFound,
		&httprequest.RemoteError{
			Message: "bar",
			Code:    httprequest.CodeNotFound,
		},
		nil,
	),
}, {
	about: "error writer",
	err:   errBadReq,
	srv: httprequest.Server{
		ErrorWriter: func(ctx context.Context, w http.ResponseWriter, err error) {
			fmt.Fprintf(w, "custom error")
		},
	},
	assertResponse: func(c *qt.C, rec *httptest.ResponseRecorder) {
		c.Assert(rec.Body.String(), qt.Equals, "custom error")
		c.Assert(rec.Code, qt.Equals, http.StatusOK)
	},
}, {
	about: "error writer overrides error mapper",
	err:   errBadReq,
	srv: httprequest.Server{
		ErrorWriter: func(ctx context.Context, w http.ResponseWriter, err error) {
			fmt.Fprintf(w, "custom error")
		},
		ErrorMapper: func(_ context.Context, err error) (int, interface{}) {
			return http.StatusInternalServerError, nil
		},
	},
	assertResponse: func(c *qt.C, rec *httptest.ResponseRecorder) {
		c.Assert(rec.Body.String(), qt.Equals, "custom error")
		c.Assert(rec.Code, qt.Equals, http.StatusOK)
	},
}}

func TestErrorfWithEmptyMessage(t *testing.T) {
	c := qt.New(t)

	err := httprequest.Errorf(httprequest.CodeNotFound, "")
	c.Assert(err, qt.DeepEquals, &httprequest.RemoteError{
		Message: httprequest.CodeNotFound,
		Code:    httprequest.CodeNotFound,
	})
}

func TestWriteError(t *testing.T) {
	c := qt.New(t)

	for _, test := range writeErrorTests {
		test := test
		c.Run(test.err.Error(), func(c *qt.C) {
			rec := httptest.NewRecorder()
			test.srv.WriteError(context.TODO(), rec, test.err)
			test.assertResponse(c, rec)
		})
	}
}

func TestHandleErrors(t *testing.T) {
	c := qt.New(t)

	req := new(http.Request)
	params := httprouter.Params{}
	// Test when handler returns an error.
	handler := testServer.HandleErrors(func(p httprequest.Params) error {
		c.Assert(p.Request, requestEquals, req)
		c.Assert(p.PathVar, qt.DeepEquals, params)
		c.Assert(p.PathPattern, qt.Equals, "")
		ctx := p.Context
		c.Assert(ctx, qt.Not(qt.IsNil))
		return errUnauth
	})
	rec := httptest.NewRecorder()
	handler(rec, req, params)
	c.Assert(rec.Code, qt.Equals, http.StatusUnauthorized)
	resp := parseErrorResponse(c, rec.Body.Bytes())
	c.Assert(resp, qt.DeepEquals, &httprequest.RemoteError{
		Message: errUnauth.Error(),
		Code:    "unauthorized",
	})

	// Test when handler returns nil.
	handler = testServer.HandleErrors(func(p httprequest.Params) error {
		c.Assert(p.Request, requestEquals, req)
		c.Assert(p.PathVar, qt.DeepEquals, params)
		c.Assert(p.PathPattern, qt.Equals, "")
		ctx := p.Context
		c.Assert(ctx, qt.Not(qt.IsNil))
		p.Response.WriteHeader(http.StatusCreated)
		p.Response.Write([]byte("something"))
		return nil
	})
	rec = httptest.NewRecorder()
	handler(rec, req, params)
	c.Assert(rec.Code, qt.Equals, http.StatusCreated)
	c.Assert(rec.Body.String(), qt.Equals, "something")
}

var handleErrorsWithErrorAfterWriteHeaderTests = []struct {
	about            string
	causeWriteHeader func(w http.ResponseWriter)
}{{
	about: "write",
	causeWriteHeader: func(w http.ResponseWriter) {
		w.Write([]byte(""))
	},
}, {
	about: "write header",
	causeWriteHeader: func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusOK)
	},
}, {
	about: "flush",
	causeWriteHeader: func(w http.ResponseWriter) {
		w.(http.Flusher).Flush()
	},
}}

func TestHandleErrorsWithErrorAfterWriteHeader(t *testing.T) {
	c := qt.New(t)

	for i, test := range handleErrorsWithErrorAfterWriteHeaderTests {
		c.Logf("test %d: %s", i, test.about)
		handler := testServer.HandleErrors(func(p httprequest.Params) error {
			test.causeWriteHeader(p.Response)
			return errgo.New("unexpected")
		})
		rec := httptest.NewRecorder()
		handler(rec, new(http.Request), nil)
		c.Assert(rec.Code, qt.Equals, http.StatusOK)
		c.Assert(rec.Body.String(), qt.Equals, "")
	}
}

func TestHandleJSON(t *testing.T) {
	c := qt.New(t)

	req := new(http.Request)
	params := httprouter.Params{}
	// Test when handler returns an error.
	handler := testServer.HandleJSON(func(p httprequest.Params) (interface{}, error) {
		c.Assert(p.Request, requestEquals, req)
		c.Assert(p.PathVar, qt.DeepEquals, params)
		c.Assert(p.PathPattern, qt.Equals, "")
		return nil, errUnauth
	})
	rec := httptest.NewRecorder()
	handler(rec, new(http.Request), params)
	resp := parseErrorResponse(c, rec.Body.Bytes())
	c.Assert(resp, qt.DeepEquals, &httprequest.RemoteError{
		Message: errUnauth.Error(),
		Code:    "unauthorized",
	})
	c.Assert(rec.Code, qt.Equals, http.StatusUnauthorized)

	// Test when handler returns a body.
	handler = testServer.HandleJSON(func(p httprequest.Params) (interface{}, error) {
		c.Assert(p.Request, requestEquals, req)
		c.Assert(p.PathVar, qt.DeepEquals, params)
		c.Assert(p.PathPattern, qt.Equals, "")
		p.Response.Header().Set("Some-Header", "value")
		return "something", nil
	})
	rec = httptest.NewRecorder()
	handler(rec, req, params)
	c.Assert(rec.Code, qt.Equals, http.StatusOK)
	c.Assert(rec.Body.String(), qt.Equals, `"something"`)
	c.Assert(rec.Header().Get("Some-Header"), qt.Equals, "value")
}

var requestEquals = qt.CmpEquals(cmpopts.IgnoreUnexported(http.Request{}))

type handlersWithRequestMethod struct{}

type x1Request struct {
	httprequest.Route `httprequest:"GET /x1/:p"`
	P                 string `httprequest:"p,path"`
}

func (r *x1Request) Test() string {
	return "test " + r.P
}

func (h *handlersWithRequestMethod) X1(arg *x1Request) (string, error) {
	return arg.P, nil
}

func assertErrorResponse(code int, body interface{}, header http.Header) func(c *qt.C, rec *httptest.ResponseRecorder) {
	return func(c *qt.C, rec *httptest.ResponseRecorder) {
		resp := reflect.New(reflect.ValueOf(body).Type())
		err := json.Unmarshal(rec.Body.Bytes(), resp.Interface())
		c.Assert(err, qt.Equals, nil)
		c.Assert(resp.Elem().Interface(), qt.DeepEquals, body)
		c.Assert(rec.Code, qt.Equals, code)
		for name, vals := range header {
			c.Assert(rec.HeaderMap[name], qt.DeepEquals, vals)
		}
	}
}

func parseErrorResponse(c *qt.C, body []byte) *httprequest.RemoteError {
	var errResp *httprequest.RemoteError
	err := json.Unmarshal(body, &errResp)
	c.Assert(err, qt.Equals, nil)
	return errResp
}
