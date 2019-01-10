// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package httprequest_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/julienschmidt/httprouter"

	"gopkg.in/httprequest.v1"
)

type testRequest struct {
	httprequest.Route `httprequest:"GET /foo"`
}

func TestContextCancelledWhenDone(t *testing.T) {
	c := qt.New(t)

	var ch <-chan struct{}
	hnd := testServer.Handle(func(p httprequest.Params, req *testRequest) {
		ch = p.Context.Done()
	})
	router := httprouter.New()
	router.Handle(hnd.Method, hnd.Path, hnd.Handle)
	srv := httptest.NewServer(router)
	_, err := http.Get(srv.URL + "/foo")
	c.Assert(err, qt.Equals, nil)
	select {
	case <-ch:
	default:
		c.Fatal("context not canceled at end of handler.")
	}
}
