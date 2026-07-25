-- 決済事業者側の顧客ID。顧客の再利用と将来の解約/ポータル画面で使う。
-- 手動付与（provider='manual'）では NULL のまま。
ALTER TABLE subscriptions ADD COLUMN provider_customer_id text;
