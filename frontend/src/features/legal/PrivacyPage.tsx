import md from '../../../../docs/legal/privacy.md?raw'

import { LegalPage } from './LegalPage'

// プライバシーポリシー。文言の正は docs/legal/privacy.md。
export function PrivacyPage() {
  return <LegalPage markdown={md} />
}
