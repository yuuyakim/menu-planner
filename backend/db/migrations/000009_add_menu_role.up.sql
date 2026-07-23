-- 献立の役割（spec.md 2.10）。主菜 / 副菜 / 汁物 を分ける。
--
-- difficulty が「手間」を表すのに対し、role は「単品で夕食が成立するか」を分ける。
-- 晩ごはんを決めようとしてカプレーゼやコーンスープが単品で出ると
-- それだけでは夕食にならず必ず引き直しになる、という引き直しコストの解消が目的。
--
-- **既存行は DEFAULT で 'main' が入る。** 360件のうち副菜・汁物にあたる約40件は
-- シード（db/seeds/menus.sql）側で 'side' / 'soup' に付け替える。
-- 逆にしない（既存を side にしてから主菜を付け替える）のは、
-- 主菜が圧倒的多数のため、DEFAULT が主菜のほうが付け替えが少なく済むから。
ALTER TABLE menus
    ADD COLUMN role text NOT NULL DEFAULT 'main';

ALTER TABLE menus
    ADD CONSTRAINT menus_role_check
        CHECK (role IN ('main', 'side', 'soup'));

-- 既定の絞り込みが role = 'main' になるため（spec.md 2.10）、
-- 検索の主経路が必ずこの列を通る。genre / difficulty との複合で引く。
CREATE INDEX menus_role_genre_difficulty_idx
    ON menus (role, genre, difficulty);
