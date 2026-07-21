import type { ReactNode } from 'react'

type Props = {
  /** 待っていることを伝える文言。読み上げるのはこちら。 */
  children: ReactNode
  /** 場面に合う絵。既定は「考え中」。 */
  image?: string
}

// MascotStatus は待ち時間にこんたてんを添えた状態表示。
//
// 絵は装飾なので alt は空にする。何を待っているかは文言が伝えるため、
// alt を付けると読み上げが文言の前にキャラの説明を挟むことになる。
//
// 揺れる動きは motion-safe に限る。動きを減らす設定を入れている利用者には
// 揺らさない（アニメーション自体が体調に響く場合がある）。
export function MascotStatus({
  children,
  image = '/mascot/pose-idea.png',
}: Props) {
  return (
    <p
      role="status"
      className="flex items-center gap-3 rounded-2xl bg-kon-cream px-5 py-4 text-kon-ink"
    >
      <img
        src={image}
        alt=""
        className="size-12 motion-safe:animate-kon-bob"
      />
      {children}
    </p>
  )
}
