-- 保存した週間献立（spec.md 2.8 / 4.2）。
--
-- 組み立てた1週間分をひとまとまりで取っておき、後から開けるようにする。
-- 買い物の場で見返すのが主な用途のため、sessionStorage ではなくサーバに持つ。
CREATE TABLE saved_weekly_menus (
    id         uuid        PRIMARY KEY,
    -- ユーザーを消したら保存した週も消す。
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- 一覧表示（新しい順）の主経路。上限10件の判定でも同じ索引を使う。
CREATE INDEX saved_weekly_menus_user_created_idx
    ON saved_weekly_menus (user_id, created_at DESC);

-- 保存した週の中身。
--
-- jsonb に7件まとめて持つ形も検討したが正規化した。menu_id に外部キーが効き、
-- 献立マスタとの不整合をDBが防げる。量も7行×10件と知れている。
CREATE TABLE saved_weekly_menu_days (
    saved_weekly_menu_id uuid     NOT NULL
        REFERENCES saved_weekly_menus (id) ON DELETE CASCADE,
    -- 起点からの通し番号（spec.md 13.3 の当日起点）。曜日は持たない。
    day                  smallint NOT NULL,
    -- 献立マスタは固定なので削除は想定せず CASCADE は付けない。
    menu_id              uuid     NOT NULL REFERENCES menus (id),

    -- 同じ週に同じ日が2件入らない。day の重複はこの主キーが防ぐ。
    PRIMARY KEY (saved_weekly_menu_id, day),

    CONSTRAINT saved_weekly_menu_days_day_range
        CHECK (day BETWEEN 1 AND 7)
);
