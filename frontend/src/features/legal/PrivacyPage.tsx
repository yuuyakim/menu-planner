import md from './content/privacy.md?raw'

import { LegalPage } from './LegalPage'

// プライバシーポリシー。文言の正は frontend/src/features/legal/content/privacy.md。
export function PrivacyPage() {
  return <LegalPage markdown={md} />
}
