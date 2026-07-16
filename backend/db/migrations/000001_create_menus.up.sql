CREATE TABLE menus (
    id          uuid        PRIMARY KEY,
    name        text        NOT NULL,
    name_kana   text        NOT NULL,
    genre       text        NOT NULL,
    difficulty  text        NOT NULL,
    description text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT menus_genre_check
        CHECK (genre IN ('japanese', 'western', 'chinese', 'other')),
    CONSTRAINT menus_difficulty_check
        CHECK (difficulty IN ('easy', 'normal', 'elaborate')),
    CONSTRAINT menus_name_not_blank
        CHECK (btrim(name) <> ''),
    CONSTRAINT menus_name_kana_not_blank
        CHECK (btrim(name_kana) <> ''),
    CONSTRAINT menus_description_not_blank
        CHECK (btrim(description) <> '')
);

-- 献立検索の主経路。genre / difficulty のどちらか片方だけの指定でも
-- 先頭列が genre なので (genre) の絞り込みには効く。
CREATE INDEX menus_genre_difficulty_idx ON menus (genre, difficulty);

-- difficulty 単独指定のときに genre 側の索引が効かないため別途張る。
CREATE INDEX menus_difficulty_idx ON menus (difficulty);

-- シード投入を冪等にするため、同名の献立を重複させない。
CREATE UNIQUE INDEX menus_name_uniq ON menus (name);
