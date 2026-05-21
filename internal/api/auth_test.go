package api

import (
	"net/http"

	"github.com/lollinoo/theia/internal/security"
)

func withTestOperator(req *http.Request) *http.Request {
	return req.WithContext(security.WithOperatorSubject(req.Context(), security.OperatorSubject{
		Name:          "test-operator",
		Authenticated: true,
	}))
}
