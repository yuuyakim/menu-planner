import { defaultRoleFilter, type Difficulty, type Genre, type RoleFilter } from '../../api/types'

// 検索の絞り込み条件。フォーム（SearchForm）とAPI呼び出し（api.ts）の
// 両方が使うため、コンポーネントとは別のモジュールに置く。

/** MenuFilter は検索の絞り込み条件。undefined は「絞り込まない」。 */
export type MenuFilter = {
  genre?: Genre
  difficulty?: Difficulty
  /**
   * role は献立の役割（spec.md 2.10）。
   *
   * **undefined にしない。** 省略するとサーバ側で主菜になるため、
   * 「すべて」を選んだつもりが主菜に絞られる。常に明示して送る。
   */
  role: RoleFilter
}

/**
 * emptyMenuFilter は「何も絞っていない」初期値。
 * ジャンル・難易度は未指定だが、役割だけは既定の主菜が入る（spec.md 2.10）。
 */
export const emptyMenuFilter: MenuFilter = { role: defaultRoleFilter }

/**
 * withDefaultRole は役割が欠けた条件に既定を補う。
 *
 * **sessionStorage には役割を持たない古い形が残っている可能性がある。**
 * 役割を足す前にタブを開いていた利用者は `{genre, difficulty}` だけを持っており、
 * そのまま送ると `role=undefined` という文字列になって 400 になる。
 */
export function withDefaultRole(filter: MenuFilter): MenuFilter {
  return { ...filter, role: filter.role ?? defaultRoleFilter }
}
