package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/JunLang-7/sduzg-alumin-platform/server/internal/common"
	"github.com/JunLang-7/sduzg-alumin-platform/server/internal/model"
	"github.com/JunLang-7/sduzg-alumin-platform/server/internal/response"
	"github.com/gin-gonic/gin"
)

const currentUserKey = "current_user"

type UserFinder interface {
	FindByID(ctx context.Context, id uint64) (*model.User, error)
}

// LoadCurrentUser 在认证后为每次请求读取数据库中的账号状态和角色。
// JWT 仅用于确认身份，不使用客户端提交或缓存的角色作为授权依据。
func LoadCurrentUser(users UserFinder, whitelist ...string) gin.HandlerFunc {
	whiteSet := make(map[string]struct{}, len(whitelist))
	for _, path := range whitelist {
		whiteSet[path] = struct{}{}
	}

	return func(c *gin.Context) {
		if _, ok := whiteSet[c.FullPath()]; ok {
			c.Next()
			return
		}

		userID, ok := CurrentUserID(c)
		if !ok || userID == 0 {
			response.Fail(c, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized")
			c.Abort()
			return
		}
		if users == nil {
			response.Fail(c, http.StatusServiceUnavailable, response.CodeServiceUnavailable, "database is unavailable")
			c.Abort()
			return
		}

		user, err := users.FindByID(c.Request.Context(), userID)
		switch {
		case errors.Is(err, common.ErrDatabaseUnavailable):
			response.Fail(c, http.StatusServiceUnavailable, response.CodeServiceUnavailable, "database is unavailable")
		case errors.Is(err, common.ErrUserNotFound):
			response.Fail(c, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized")
		case err != nil:
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "internal server error")
		case user == nil || user.ID != userID || user.DeletedAt.Valid:
			response.Fail(c, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized")
		case user.Status != common.UserStatusActive:
			response.Fail(c, http.StatusForbidden, response.CodeForbidden, "account is disabled")
		case user.Role != common.RoleAlumni && user.Role != common.RoleAdmin && user.Role != common.RoleSuperAdmin:
			response.Fail(c, http.StatusForbidden, response.CodeForbidden, "permission denied")
		default:
			c.Set(currentUserKey, user)
			c.Next()
			return
		}
		c.Abort()
	}
}

// CurrentUser 返回当前请求从数据库加载的用户，仅在服务端请求内复用。
func CurrentUser(c *gin.Context) (*model.User, bool) {
	value, exists := c.Get(currentUserKey)
	user, ok := value.(*model.User)
	return user, exists && ok && user != nil
}

// RequireRoles 根据当前请求的数据库角色拒绝越权访问。
func RequireRoles(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(c *gin.Context) {
		user, ok := CurrentUser(c)
		if !ok {
			response.Fail(c, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized")
			c.Abort()
			return
		}
		if _, ok := allowed[user.Role]; !ok {
			response.Fail(c, http.StatusForbidden, response.CodeForbidden, "permission denied")
			c.Abort()
			return
		}

		c.Next()
	}
}
