package service_test

import (
	"context"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeUserRepository はDBを使わずに UserRepository を代用する。
// 保存された内容を検査でき、任意のエラーを返せる。
type fakeUserRepository struct {
	// saved は CreateWithPassword に渡された最後の内容。
	savedUser domain.User
	savedHash string
	calls     int

	// takenEmails は登録済みとして扱うメール。ErrEmailTaken を返させる。
	takenEmails map[string]bool
	// credentials はメール(正規化済み)ごとのパスワード認証。ログイン照合に使う。
	credentials map[string]service.PasswordCredential
	// err は非nilなら CreateWithPassword がそのまま返すエラー。
	err error

	// Google upsert に渡された最後の値。
	lastGoogleSub   string
	lastGoogleEmail string
	lastGoogleName  string
	googleCalls     int
}

func newFakeUserRepository() *fakeUserRepository {
	return &fakeUserRepository{
		takenEmails: map[string]bool{},
		credentials: map[string]service.PasswordCredential{},
	}
}

func (r *fakeUserRepository) FindPasswordCredential(_ context.Context, email domain.Email) (service.PasswordCredential, error) {
	cred, ok := r.credentials[email.String()]
	if !ok {
		return service.PasswordCredential{}, service.ErrCredentialNotFound
	}
	return cred, nil
}

func (r *fakeUserRepository) FindOrCreateGoogleUser(_ context.Context, sub string, email domain.Email, displayName string) (domain.User, error) {
	r.googleCalls++
	r.lastGoogleSub = sub
	r.lastGoogleEmail = email.String()
	r.lastGoogleName = displayName
	u, err := domain.NewUser(email)
	if err != nil {
		return domain.User{}, err
	}
	return u, nil
}

func (r *fakeUserRepository) FindByID(_ context.Context, id domain.UserID) (domain.User, error) {
	// seedCredential で入れたユーザーを ID で引けるようにする。
	for _, cred := range r.credentials {
		if cred.User.ID.String() == id.String() {
			return cred.User, nil
		}
	}
	return domain.User{}, service.ErrUserNotFound
}

func (r *fakeUserRepository) FindByEmail(_ context.Context, email domain.Email) (domain.User, error) {
	cred, ok := r.credentials[email.String()]
	if !ok {
		return domain.User{}, service.ErrUserNotFound
	}
	return cred.User, nil
}

func (r *fakeUserRepository) CreateWithPassword(_ context.Context, u domain.User, hash string) error {
	r.calls++
	if r.err != nil {
		return r.err
	}
	if r.takenEmails[u.Email.String()] {
		return service.ErrEmailTaken
	}
	r.savedUser = u
	r.savedHash = hash
	r.takenEmails[u.Email.String()] = true
	return nil
}
