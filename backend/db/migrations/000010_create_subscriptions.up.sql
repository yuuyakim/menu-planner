-- 有料プランの加入（設計 4.1）。free はレコードを持たない。
-- 行が無いこと自体が free を意味するため、「無料の加入」を作る責務が生じない。
CREATE TABLE subscriptions (
    -- 1利用者につき高々1件。複数同時加入は仕様にないため主キーで固定する。
    user_id                  uuid        PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    plan                     text        NOT NULL,
    -- active / past_due / canceled。CHECK ではなくアプリ側で検証する
    -- （決済事業者ごとに増える値を DDL の変更なしに受けられるようにするため）。
    status                   text        NOT NULL,
    current_period_end       timestamptz NOT NULL,
    -- 解約予約。利用者都合の解約は即時失効させず期末まで使えるようにする。
    -- 書き込む経路は決済フェーズで作るため、今は既定値のまま使われる。
    cancel_at_period_end     boolean     NOT NULL DEFAULT false,
    -- 加入を作った経路。現在は 'manual'（運用者の手動付与）のみ。
    provider                 text        NOT NULL,
    -- 決済事業者側の加入ID。手動付与では NULL。
    provider_subscription_id text,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now()
);

-- 決済事業者の加入IDは高々1行にしか対応しない。将来 Webhook が同じイベントを
-- 二度配送しても、DBが二重適用を弾く。今は張るコストがゼロだが、後から張ると
-- 既存データの重複を掃除する必要が出るため先に入れる。
--
-- 手動付与は provider_subscription_id が NULL になる。NULL 同士は一意制約で
-- 重複と見なされないが、意図を明示するため部分索引にする。
CREATE UNIQUE INDEX subscriptions_provider_subscription_id_key
    ON subscriptions (provider_subscription_id)
    WHERE provider_subscription_id IS NOT NULL;
