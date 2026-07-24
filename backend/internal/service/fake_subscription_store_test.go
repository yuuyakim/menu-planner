package service_test

import (
	"context"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeSubscriptionStore は加入の保存をメモリ上で模す。
type fakeSubscriptionStore struct {
	subs map[string]domain.Subscription
	// findErr が非nilなら Find がそれを返す。障害時の振る舞いを見るため。
	findErr error
}

func newFakeSubscriptionStore() *fakeSubscriptionStore {
	return &fakeSubscriptionStore{subs: map[string]domain.Subscription{}}
}

func (f *fakeSubscriptionStore) Find(
	_ context.Context, userID domain.UserID,
) (domain.Subscription, error) {
	if f.findErr != nil {
		return domain.Subscription{}, f.findErr
	}
	sub, ok := f.subs[userID.String()]
	if !ok {
		return domain.Subscription{}, service.ErrSubscriptionNotFound
	}
	return sub, nil
}

func (f *fakeSubscriptionStore) Upsert(_ context.Context, sub domain.Subscription) error {
	f.subs[sub.UserID.String()] = sub
	return nil
}
