-- 索引と制約は列と一緒に落ちるが、明示して順序を読めるようにする。
DROP INDEX IF EXISTS menus_role_genre_difficulty_idx;

ALTER TABLE menus
    DROP CONSTRAINT IF EXISTS menus_role_check;

ALTER TABLE menus
    DROP COLUMN IF EXISTS role;
