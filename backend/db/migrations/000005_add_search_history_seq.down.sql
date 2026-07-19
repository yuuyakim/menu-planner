DROP INDEX search_histories_user_order_idx;
CREATE INDEX search_histories_user_searched_idx
    ON search_histories (user_id, searched_at DESC);
ALTER TABLE search_histories DROP COLUMN seq;
