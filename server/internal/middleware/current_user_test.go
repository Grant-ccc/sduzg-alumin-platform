package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JunLang-7/sduzg-alumin-platform/server/internal/common"
	"github.com/JunLang-7/sduzg-alumin-platform/server/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type changingUserStore struct {
	user  *model.User
	calls int
}

func (s *changingUserStore) FindByID(context.Context, uint64) (*model.User, error) {
	s.calls++
	return s.user, nil
}

func TestLoadCurrentUserFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		store  UserFinder
		userID uint64
		want   int
	}{
		{"missing identity", fakeRoleUserStore{}, 0, http.StatusUnauthorized},
		{"missing repository", nil, 1, http.StatusServiceUnavailable},
		{"missing user", fakeRoleUserStore{err: common.ErrUserNotFound}, 1, http.StatusUnauthorized},
		{"nil user", fakeRoleUserStore{}, 1, http.StatusUnauthorized},
		{"wrong user", fakeRoleUserStore{user: &model.User{ID: 2}}, 1, http.StatusUnauthorized},
		{"disabled", fakeRoleUserStore{user: &model.User{ID: 1, Role: common.RoleAdmin, Status: "disabled"}}, 1, http.StatusForbidden},
		{"deleted status", fakeRoleUserStore{user: &model.User{ID: 1, Role: common.RoleAdmin, Status: common.UserStatusDeleted}}, 1, http.StatusForbidden},
		{"soft deleted", fakeRoleUserStore{user: &model.User{ID: 1, Role: common.RoleAdmin, Status: common.UserStatusActive, DeletedAt: gorm.DeletedAt{Valid: true}}}, 1, http.StatusUnauthorized},
		{"unknown role", fakeRoleUserStore{user: &model.User{ID: 1, Role: "owner", Status: common.UserStatusActive}}, 1, http.StatusForbidden},
		{"database unavailable", fakeRoleUserStore{err: common.ErrDatabaseUnavailable}, 1, http.StatusServiceUnavailable},
		{"database error", fakeRoleUserStore{err: errors.New("query failed")}, 1, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(func(c *gin.Context) { c.Set(CurrentUserIDKey, tt.userID) })
			engine.Use(LoadCurrentUser(tt.store))
			engine.GET("/private", func(c *gin.Context) { t.Error("denied request reached handler") })
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/private", nil))
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestRequireRolesUsesFreshDatabaseRoleOncePerRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &changingUserStore{user: &model.User{ID: 1, Role: common.RoleSuperAdmin, Status: common.UserStatusActive}}
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set(CurrentUserIDKey, uint64(1)) })
	engine.Use(LoadCurrentUser(store))
	engine.GET("/admin", RequireRoles(common.RoleAdmin, common.RoleSuperAdmin), func(c *gin.Context) {
		first, ok := CurrentUser(c)
		second, otherOK := CurrentUser(c)
		if !ok || !otherOK || first != second {
			t.Error("expected request-local database user")
		}
		c.Status(http.StatusNoContent)
	})
	engine.GET("/super-admin", RequireRoles(common.RoleSuperAdmin), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for i, tt := range []struct {
		role, path string
		want       int
	}{
		{common.RoleSuperAdmin, "/super-admin", http.StatusNoContent},
		{common.RoleAdmin, "/super-admin", http.StatusForbidden},
		{common.RoleAdmin, "/admin", http.StatusNoContent},
		{common.RoleAlumni, "/admin", http.StatusForbidden},
	} {
		store.user.Role = tt.role
		req := httptest.NewRequest(http.MethodGet, tt.path+"?role=super_admin", nil)
		req.Header.Set("X-Role", "super_admin")
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		if rec.Code != tt.want || store.calls != i+1 {
			t.Fatalf("role=%s status=%d calls=%d, want status=%d calls=%d", tt.role, rec.Code, store.calls, tt.want, i+1)
		}
	}
}

func TestRequireRolesRejectsMissingDatabaseUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/admin", RequireRoles(common.RoleAdmin), func(c *gin.Context) { t.Error("missing user reached handler") })
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rec.Code)
	}
}
