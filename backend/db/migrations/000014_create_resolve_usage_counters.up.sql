-- 「読み取る」（LLM 呼び出し）の日次カウンタ（設計 6章）。
--
-- メモリではなくDBに持つのは本番構成のため。Cloud Run は min-instances=0 で
-- アイドルのたびにプロセスが落ち、max-instances=2 で2つ並ぶ。メモリ上の
-- カウンタは日次上限として当てにならず、全体上限は原理的に守れない。
--
-- **数えるのは LLM 呼び出しが発生したときだけ。** 完全一致やキャッシュで
-- 解けたリクエストは料金が発生しないので枠を消さない（設計 4章）。
CREATE TABLE resolve_usage_counters (
    -- JST の日付。UTC で持つと日本の深夜に枠がリセットされる（設計 6.1）。
    usage_date date NOT NULL,
    -- 'ip'（非ログイン）/ 'user'（ログイン）/ 'total'（サービス全体）。
    scope      text NOT NULL,
    -- IPのHMAC / ユーザーID。total は持たない。
    -- **生のIPは入れない。** 元に戻せない値にすることで、
    -- プライバシーポリシーの改定なしに数を数えられる（設計 6.2）。
    subject    text NOT NULL,
    count      int  NOT NULL DEFAULT 0,

    PRIMARY KEY (usage_date, scope, subject),
    CONSTRAINT resolve_usage_counters_scope_valid
        CHECK (scope IN ('ip', 'user', 'total')),
    -- total は subject を持たない。他は必ず持つ。
    -- 空の subject を 'ip' で作れてしまうと、全非ログインが1行に集約されて
    -- 上限が誰にも当たらなくなる。
    CONSTRAINT resolve_usage_counters_subject_matches_scope
        CHECK ((scope = 'total' AND subject = '')
            OR (scope <> 'total' AND btrim(subject) <> ''))
);
