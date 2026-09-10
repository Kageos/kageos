package v1

import (
	"github.com/kageos/kageos/core/hr-server/service"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kageos/kageos/pkg/ginx/response"
	"github.com/kageos/kageos/pkg/openapitoken"
)

// OpenAPIToken exposes the HR-owned OpenAPI Token validation boundary.
type OpenAPIToken struct {
	store *openapitoken.Store
	users *service.UserService
}

func NewOpenAPIToken(store *openapitoken.Store, users *service.UserService) *OpenAPIToken {
	return &OpenAPIToken{store: store, users: users}
}

// Validate is called by the API gateway on cache misses.
func (h *OpenAPIToken) Validate(c *gin.Context) {
	rawToken := openapitoken.BearerToken(c.GetHeader("Authorization"))
	if strings.TrimSpace(rawToken) == "" {
		response.NoAuth(c, "未提供 OpenAPI Token")
		return
	}
	principal, err := h.store.Validate(rawToken, c.ClientIP(), c.GetHeader("User-Agent"))
	if err != nil {
		response.NoAuth(c, "OpenAPI Token 无效、已过期或已吊销")
		return
	}
	user, err := h.users.GetUserByUsername(principal.Username)
	if err != nil || !user.IsActive() || (principal.UserID != 0 && user.ID != principal.UserID) {
		response.NoAuth(c, "账户不存在或已停用")
		return
	}
	response.OkWithData(c, principal)
}
