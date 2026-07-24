-- 保存済みの週間献立に紐づく買い物リストの差分（設計 5.1）。
-- リスト本体は献立から毎回導出し、ここには導出結果からの「ズレ」だけを持つ。
-- 行が無いことは「献立由来のまま・未チェック」を意味する。
CREATE TABLE shopping_list_overrides (
    saved_weekly_menu_id uuid        NOT NULL REFERENCES saved_weekly_menus(id) ON DELETE CASCADE,
    name                 text        NOT NULL,
    category             text        NOT NULL,
    -- origin は 'derived' / 'manual'。CHECK ではなくアプリで検証する
    -- （既存 menus.role 等の流儀。将来の値を DDL 変更なしに受けられるように）。
    origin               text        NOT NULL,
    checked              boolean     NOT NULL DEFAULT false,
    hidden               boolean     NOT NULL DEFAULT false,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    -- 同じリストに同名の品目は作れない。献立由来と同名の手動品目が並ぶのを防ぐ。
    PRIMARY KEY (saved_weekly_menu_id, name)
);
