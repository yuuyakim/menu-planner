-- 食材マスタ（spec.md 4.2）。献立に必要な食材を持つ。
--
-- **調味料は登録しない**（spec.md 14.4）。醤油・塩・砂糖・油・だしなどは
-- 大半の家庭に常備されており、買い物リストに並べると本当に買うものが埋もれるため。
-- そのため category に seasoning は無く、常備品フラグのような列も持たない。
--
-- 分量も持たない（spec.md 14.2）。買い物リストは食材名のチェックリストとして提供する。
CREATE TABLE ingredients (
    id         uuid        PRIMARY KEY,
    name       text        NOT NULL,
    -- 一覧の並び順に使う。欠けると順序が崩れるため必須。
    name_kana  text        NOT NULL,
    category   text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT ingredients_category_check
        CHECK (category IN ('vegetable', 'meat', 'seafood', 'dairy_egg', 'staple', 'other')),
    CONSTRAINT ingredients_name_not_blank
        CHECK (btrim(name) <> ''),
    CONSTRAINT ingredients_name_kana_not_blank
        CHECK (btrim(name_kana) <> '')
);

-- シード投入を冪等にするため、同名の食材を重複させない。
CREATE UNIQUE INDEX ingredients_name_uniq ON ingredients (name);

-- 献立と食材の多対多（spec.md 4.2）。
CREATE TABLE menu_ingredients (
    -- 献立を消したら紐付けも消す。
    menu_id       uuid NOT NULL REFERENCES menus (id) ON DELETE CASCADE,
    -- どこかの献立が使っている食材は消せないようにする。
    -- 消せてしまうと、献立から食材が黙って欠落する。
    ingredient_id uuid NOT NULL REFERENCES ingredients (id) ON DELETE RESTRICT,

    PRIMARY KEY (menu_id, ingredient_id)
);

-- 「この食材を使う献立」を引く経路（買い物リストの usedIn）。
-- (menu_id, ingredient_id) の主キーでは ingredient_id 単独の絞り込みに効かないため別途張る。
CREATE INDEX menu_ingredients_ingredient_idx ON menu_ingredients (ingredient_id);
