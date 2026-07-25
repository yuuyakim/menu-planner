import md from './content/terms.md?raw'

import { LegalPage } from './LegalPage'

// 利用規約。文言の正は frontend/src/features/legal/content/terms.md。
export function TermsPage() {
  return <LegalPage markdown={md} />
}
