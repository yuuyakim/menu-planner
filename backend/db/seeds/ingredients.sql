-- 食材マスタ（spec.md 4.2 / 14章）。
--
-- **調味料は含めない**（14.4）。醤油・味噌・塩・砂糖・油・だし・粉類は登録しない。
-- 境界は「その献立のために買い足す必要があるか」で判断し、迷ったら登録しない側に倒す。
-- そのため、にんにく・生姜（野菜として買う）やカレールウ（その献立のために買う）は含み、
-- 小麦粉・片栗粉・パン粉は含まない。
--
-- 分量も持たない（14.2）。買い物リストは食材名のチェックリストとして提供する。
--
-- name の UNIQUE 制約と ON CONFLICT DO NOTHING により再実行しても重複しない（冪等）。

INSERT INTO ingredients (id, name, name_kana, category) VALUES
-- ============ 野菜 ============
(gen_random_uuid(), '玉ねぎ',       'たまねぎ',       'vegetable'),
(gen_random_uuid(), '生姜',         'しょうが',       'vegetable'),
(gen_random_uuid(), 'にんにく',     'にんにく',       'vegetable'),
(gen_random_uuid(), 'キャベツ',     'きゃべつ',       'vegetable'),
(gen_random_uuid(), '大根',         'だいこん',       'vegetable'),
(gen_random_uuid(), 'じゃがいも',   'じゃがいも',     'vegetable'),
(gen_random_uuid(), 'にんじん',     'にんじん',       'vegetable'),
(gen_random_uuid(), 'ねぎ',         'ねぎ',           'vegetable'),
(gen_random_uuid(), 'ごぼう',       'ごぼう',         'vegetable'),
(gen_random_uuid(), 'れんこん',     'れんこん',       'vegetable'),
(gen_random_uuid(), '椎茸',         'しいたけ',       'vegetable'),
(gen_random_uuid(), '干し椎茸',     'ほししいたけ',   'vegetable'),
(gen_random_uuid(), 'しめじ',       'しめじ',         'vegetable'),
(gen_random_uuid(), 'えのき',       'えのき',         'vegetable'),
(gen_random_uuid(), 'マッシュルーム','まっしゅるーむ', 'vegetable'),
(gen_random_uuid(), '松茸',         'まつたけ',       'vegetable'),
(gen_random_uuid(), 'レモン',       'れもん',         'vegetable'),
(gen_random_uuid(), 'ライム',       'らいむ',         'vegetable'),
(gen_random_uuid(), '三つ葉',       'みつば',         'vegetable'),
(gen_random_uuid(), '大葉',         'おおば',         'vegetable'),
(gen_random_uuid(), 'かぼちゃ',     'かぼちゃ',       'vegetable'),
(gen_random_uuid(), 'きゅうり',     'きゅうり',       'vegetable'),
(gen_random_uuid(), 'いんげん',     'いんげん',       'vegetable'),
(gen_random_uuid(), 'なす',         'なす',           'vegetable'),
(gen_random_uuid(), 'さつまいも',   'さつまいも',     'vegetable'),
(gen_random_uuid(), '春菊',         'しゅんぎく',     'vegetable'),
(gen_random_uuid(), '白菜',         'はくさい',       'vegetable'),
(gen_random_uuid(), 'ピーマン',     'ぴーまん',       'vegetable'),
(gen_random_uuid(), 'パプリカ',     'ぱぷりか',       'vegetable'),
(gen_random_uuid(), 'レタス',       'れたす',         'vegetable'),
(gen_random_uuid(), 'サンチュ',     'さんちゅ',       'vegetable'),
(gen_random_uuid(), 'セロリ',       'せろり',         'vegetable'),
(gen_random_uuid(), 'ズッキーニ',   'ずっきーに',     'vegetable'),
(gen_random_uuid(), 'ほうれん草',   'ほうれんそう',   'vegetable'),
(gen_random_uuid(), 'トマト',       'とまと',         'vegetable'),
(gen_random_uuid(), 'ミニトマト',   'みにとまと',     'vegetable'),
(gen_random_uuid(), 'オリーブ',     'おりーぶ',       'vegetable'),
(gen_random_uuid(), 'たけのこ',     'たけのこ',       'vegetable'),
(gen_random_uuid(), 'ニラ',         'にら',           'vegetable'),
(gen_random_uuid(), 'チンゲン菜',   'ちんげんさい',   'vegetable'),
(gen_random_uuid(), 'もやし',       'もやし',         'vegetable'),
(gen_random_uuid(), 'アボカド',     'あぼかど',       'vegetable'),
(gen_random_uuid(), 'バジル',       'ばじる',         'vegetable'),
(gen_random_uuid(), 'パクチー',     'ぱくちー',       'vegetable'),
(gen_random_uuid(), 'ビーツ',       'びーつ',         'vegetable'),
(gen_random_uuid(), '長芋',         'ながいも',       'vegetable'),
(gen_random_uuid(), '里芋',         'さといも',       'vegetable'),

-- ============ 肉 ============
(gen_random_uuid(), '鶏もも肉',     'とりももにく',   'meat'),
(gen_random_uuid(), '鶏むね肉',     'とりむねにく',   'meat'),
(gen_random_uuid(), '鶏ひき肉',     'とりひきにく',   'meat'),
(gen_random_uuid(), '鶏レバー',     'とりればー',     'meat'),
(gen_random_uuid(), '豚こま切れ肉', 'ぶたこまぎれにく','meat'),
(gen_random_uuid(), '豚バラ肉',     'ぶたばらにく',   'meat'),
(gen_random_uuid(), '豚ロース肉',   'ぶたろーすにく', 'meat'),
(gen_random_uuid(), '豚もも肉',     'ぶたももにく',   'meat'),
(gen_random_uuid(), '豚ひき肉',     'ぶたひきにく',   'meat'),
(gen_random_uuid(), '牛肉',         'ぎゅうにく',     'meat'),
(gen_random_uuid(), '牛すね肉',     'ぎゅうすねにく', 'meat'),
(gen_random_uuid(), '合いびき肉',   'あいびきにく',   'meat'),
(gen_random_uuid(), 'ラム肉',       'らむにく',       'meat'),
(gen_random_uuid(), '鴨肉',         'かもにく',       'meat'),
(gen_random_uuid(), 'ベーコン',     'べーこん',       'meat'),
(gen_random_uuid(), 'ハム',         'はむ',           'meat'),
(gen_random_uuid(), '生ハム',       'なまはむ',       'meat'),
(gen_random_uuid(), 'ウインナー',   'ういんなー',     'meat'),
(gen_random_uuid(), 'チャーシュー', 'ちゃーしゅー',   'meat'),

-- ============ 魚介 ============
(gen_random_uuid(), '鮭',           'さけ',           'seafood'),
(gen_random_uuid(), '鯖',           'さば',           'seafood'),
(gen_random_uuid(), 'ぶり',         'ぶり',           'seafood'),
(gen_random_uuid(), '鯛',           'たい',           'seafood'),
(gen_random_uuid(), '白身魚',       'しろみざかな',   'seafood'),
(gen_random_uuid(), 'まぐろ',       'まぐろ',         'seafood'),
(gen_random_uuid(), 'サーモン',     'さーもん',       'seafood'),
(gen_random_uuid(), '海老',         'えび',           'seafood'),
(gen_random_uuid(), 'いか',         'いか',           'seafood'),
(gen_random_uuid(), 'あさり',       'あさり',         'seafood'),
(gen_random_uuid(), 'うなぎ',       'うなぎ',         'seafood'),
(gen_random_uuid(), '数の子',       'かずのこ',       'seafood'),
(gen_random_uuid(), 'かに風味かまぼこ','かにふうみかまぼこ','seafood'),
(gen_random_uuid(), 'フカヒレ',     'ふかひれ',       'seafood'),
(gen_random_uuid(), '干し貝柱',     'ほしかいばしら', 'seafood'),
(gen_random_uuid(), '上海蟹',       'しゃんはいがに', 'seafood'),
(gen_random_uuid(), 'さんま',       'さんま',         'seafood'),
(gen_random_uuid(), 'いわし',       'いわし',         'seafood'),
(gen_random_uuid(), 'あじ',         'あじ',           'seafood'),
-- 干物は生の魚と売り場も用途も違うため、別の食材として持つ。
(gen_random_uuid(), 'あじの開き',   'あじのひらき',   'seafood'),
(gen_random_uuid(), 'しらす',       'しらす',         'seafood'),
(gen_random_uuid(), 'ちくわ',       'ちくわ',         'seafood'),
(gen_random_uuid(), 'はんぺん',     'はんぺん',       'seafood'),

-- ============ 卵・乳 ============
(gen_random_uuid(), '卵',           'たまご',         'dairy_egg'),
(gen_random_uuid(), 'うずら卵',     'うずらたまご',   'dairy_egg'),
(gen_random_uuid(), '牛乳',         'ぎゅうにゅう',   'dairy_egg'),
(gen_random_uuid(), '生クリーム',   'なまくりーむ',   'dairy_egg'),
(gen_random_uuid(), 'サワークリーム','さわーくりーむ', 'dairy_egg'),
(gen_random_uuid(), 'チーズ',       'ちーず',         'dairy_egg'),
(gen_random_uuid(), '粉チーズ',     'こなちーず',     'dairy_egg'),
(gen_random_uuid(), 'バター',       'ばたー',         'dairy_egg'),
(gen_random_uuid(), 'ヨーグルト',   'よーぐると',     'dairy_egg'),

-- ============ 主食 ============
(gen_random_uuid(), '米',           'こめ',           'staple'),
(gen_random_uuid(), 'もち米',       'もちごめ',       'staple'),
(gen_random_uuid(), '食パン',       'しょくぱん',     'staple'),
(gen_random_uuid(), 'ホットドッグ用パン','ほっとどっぐようぱん','staple'),
(gen_random_uuid(), 'うどん',       'うどん',         'staple'),
(gen_random_uuid(), 'そば',         'そば',           'staple'),
(gen_random_uuid(), '中華麺',       'ちゅうかめん',   'staple'),
(gen_random_uuid(), 'ビーフン',     'びーふん',       'staple'),
(gen_random_uuid(), 'フォー',       'ふぉー',         'staple'),
(gen_random_uuid(), '春雨',         'はるさめ',       'staple'),
(gen_random_uuid(), 'スパゲッティ', 'すぱげってぃ',   'staple'),
(gen_random_uuid(), 'マカロニ',     'まかろに',       'staple'),
(gen_random_uuid(), 'ラザニアシート','らざにあしーと', 'staple'),
(gen_random_uuid(), 'クスクス',     'くすくす',       'staple'),
(gen_random_uuid(), 'トルティーヤ', 'とるてぃーや',   'staple'),
(gen_random_uuid(), 'パイシート',   'ぱいしーと',     'staple'),
(gen_random_uuid(), '餃子の皮',     'ぎょうざのかわ', 'staple'),
(gen_random_uuid(), '焼売の皮',     'しゅうまいのかわ','staple'),
(gen_random_uuid(), '春巻きの皮',   'はるまきのかわ', 'staple'),
(gen_random_uuid(), '小籠包の皮',   'しょうろんぽうのかわ','staple'),
(gen_random_uuid(), '春餅',         'しゅんぴん',     'staple'),
(gen_random_uuid(), 'そうめん',     'そうめん',       'staple'),
(gen_random_uuid(), 'きりたんぽ',   'きりたんぽ',     'staple'),

-- ============ その他 ============
(gen_random_uuid(), '豆腐',         'とうふ',         'other'),
(gen_random_uuid(), '焼き豆腐',     'やきどうふ',     'other'),
(gen_random_uuid(), '油揚げ',       'あぶらあげ',     'other'),
(gen_random_uuid(), '厚揚げ',       'あつあげ',       'other'),
-- 梅干しは味付けに使うが、常備しておらずその献立のために買うため食材として扱う（14.4）。
(gen_random_uuid(), '梅干し',       'うめぼし',       'other'),
(gen_random_uuid(), 'こんにゃく',   'こんにゃく',     'other'),
(gen_random_uuid(), '糸こんにゃく', 'いとこんにゃく', 'other'),
(gen_random_uuid(), 'しらたき',     'しらたき',       'other'),
(gen_random_uuid(), '海苔',         'のり',           'other'),
(gen_random_uuid(), 'わかめ',       'わかめ',         'other'),
(gen_random_uuid(), '昆布',         'こんぶ',         'other'),
(gen_random_uuid(), '黒豆',         'くろまめ',       'other'),
(gen_random_uuid(), 'ひよこ豆',     'ひよこまめ',     'other'),
(gen_random_uuid(), 'キムチ',       'きむち',         'other'),
(gen_random_uuid(), 'トマト缶',     'とまとかん',     'other'),
(gen_random_uuid(), 'ツナ缶',       'つなかん',       'other'),
(gen_random_uuid(), 'コーン缶',     'こーんかん',     'other'),
(gen_random_uuid(), 'ココナッツミルク','ここなっつみるく','other'),
-- ルウ類は常備しておらず、その献立のために買う必要があるため食材として扱う（14.4）。
(gen_random_uuid(), 'カレールウ',   'かれーるう',     'other'),
(gen_random_uuid(), 'シチュールウ', 'しちゅーるう',   'other'),
(gen_random_uuid(), 'デミグラスソース缶','でみぐらすそーすかん','other'),
(gen_random_uuid(), 'チョコレート', 'ちょこれーと',   'other'),
(gen_random_uuid(), 'アーモンド',   'あーもんど',     'other')
-- **upsert にする。** DO NOTHING（挿入のみ）だと、カナやカテゴリの誤りを直しても
-- 既にシード済みのDB（本番など）には永久に反映されない。カナは買い物リストの並び順、
-- カテゴリは売り場の分類に使うため、ずれたまま直せないのは実害になる。
-- このファイルを「あるべき状態」の定義として扱い、再実行で追いつかせる。
--
-- ただし**名前の変更だけはこれでは直らない**（name がキーのため、旧行が残って
-- 新行が増える）。名前を変えるときはマイグレーションで対応すること。
ON CONFLICT (name) DO UPDATE SET
    name_kana = EXCLUDED.name_kana,
    category  = EXCLUDED.category;
