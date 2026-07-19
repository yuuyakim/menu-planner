-- お気に入り（spec.md 4.2）。ユーザーが献立をブックマークする。
-- 履歴と違い自動削除はされない（spec.md 2.6）。
CREATE TABLE favorites (
    id         uuid        PRIMARY KEY,
    -- ユーザーを消したらお気に入りも消す。
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- 参照先の献立。献立マスタは固定なので削除は想定せず CASCADE は付けない。
    menu_id    uuid        NOT NULL REFERENCES menus (id),
    created_at timestamptz NOT NULL DEFAULT now(),

    -- 同じ献立を二重にお気に入り登録できない。重複追加は 409 にする（7-B）。
    CONSTRAINT favorites_user_menu_uniq UNIQUE (user_id, menu_id)
);

-- 一覧表示（新しい順）の主経路。
CREATE INDEX favorites_user_created_idx
    ON favorites (user_id, created_at DESC);
