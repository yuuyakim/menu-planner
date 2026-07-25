import md from '../../../../docs/legal/terms.md?raw'

import { LegalPage } from './LegalPage'

// 利用規約。文言の正は docs/legal/terms.md。
export function TermsPage() {
  return <LegalPage markdown={md} />
}
