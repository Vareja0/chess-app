package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/vareja0/go-jwt/controllers"
	"github.com/vareja0/go-jwt/initializers"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	dialector := postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true})
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	initializers.DB = db
	t.Cleanup(func() { sqlDB.Close() })
	return mock
}

func newRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.LoadHTMLGlob("../views/*")
	return r
}

func jsonBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return bytes.NewBuffer(b)
}

// ── Signup ────────────────────────────────────────────────────────────────────

func TestSignup_InvalidBody(t *testing.T) {
	setupDB(t)
	setupRedis(t)

	r := newRouter()
	r.POST("/signup", controllers.Signup)

	req := httptest.NewRequest(http.MethodPost, "/signup", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSignup_DBError(t *testing.T) {
	mock := setupDB(t)
	setupRedis(t)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "users"`)).
		WillReturnError(gorm.ErrInvalidDB)
	mock.ExpectRollback()

	r := newRouter()
	r.POST("/signup", controllers.Signup)

	body := jsonBody(t, map[string]string{"Email": "test@test.com", "Password": "pass123"})
	req := httptest.NewRequest(http.MethodPost, "/signup", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSignup_Success(t *testing.T) {
	mock := setupDB(t)
	setupRedis(t)
	os.Setenv("SECRET_KEY", "test-secret")
	os.Setenv("REFRESH_SECRET_KEY", "test-refresh-secret")

	// INSERT user
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "users"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	// issueTokens: soft-delete old session + create new
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "sessions"`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "sessions"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	r := newRouter()
	r.POST("/signup", controllers.Signup)

	body := jsonBody(t, map[string]string{"username": "testuser", "email": "new@test.com", "password": "secure123"})
	req := httptest.NewRequest(http.MethodPost, "/signup", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d — body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	cookies := w.Result().Cookies()
	cookieMap := make(map[string]string)
	for _, c := range cookies {
		cookieMap[c.Name] = c.Value
	}
	if cookieMap["Authorization"] == "" {
		t.Error("Authorization cookie not set after signup")
	}
	if cookieMap["RefreshToken"] == "" {
		t.Error("RefreshToken cookie not set after signup")
	}
}

// ── Login ─────────────────────────────────────────────────────────────────────

func TestLogin_InvalidBody(t *testing.T) {
	setupDB(t)
	setupRedis(t)

	r := newRouter()
	r.POST("/login", controllers.Login)

	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	mock := setupDB(t)
	setupRedis(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password"}))

	r := newRouter()
	r.POST("/login", controllers.Login)

	body := jsonBody(t, map[string]string{"Email": "ghost@test.com", "Password": "pass"})
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	mock := setupDB(t)
	setupRedis(t)

	hash, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password", "created_at", "updated_at", "deleted_at"}).
			AddRow(1, "user@test.com", string(hash), time.Now(), time.Now(), nil))

	r := newRouter()
	r.POST("/login", controllers.Login)

	body := jsonBody(t, map[string]string{"Email": "user@test.com", "Password": "wrong-password"})
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestLogin_Success(t *testing.T) {
	mock := setupDB(t)
	setupRedis(t)
	os.Setenv("SECRET_KEY", "test-secret")
	os.Setenv("REFRESH_SECRET_KEY", "test-refresh-secret")

	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password", "created_at", "updated_at", "deleted_at"}).
			AddRow(1, "user@test.com", string(hash), time.Now(), time.Now(), nil))

	// soft-delete old session + create new
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "sessions"`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "sessions"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	r := newRouter()
	r.POST("/login", controllers.Login)

	body := jsonBody(t, map[string]string{"Email": "user@test.com", "Password": "password123"})
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d — body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	cookies := w.Result().Cookies()
	cookieMap := make(map[string]string)
	for _, c := range cookies {
		cookieMap[c.Name] = c.Value
	}
	if cookieMap["Authorization"] == "" {
		t.Error("Authorization cookie not set")
	}
	if cookieMap["RefreshToken"] == "" {
		t.Error("RefreshToken cookie not set")
	}
}
