package router

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/JunLang-7/sduzg-alumin-platform/server/internal/common"
	"github.com/JunLang-7/sduzg-alumin-platform/server/internal/config"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func authorizationRouter(t *testing.T) (*gin.Engine, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		db.Close()
	})
	gormDB, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	engine := New(Dependencies{
		Config: config.Config{
			App:  config.AppConfig{Name: "test-api", Env: config.EnvDevelopment},
			Auth: config.AuthConfig{JWTSecret: "test-secret", AccessTokenTTL: time.Hour},
		},
		DB:     gormDB,
		Logger: zap.NewNop(),
	})
	return engine, mock
}

func expectDatabaseUser(mock sqlmock.Sqlmock, role, status string) {
	mock.ExpectQuery("SELECT .* FROM `users`").WithArgs(uint64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "account", "password_hash", "role", "status"}).
			AddRow(1, "test-user", "never-return-this-hash", role, status))
}

func TestManagementRoutesRejectSpoofedClientRole(t *testing.T) {
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/admin/alumni"},
		{http.MethodPut, "/api/v1/admin/alumni/2"},
		{http.MethodDelete, "/api/v1/admin/alumni/2"},
		{http.MethodGet, "/api/v1/admin/dashboard/overview"},
		{http.MethodGet, "/api/v1/admin/dashboard/distribution"},
		{http.MethodGet, "/api/v1/super-admin/admins"},
		{http.MethodPost, "/api/v1/super-admin/admins"},
		{http.MethodDelete, "/api/v1/super-admin/admins/2"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			engine, mock := authorizationRouter(t)
			expectDatabaseUser(mock, common.RoleAlumni, common.UserStatusActive)
			// Even a correctly signed legacy token claiming super_admin cannot override the DB.
			token := testAccessToken(t, "test-secret", time.Now().Add(time.Hour), common.RoleSuperAdmin)
			req := httptest.NewRequest(route.method, route.path+"?role=super_admin", strings.NewReader(`{"role":"super_admin","user_id":2}`))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Role", "super_admin")
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":40300`) {
				t.Fatalf("expected 403 before business query/write, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDatabaseRoleChangesApplyWithoutNewToken(t *testing.T) {
	engine, mock := authorizationRouter(t)
	token := testAccessToken(t, "test-secret", time.Now().Add(time.Hour), common.RoleSuperAdmin)
	for _, role := range []string{common.RoleSuperAdmin, common.RoleAdmin, common.RoleAlumni} {
		expectDatabaseUser(mock, role, common.UserStatusActive)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"role":"`+role+`"`) {
			t.Fatalf("expected current database role %s, got %d: %s", role, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") != "no-store" || strings.Contains(rec.Body.String(), "never-return-this-hash") || strings.Contains(rec.Body.String(), "password_hash") {
			t.Fatalf("current user must be non-cacheable and exclude password hash: %s", rec.Body.String())
		}
	}
	// The same token is denied at super-admin routes after the DB demotion.
	expectDatabaseUser(mock, common.RoleAdmin, common.UserStatusActive)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/super-admin/admins", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("demoted admin status=%d, want 403", rec.Code)
	}
}

func TestEveryPrivateRouteChecksCurrentDatabaseUser(t *testing.T) {
	engine, mock := authorizationRouter(t)
	for _, route := range engine.Routes() {
		if route.Path == "/api/v1/health/live" || route.Path == "/api/v1/health/ready" || route.Path == "/api/v1/auth/login" {
			continue
		}
		t.Run(route.Method+" "+route.Path, func(t *testing.T) {
			mock.ExpectQuery("SELECT .* FROM `users`").WithArgs(uint64(1), 1).
				WillReturnRows(sqlmock.NewRows([]string{"id"}))
			path := strings.ReplaceAll(route.Path, ":id", "2")
			req := httptest.NewRequest(route.Method, path, nil)
			req.Header.Set("Authorization", "Bearer "+testAccessToken(t, "test-secret", time.Now().Add(time.Hour)))
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("missing database user status=%d, want 401: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAuthLogoutRouteWithActiveDatabaseUser(t *testing.T) {
	engine, mock := authorizationRouter(t)
	expectDatabaseUser(mock, common.RoleAlumni, common.UserStatusActive)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+testAccessToken(t, "test-secret", time.Now().Add(time.Hour)))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"logged_out":true`) {
		t.Fatalf("expected successful logout, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCurrentUserRouteRejectsInvalidAccountsAndTokens(t *testing.T) {
	for _, tt := range []struct {
		name, role, status string
		want               int
	}{
		{"disabled", common.RoleSuperAdmin, "disabled", http.StatusForbidden},
		{"deleted", common.RoleSuperAdmin, common.UserStatusDeleted, http.StatusForbidden},
		{"unknown role", "owner", common.UserStatusActive, http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			engine, mock := authorizationRouter(t)
			expectDatabaseUser(mock, tt.role, tt.status)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
			req.Header.Set("Authorization", "Bearer "+testAccessToken(t, "test-secret", time.Now().Add(time.Hour)))
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status=%d, want %d", rec.Code, tt.want)
			}
		})
	}
	for _, token := range []string{"", testAccessToken(t, "wrong-secret", time.Now().Add(time.Hour)), testAccessToken(t, "test-secret", time.Now().Add(-time.Hour))} {
		engine, _ := authorizationRouter(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("invalid token status=%d, want 401", rec.Code)
		}
	}
	engine, mock := authorizationRouter(t)
	mock.ExpectQuery("SELECT .* FROM `users`").WithArgs(uint64(1), 1).WillReturnError(errors.New("database read failed"))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+testAccessToken(t, "test-secret", time.Now().Add(time.Hour)))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("database error status=%d, want 500", rec.Code)
	}
}
