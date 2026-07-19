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
	// err は非nilなら CreateWithPassword がそのまま返すエラー。
	err error
}

func newFakeUserRepository() *fakeUserRepository {
	return &fakeUserRepository{takenEmails: map[string]bool{}}
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
