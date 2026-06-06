package api

// This file exercises auth behavior so refactors preserve the documented contract.

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
)

func withTestOperator(req *http.Request) *http.Request {
	userID := uuid.New()
	return req.WithContext(withAuthenticatedUser(req.Context(), &service.AuthenticatedUser{
		User: domain.UserWithRolesAndPermissions{
			User: domain.User{
				ID:          userID,
				Username:    "test-operator",
				Email:       "test-operator@example.test",
				DisplayName: "Test Operator",
				Status:      domain.UserStatusActive,
			},
		},
		Session: service.AuthenticatedSession{
			ID:     uuid.New(),
			UserID: userID,
		},
	}))
}
