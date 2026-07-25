import md from './content/tokushoho.md?raw'

import { LegalPage } from './LegalPage'

// 特定商取引法に基づく表記。文言の正は frontend/src/features/legal/content/tokushoho.md。
export function TokushohoPage() {
  return <LegalPage markdown={md} />
}
