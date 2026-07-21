-- 献立マスタ。4ジャンル × 3難易度 × 各10件 = 120件。
--
-- 週間献立で7日分を重複なく引くため、(genre, difficulty) の組み合わせごとに
-- 最低10件を確保する。id は gen_random_uuid() で採番し、name の UNIQUE 制約と
-- ON CONFLICT DO NOTHING により再実行しても重複しない（冪等）。

INSERT INTO menus (id, name, name_kana, genre, difficulty, description) VALUES
-- ============ 和食 × 簡単 ============
(gen_random_uuid(), '親子丼',        'おやこどん',        'japanese', 'easy', '鶏肉と卵を甘辛い出汁でとじた定番の丼もの'),
(gen_random_uuid(), '豚の生姜焼き',   'ぶたのしょうがやき',  'japanese', 'easy', '生姜の効いたタレで豚肉を焼く、ご飯が進む一品'),
(gen_random_uuid(), '鮭の塩焼き',     'さけのしおやき',     'japanese', 'easy', '塩を振って焼くだけ。魚料理の入り口として手軽'),
(gen_random_uuid(), '牛丼',          'ぎゅうどん',        'japanese', 'easy', '玉ねぎと牛肉を甘辛く煮てご飯にのせる'),
(gen_random_uuid(), 'かつ丼',        'かつどん',          'japanese', 'easy', '揚げたとんかつを卵でとじた満足感のある丼'),
(gen_random_uuid(), '肉じゃが',      'にくじゃが',        'japanese', 'easy', 'じゃがいもと牛肉を出汁で煮込んだ家庭料理の代表'),
(gen_random_uuid(), 'だし巻き卵定食', 'だしまきたまごていしょく', 'japanese', 'easy', 'ふんわりした卵焼きを主役にした軽めの定食'),
(gen_random_uuid(), 'きつねうどん',   'きつねうどん',      'japanese', 'easy', '甘く煮た油揚げをのせた出汁の効いたうどん'),
(gen_random_uuid(), 'ざるそば',      'ざるそば',          'japanese', 'easy', '茹でて冷水で締めるだけ。暑い日に向く'),
(gen_random_uuid(), '鶏の照り焼き',   'とりのてりやき',     'japanese', 'easy', '甘辛いタレを絡めて照りよく仕上げた鶏もも肉'),

-- ============ 和食 × 普通 ============
(gen_random_uuid(), '筑前煮',        'ちくぜんに',        'japanese', 'normal', '根菜と鶏肉を出汁で含め煮にした滋味深い煮物'),
(gen_random_uuid(), '鯖の味噌煮',     'さばのみそに',      'japanese', 'normal', '味噌と生姜で青魚の風味を活かして煮る'),
(gen_random_uuid(), '豚汁定食',      'とんじるていしょく',  'japanese', 'normal', '具だくさんの豚汁を主役にした定食'),
(gen_random_uuid(), '鶏の唐揚げ',     'とりのからあげ',     'japanese', 'normal', '下味を付けて二度揚げする、外はカリッと中はジューシー'),
(gen_random_uuid(), '茶碗蒸し',      'ちゃわんむし',      'japanese', 'normal', '出汁と卵の配合と蒸し加減が仕上がりを左右する'),
(gen_random_uuid(), 'ぶり大根',      'ぶりだいこん',      'japanese', 'normal', 'ぶりの旨味を大根に含ませた冬の煮物'),
(gen_random_uuid(), 'かぼちゃの煮物',  'かぼちゃのにもの',   'japanese', 'normal', '煮崩れさせずに味を含ませるのがコツ'),
(gen_random_uuid(), '手巻き寿司',     'てまきずし',        'japanese', 'normal', '酢飯と具材を用意して各自で巻く。人が集まる日に'),
(gen_random_uuid(), '鶏そぼろ丼',     'とりそぼろどん',     'japanese', 'normal', '鶏そぼろと炒り卵を彩りよく盛った二色丼'),
(gen_random_uuid(), 'いなり寿司',     'いなりずし',        'japanese', 'normal', '甘辛く煮た油揚げに酢飯を詰める'),

-- ============ 和食 × 手が込んだ ============
(gen_random_uuid(), '天ぷら盛り合わせ', 'てんぷらもりあわせ',  'japanese', 'elaborate', '衣の温度管理と揚げ油の見極めが要る'),
(gen_random_uuid(), '握り寿司',      'にぎりずし',        'japanese', 'elaborate', 'ネタの仕込みとシャリの加減に技術が要る'),
(gen_random_uuid(), '鰻の蒲焼き',     'うなぎのかばやき',   'japanese', 'elaborate', 'タレを重ねながら焼き上げる。下処理も手間'),
(gen_random_uuid(), 'すき焼き',      'すきやき',          'japanese', 'elaborate', '割下の配合と具材を入れる順序で味が決まる'),
(gen_random_uuid(), 'しゃぶしゃぶ',   'しゃぶしゃぶ',      'japanese', 'elaborate', '出汁とタレを揃え、肉と野菜を順に楽しむ'),
(gen_random_uuid(), 'おせち料理',     'おせちりょうり',     'japanese', 'elaborate', '多数の品を一つずつ仕込む。数日がかりの献立'),
(gen_random_uuid(), '土瓶蒸し',      'どびんむし',        'japanese', 'elaborate', '松茸と鱧を土瓶で蒸した、出汁を味わう秋の一品'),
(gen_random_uuid(), '鯛の姿造り',     'たいのすがたづくり',  'japanese', 'elaborate', '一尾をさばいて盛り付ける。包丁の技術が要る'),
(gen_random_uuid(), '松花堂弁当',     'しょうかどうべんとう', 'japanese', 'elaborate', '四つ切りの器に複数の料理を彩りよく詰める'),
(gen_random_uuid(), '鴨鍋',          'かもなべ',          'japanese', 'elaborate', '鴨の火入れと出汁の取り方で仕上がりが変わる'),

-- ============ 洋食 × 簡単 ============
(gen_random_uuid(), 'ナポリタン',     'なぽりたん',        'western', 'easy', 'ケチャップで炒めた喫茶店風のスパゲッティ'),
(gen_random_uuid(), 'ミートソーススパゲッティ', 'みーとそーすすぱげってぃ', 'western', 'easy', '挽き肉とトマトのソースを絡めた定番パスタ'),
(gen_random_uuid(), 'オムライス',     'おむらいす',        'western', 'easy', 'チキンライスを卵で包む。卵の火入れだけ注意'),
(gen_random_uuid(), 'ハンバーグ',     'はんばーぐ',        'western', 'easy', 'こねて焼くだけ。付け合わせで見栄えが変わる'),
(gen_random_uuid(), 'チキンソテー',   'ちきんそてー',      'western', 'easy', '皮目をパリッと焼き上げた鶏もも肉'),
(gen_random_uuid(), 'ツナサンド',     'つなさんど',        'western', 'easy', 'ツナとマヨネーズを挟むだけの手軽な軽食'),
(gen_random_uuid(), 'コーンスープとパン', 'こーんすーぷとぱん', 'western', 'easy', '甘みのあるスープとパンで済ませる軽い献立'),
(gen_random_uuid(), 'ベーコンエッグ',  'べーこんえっぐ',     'western', 'easy', '焼くだけ。朝食にも夜食にも使える'),
(gen_random_uuid(), 'ポテトサラダ',   'ぽてとさらだ',      'western', 'easy', '茹でて潰して和えるだけ。作り置きにも向く'),
(gen_random_uuid(), 'カルボナーラ',   'かるぼなーら',      'western', 'easy', '卵とチーズを余熱で和える。分離だけ注意'),

-- ============ 洋食 × 普通 ============
(gen_random_uuid(), 'ロールキャベツ',  'ろーるきゃべつ',     'western', 'normal', 'キャベツで挽き肉を巻き、スープで煮込む'),
(gen_random_uuid(), 'グラタン',      'ぐらたん',          'western', 'normal', 'ホワイトソースを作ってオーブンで焼き上げる'),
(gen_random_uuid(), 'ミネストローネ',  'みねすとろーね',     'western', 'normal', '野菜を細かく切り揃えて煮込むトマトのスープ'),
(gen_random_uuid(), 'ポトフ',        'ぽとふ',            'western', 'normal', '大ぶりの野菜と肉をじっくり煮込む'),
(gen_random_uuid(), 'チキンのトマト煮込み', 'ちきんのとまとにこみ', 'western', 'normal', '鶏肉をトマトとハーブで煮込んだ一皿'),
(gen_random_uuid(), 'キッシュ',      'きっしゅ',          'western', 'normal', '生地を敷いて卵液を流し、オーブンで焼く'),
(gen_random_uuid(), 'ハヤシライス',   'はやしらいす',      'western', 'normal', 'デミグラス風のソースで牛肉と玉ねぎを煮る'),
(gen_random_uuid(), 'クリームシチュー', 'くりーむしちゅー',   'western', 'normal', 'ルウから作ると小麦粉の炒め加減が味を決める'),
(gen_random_uuid(), 'アクアパッツァ',  'あくあぱっつぁ',     'western', 'normal', '白身魚とあさりを蒸し煮にした地中海風'),
(gen_random_uuid(), 'ガーリックシュリンプ', 'がーりっくしゅりんぷ', 'western', 'normal', 'にんにくとバターで海老を焼き付ける'),

-- ============ 洋食 × 手が込んだ ============
(gen_random_uuid(), 'ビーフシチュー',  'びーふしちゅー',     'western', 'elaborate', '牛肉を数時間煮込む。赤ワインで香りを重ねる'),
(gen_random_uuid(), 'ラザニア',      'らざにあ',          'western', 'elaborate', 'ミートソースとベシャメルを層に重ねて焼く'),
(gen_random_uuid(), 'ローストビーフ',  'ろーすとびーふ',     'western', 'elaborate', '低温でじっくり火入れし、休ませて仕上げる'),
(gen_random_uuid(), 'ブイヤベース',   'ぶいやべーす',      'western', 'elaborate', '複数の魚介から出汁を取る南仏の魚介鍋'),
(gen_random_uuid(), 'パエリア',      'ぱえりあ',          'western', 'elaborate', '米に出汁を吸わせ、おこげまで作る'),
(gen_random_uuid(), 'コンフィ・ド・カナール', 'こんふぃどかなーる', 'western', 'elaborate', '鴨を脂で低温調理する保存食由来の一品'),
(gen_random_uuid(), 'ビーフウェリントン', 'びーふうぇりんとん', 'western', 'elaborate', 'フィレ肉をパイ生地で包んで焼く。火入れが難所'),
(gen_random_uuid(), 'オッソブーコ',   'おっそぶーこ',      'western', 'elaborate', '仔牛のすね肉を白ワインで煮込むミラノ料理'),
(gen_random_uuid(), 'テリーヌ',      'てりーぬ',          'western', 'elaborate', '型に詰めて湯煎焼きし、冷やして仕上げる'),
(gen_random_uuid(), 'コック・オ・ヴァン', 'こっくおゔぁん',    'western', 'elaborate', '鶏を赤ワインで煮込むフランスの郷土料理'),

-- ============ 中華 × 簡単 ============
(gen_random_uuid(), '麻婆豆腐',      'まーぼーどうふ',     'chinese', 'easy', '豆板醤と豆腐で作る、短時間で仕上がる定番'),
(gen_random_uuid(), 'チャーハン',     'ちゃーはん',        'chinese', 'easy', '強火で手早く炒める。冷やご飯の活用にも'),
(gen_random_uuid(), '回鍋肉',        'ほいこーろー',      'chinese', 'easy', 'キャベツと豚肉を甜麺醤で炒める'),
(gen_random_uuid(), '青椒肉絲',      'ちんじゃおろーす',   'chinese', 'easy', 'ピーマンと細切り肉を手早く炒める'),
(gen_random_uuid(), '中華スープ',     'ちゅうかすーぷ',     'chinese', 'easy', '鶏がらベースの卵スープ。副菜としても'),
(gen_random_uuid(), '冷やし中華',     'ひやしちゅうか',     'chinese', 'easy', '具材を千切りにして並べる。夏の定番'),
(gen_random_uuid(), '卵とトマトの炒め物', 'たまごととまとのいためもの', 'chinese', 'easy', '卵をふんわり仕上げるのがコツ'),
(gen_random_uuid(), '焼きビーフン',   'やきびーふん',      'chinese', 'easy', '戻した米麺と野菜を炒め合わせる'),
(gen_random_uuid(), 'ニラ玉',        'にらたま',          'chinese', 'easy', 'ニラと卵を炒めるだけ。手早く一品追加できる'),
(gen_random_uuid(), '天津飯',        'てんしんはん',      'chinese', 'easy', 'かに玉をご飯にのせ、あんをかける'),

-- ============ 中華 × 普通 ============
(gen_random_uuid(), '餃子',          'ぎょうざ',          'chinese', 'normal', '包む手間はあるが、焼き加減で差が出る'),
(gen_random_uuid(), '酢豚',          'すぶた',            'chinese', 'normal', '揚げた豚肉に甘酢あんを絡める'),
(gen_random_uuid(), 'エビチリ',      'えびちり',          'chinese', 'normal', '海老の下処理とチリソースの辛さ調整がポイント'),
(gen_random_uuid(), '春巻き',        'はるまき',          'chinese', 'normal', '具を炒めて冷まし、包んで揚げる'),
(gen_random_uuid(), '担々麺',        'たんたんめん',      'chinese', 'normal', '芝麻醤のスープと肉味噌を合わせる'),
(gen_random_uuid(), '油淋鶏',        'ゆーりんちー',      'chinese', 'normal', '揚げた鶏に葱ダレをかける'),
(gen_random_uuid(), '八宝菜',        'はっぽうさい',      'chinese', 'normal', '多種の具材を炒めてあんでまとめる'),
(gen_random_uuid(), '麻婆茄子',      'まーぼーなす',      'chinese', 'normal', '茄子を素揚げしてから炒め合わせる'),
(gen_random_uuid(), '焼売',          'しゅうまい',        'chinese', 'normal', '餡を包んで蒸す。皮の扱いに慣れが要る'),
(gen_random_uuid(), '中華丼',        'ちゅうかどん',      'chinese', 'normal', '八宝菜風の具をご飯にのせる'),

-- ============ 中華 × 手が込んだ ============
(gen_random_uuid(), '北京ダック',     'ぺきんだっく',      'chinese', 'elaborate', '皮をパリッと焼き上げる。仕込みに数日かかる'),
(gen_random_uuid(), '小籠包',        'しょうろんぽう',     'chinese', 'elaborate', 'スープを煮凝りにして包む。皮も手作り'),
(gen_random_uuid(), 'フカヒレの姿煮',  'ふかひれのすがたに',  'chinese', 'elaborate', '戻しと煮込みに時間をかける高級料理'),
(gen_random_uuid(), '東坡肉',        'とんぽーろー',      'chinese', 'elaborate', '豚バラを長時間煮込んでとろとろに仕上げる'),
(gen_random_uuid(), '佛跳牆',        'ぶっちょうしょう',   'chinese', 'elaborate', '多数の高級食材を壺で蒸し込む福建料理'),
(gen_random_uuid(), '上海蟹の紹興酒漬け', 'しゃんはいがにのしょうこうしゅづけ', 'chinese', 'elaborate', '生の蟹を紹興酒に漬け込む。仕込みに数日'),
(gen_random_uuid(), '中華ちまき',     'ちゅうかちまき',     'chinese', 'elaborate', 'もち米と具材を竹皮で包んで蒸す'),
(gen_random_uuid(), '刀削麺',        'とうしょうめん',     'chinese', 'elaborate', '生地を削って茹でる。麺打ちの技術が要る'),
(gen_random_uuid(), '火鍋',          'ひなべ',            'chinese', 'elaborate', '複数のスープと薬味を揃える。準備が大がかり'),
(gen_random_uuid(), '叉焼',          'ちゃーしゅー',      'chinese', 'elaborate', '漬け込みと焼きに時間をかける'),

-- ============ その他 × 簡単 ============
(gen_random_uuid(), 'カレーライス',   'かれーらいす',      'other', 'easy', 'ルウを使えば失敗が少ない。作り置きにも向く'),
(gen_random_uuid(), 'タコライス',     'たこらいす',        'other', 'easy', '挽き肉と野菜をご飯にのせる沖縄発の一皿'),
(gen_random_uuid(), 'ガパオライス',   'がぱおらいす',      'other', 'easy', 'バジルと挽き肉を炒めて目玉焼きをのせる'),
(gen_random_uuid(), 'ビビンバ',      'びびんば',          'other', 'easy', 'ナムルと肉をご飯にのせて混ぜる'),
(gen_random_uuid(), 'キムチチゲ',     'きむちちげ',        'other', 'easy', 'キムチと豆腐を煮るだけ。体が温まる'),
(gen_random_uuid(), 'チキンオーバーライス', 'ちきんおーばーらいす', 'other', 'easy', 'スパイスチキンとサラダをご飯にのせる'),
(gen_random_uuid(), 'サラダボウル',   'さらだぼうる',      'other', 'easy', '野菜とタンパク質を一皿に。軽く済ませたい日に'),
(gen_random_uuid(), 'ホットドッグ',   'ほっとどっぐ',      'other', 'easy', 'ソーセージを挟むだけ。手軽な軽食'),
(gen_random_uuid(), 'タコス',        'たこす',            'other', 'easy', '具材を用意して各自で巻く'),
(gen_random_uuid(), 'プルコギ',      'ぷるこぎ',          'other', 'easy', '甘辛いタレで牛肉と野菜を炒める'),

-- ============ その他 × 普通 ============
(gen_random_uuid(), 'トムヤムクン',   'とむやむくん',      'other', 'normal', '酸味と辛味のバランスを取るタイの海老スープ'),
(gen_random_uuid(), 'グリーンカレー',  'ぐりーんかれー',     'other', 'normal', 'ペーストとココナッツミルクで作るタイカレー'),
(gen_random_uuid(), 'チャプチェ',     'ちゃぷちぇ',        'other', 'normal', '春雨と野菜を炒め合わせた韓国料理'),
(gen_random_uuid(), 'スンドゥブチゲ',  'すんどぅぶちげ',     'other', 'normal', '純豆腐を使った辛味のある鍋'),
(gen_random_uuid(), 'バターチキンカレー', 'ばたーちきんかれー', 'other', 'normal', 'トマトとバターでまろやかに仕上げるインドカレー'),
(gen_random_uuid(), 'フォー',        'ふぉー',            'other', 'normal', '澄んだスープと米麺のベトナム料理'),
(gen_random_uuid(), 'ケバブプレート',  'けばぶぷれーと',     'other', 'normal', 'スパイスで漬けた肉を焼いてライスと合わせる'),
(gen_random_uuid(), 'チキンティッカマサラ', 'ちきんてぃっかまさら', 'other', 'normal', '焼いた鶏をスパイスソースで煮込む'),
(gen_random_uuid(), 'サムギョプサル',  'さむぎょぷさる',     'other', 'normal', '豚バラを焼いて野菜で包む'),
(gen_random_uuid(), 'ナシゴレン',     'なしごれん',        'other', 'normal', 'ケチャップマニスで炒めるインドネシア風炒飯'),

-- ============ その他 × 手が込んだ ============
(gen_random_uuid(), 'ビリヤニ',      'びりやに',          'other', 'elaborate', '米と肉を層にして炊き上げる。スパイス配合が要'),
(gen_random_uuid(), 'サムゲタン',     'さむげたん',        'other', 'elaborate', '鶏を丸ごと薬膳と共に煮込む'),
(gen_random_uuid(), 'ムサカ',        'むさか',            'other', 'elaborate', '茄子と挽き肉を層にして焼くギリシャ料理'),
(gen_random_uuid(), 'タジン鍋',      'たじんなべ',        'other', 'elaborate', '専用鍋で蒸し煮にするモロッコ料理'),
(gen_random_uuid(), 'シュラスコ',     'しゅらすこ',        'other', 'elaborate', '塊肉を串に刺して炭火でじっくり焼く'),
(gen_random_uuid(), 'モレ・ポブラーノ', 'もれぽぶらーの',    'other', 'elaborate', '多数の唐辛子とチョコを使うメキシコのソース'),
(gen_random_uuid(), 'クスクス',      'くすくす',          'other', 'elaborate', '蒸したセモリナに煮込みを合わせる北アフリカ料理'),
(gen_random_uuid(), 'ボルシチ',      'ぼるしち',          'other', 'elaborate', 'ビーツと牛肉をじっくり煮込む'),
(gen_random_uuid(), 'ラムのロースト',  'らむのろーすと',     'other', 'elaborate', 'ハーブをまとわせて塊肉を焼き上げる'),
(gen_random_uuid(), 'セビーチェ',     'せびーちぇ',        'other', 'elaborate', '魚介を柑橘で締めるペルーの前菜。鮮度が命')
-- **upsert にする。** DO NOTHING（挿入のみ）だと、説明文やカナを直しても
-- 既にシード済みのDB（本番など）には永久に反映されない。
-- このファイルを「あるべき状態」の定義として扱い、再実行で追いつかせる。
--
-- ただし**名前の変更だけはこれでは直らない**（name がキーのため、旧行が残って
-- 新行が増える）。名前を変えるときはマイグレーションで対応すること。
ON CONFLICT (name) DO UPDATE SET
    name_kana   = EXCLUDED.name_kana,
    genre       = EXCLUDED.genre,
    difficulty  = EXCLUDED.difficulty,
    description = EXCLUDED.description;
