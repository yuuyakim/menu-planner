-- 入力語から食材への解決キャッシュ（設計 5章）。
--
-- 「豚こま」→ 豚肉 のような表記揺れの対応づけを、一度 LLM に聞いたら保存しておく。
-- 同じ語を二度 LLM に問い合わせないための仕組みであり、運用が続くほど
-- LLM 呼び出しは逓減する。spec.md 2.9 が却下した「別名辞書」を、人手ではなく
-- LLM に育てさせる形にあたる。
--
-- **利用者に紐づかない。** 誰が書いた語であっても対応づけの結果は同じなので、
-- 未認証も含めて全員で共有する。
CREATE TABLE ingredient_resolutions (
    -- domain.NormalizeIngredientWord を通した後の語。
    -- 正規化前の語を入れると「タマネギ」と「たまねぎ」が別行になり、
    -- キャッシュが効かなくなる。
    input_word    text        PRIMARY KEY,
    -- **NULL は「マスタに無い」を意味する。**
    -- 未解決もキャッシュしないと、マスタに無い語だけが毎回 LLM を通ることになり、
    -- いちばん無駄な呼び出しが残り続ける。
    ingredient_id uuid        REFERENCES ingredients (id) ON DELETE CASCADE,
    resolved_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT ingredient_resolutions_input_word_not_blank
        CHECK (btrim(input_word) <> '')
);

-- 食材マスタを更新したとき、NULL 行だけを消す運用のための経路（設計 9章）。
-- 新しい食材が足されると、過去に NULL で保存した語が解決可能になる。
CREATE INDEX ingredient_resolutions_unresolved_idx
    ON ingredient_resolutions (input_word)
    WHERE ingredient_id IS NULL;
