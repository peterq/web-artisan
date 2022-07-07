package http_server_util

import (
	"fmt"
	"github.com/pkg/errors"
	"net/http"
)

func HandleFuncWithError(fn func(writer http.ResponseWriter, request *http.Request) error) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var err error
		defer func() {
			if p := recover(); p != nil {
				if e, ok := p.(error); ok {
					err = e
				} else {
					err = errors.New(fmt.Sprintf("%#v", p))
				}
			}
			if err == nil {
				return
			}
			if respAble, ok := err.(interface {
				WriteHttpResponse(writer http.ResponseWriter, request *http.Request)
			}); ok {
				respAble.WriteHttpResponse(writer, request)
				return
			}

			var code = 502
			if codeAble, ok := err.(interface {
				HttpStatusCode() int
			}); ok {
				code = codeAble.HttpStatusCode()
			}
			writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			writer.WriteHeader(code)
			_, _ = writer.Write([]byte(err.Error()))
		}()
		err = fn(writer, request)
	}
}
