-- 認証の基盤（spec.md 4.2）。1人のユーザーに対しパスワードと Google の
-- 2方式を共存させるため、認証手段は users から分けて auth_identities に持つ。
CREATE TABLE users (
    id           uuid        PRIMARY KEY,
    email        text        NOT NULL UNIQUE,
    display_name text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT users_email_not_blank
        CHECK (btrim(email) <> ''),
    CONSTRAINT users_display_name_not_blank
        CHECK (btrim(display_name) <> '')
);

-- 認証手段。同一メールに対しパスワードと Google の共存を許すため、
-- 1ユーザーが複数の identity を持てる。
CREATE TABLE auth_identities (
    id            uuid        PRIMARY KEY,
    -- ユーザーを消したら認証手段も一緒に消す。認証手段だけ残っても
    -- 参照先が無く意味を持たないため。
    user_id       uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- 認証方式。値は password / google の2つ。
    provider      text        NOT NULL,
    -- google の sub（アカウントの一意な識別子）。password では NULL。
    provider_uid  text,
    -- bcrypt ハッシュ。password のみ。google では NULL。
    password_hash text,
    created_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT auth_identities_provider_check
        CHECK (provider IN ('password', 'google')),
    -- 方式ごとに必須の項目が異なる。方式と中身の不整合をDBで防ぐ。
    -- password は hash 必須、google は uid 必須。
    CONSTRAINT auth_identities_password_needs_hash
        CHECK (provider <> 'password' OR password_hash IS NOT NULL),
    CONSTRAINT auth_identities_google_needs_uid
        CHECK (provider <> 'google' OR provider_uid IS NOT NULL)
);

-- 同じ (provider, provider_uid) が2つあると、どのユーザーの Google アカウント
-- なのか一意に定まらない。password は provider_uid が NULL で、NULL 同士は
-- 一意制約で重複と見なされないため、この索引はそのまま共存できる。
CREATE UNIQUE INDEX auth_identities_provider_uid_uniq
    ON auth_identities (provider, provider_uid);

-- ログイン時に user_id で identity を引くため。
CREATE INDEX auth_identities_user_id_idx
    ON auth_identities (user_id);
