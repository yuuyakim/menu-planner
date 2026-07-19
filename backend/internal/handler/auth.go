package handler

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// UserSignUpper はサインアップを抽象化する。実装は service.AuthService。
// handler が必要とする操作だけを宣言し、テストで差し替えられるようにする。
type UserSignUpper interface {
	SignUp(ctx context.Context, email, password string) (domain.User, error)
}

// AuthHandler は認証APIの受け口。
type AuthHandler struct {
	svc UserSignUpper
}

// NewAuthHandler は AuthHandler を生成する。
func NewAuthHandler(s UserSignUpper) *AuthHandler {
	return &AuthHandler{svc: s}
}

// RegisterRoutes は認証APIのルーティングを登録する。
func (h *AuthHandler) RegisterRoutes(e *echo.Echo) {
	g := e.Group(APIBasePath)
	g.POST("/auth/signup", h.SignUp)
}

// signupRequest は POST /auth/signup のリクエスト（spec.md 5.2）。
// 表示名は受け取らない。メールのローカル部から導出する。
type signupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// userDTO はユーザーのAPI表現。パスワードやハッシュは決して含めない。
type userDTO struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

// userResponse はユーザー1件を返すエンドポイントの共通レスポンス。
type userResponse struct {
	User userDTO `json:"user"`
}

// SignUp はメールとパスワードで新規ユーザーを登録する。
//
//	POST /api/v1/auth/signup
//
// 成功時は 201。Cookie やトークンの発行はこの段階では行わない（5-E / 5-F）。
func (h *AuthHandler) SignUp(c echo.Context) error {
	var req signupRequest
	if err := c.Bind(&req); err != nil {
		return err
	}

	user, err := h.svc.SignUp(c.Request().Context(), req.Email, req.Password)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, userResponse{User: toUserDTO(user)})
}

func toUserDTO(u domain.User) userDTO {
	return userDTO{
		ID:          u.ID.String(),
		Email:       u.Email.String(),
		DisplayName: u.DisplayName,
	}
}
