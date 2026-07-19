-- FIFO のタイブレーク用に単調増加する列を足す（spec.md 2.5 / 4.3）。
-- searched_at は now()（トランザクション時刻）なので、週間献立の一括登録では
-- 7件が同じ値になる。searched_at だけで並べると同値の行の順序が定まらず、
-- 「最新15件」の判定が非決定的になる。挿入順で確実に並べるための seq を持つ。
ALTER TABLE search_histories
    ADD COLUMN seq bigint GENERATED ALWAYS AS IDENTITY;

-- FIFO判定と一覧表示は searched_at の降順、同値なら seq の降順で並べる。
-- 6-A の (user_id, searched_at DESC) を、タイブレークまで含む索引に置き換える。
DROP INDEX search_histories_user_searched_idx;
CREATE INDEX search_histories_user_order_idx
    ON search_histories (user_id, searched_at DESC, seq DESC);
