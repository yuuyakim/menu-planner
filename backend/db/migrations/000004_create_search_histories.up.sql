-- 検索履歴（spec.md 4.2）。提示された献立1件が1レコード。
-- ユーザーごとに最新15件を保持し、超過分は削除する（FIFO。実装は service 層・6-B）。
CREATE TABLE search_histories (
    id          uuid        PRIMARY KEY,
    -- ユーザーを消したら履歴も消す。
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- 参照先の献立。献立マスタは固定なので削除は想定せず CASCADE は付けない。
    menu_id     uuid        NOT NULL REFERENCES menus (id),
    -- 検索の種類。1食分(single)か週間(weekly)か。
    search_mode text        NOT NULL,
    searched_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT search_histories_mode_check
        CHECK (search_mode IN ('single', 'weekly'))
);

-- FIFO判定（最新15件の絞り込み）と一覧表示（新しい順）の主経路。
CREATE INDEX search_histories_user_searched_idx
    ON search_histories (user_id, searched_at DESC);
