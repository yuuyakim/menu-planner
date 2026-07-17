-- 外部検索APIの消費を抑えるためのキャッシュ（spec.md 4.2 / 13.2）。
-- 献立は120件固定で検索語は「{献立名} レシピ」の最大120通りしかないため、
-- ここに溜めればAPI消費は生涯およそ120クエリで頭打ちになる。
CREATE TABLE recipe_link_caches (
    -- 献立1件につき1エントリ。検索語が献立名から決まるため menu_id で一意になる。
    menu_id    uuid        PRIMARY KEY REFERENCES menus (id) ON DELETE CASCADE,
    -- RecipeLink の配列。件数が0〜3と少なく、この表以外から参照しないため
    -- 別表に正規化せず jsonb に持つ。
    links      jsonb       NOT NULL,
    -- TTL(7日)の判定に使う。既定値を置かないのは、書いた側が意図した時刻を
    -- 明示的に渡すため（テストで時刻を固定できる）。
    fetched_at timestamptz NOT NULL,

    -- 配列以外が入るとアプリ側の解釈が壊れる。型を表側でも縛る。
    CONSTRAINT recipe_link_caches_links_is_array
        CHECK (jsonb_typeof(links) = 'array')
);
