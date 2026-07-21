import type { ReactNode } from 'react'

type Props = {
  /** 何が無いのか、これからどうすれば埋まるのかを伝える文言。 */
  children: ReactNode
  /** 場面に合う絵。既定は「うーん…」。 */
  image?: string
}

// MascotEmpty は一覧が空のときの表示。
//
// role は付けない。空は通常の状態であって知らせるべき出来事ではなく、
// status にすると画面を開くたびに読み上げが割り込む。
// 絵は装飾（alt=""）で、意味は文言が持つ。
export function MascotEmpty({
  children,
  image = '/mascot/face-thinking.png',
}: Props) {
  return (
    <div className="flex flex-col items-center gap-3 rounded-2xl border border-dashed border-kon-leaf-soft bg-white/70 px-6 py-10 text-center">
      <img src={image} alt="" className="size-24" />
      <p className="text-kon-ink/75">{children}</p>
    </div>
  )
}
