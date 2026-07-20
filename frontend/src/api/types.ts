import type { components } from './schema'

// schema.d.ts は自動生成物で、参照が `components['schemas']['Menu']` と冗長になる。
// アプリ側はこの別名だけを使い、生成物への依存をこのファイルに閉じ込める。
// API仕様が変われば再生成された型がここを通って全体に伝播する。

type Schemas = components['schemas']

export type Genre = Schemas['Genre']
export type Difficulty = Schemas['Difficulty']
export type SearchMode = Schemas['SearchMode']

export type Menu = Schemas['Menu']
export type DayMenu = Schemas['DayMenu']
export type Recipe = Schemas['Recipe']
export type User = Schemas['User']
export type HistoryItem = Schemas['HistoryItem']
export type FavoriteItem = Schemas['FavoriteItem']

/** RFC 7807 の problem+json。 */
export type Problem = Schemas['Problem']

// 画面で選択肢を並べるための値。型は生成物から取り、値はここで定義する
// （OpenAPI の enum は型としてしか生成されないため）。
// 網羅は Record の型注釈が保証する。増減すれば型エラーになる。
export const genreLabels: Record<Genre, string> = {
  japanese: '和食',
  western: '洋食',
  chinese: '中華',
  other: 'その他',
}

export const difficultyLabels: Record<Difficulty, string> = {
  easy: '簡単',
  normal: '普通',
  elaborate: '手が込んだ',
}

export const genres = Object.keys(genreLabels) as Genre[]
export const difficulties = Object.keys(difficultyLabels) as Difficulty[]
