/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { balanceKeyMatchesHint } from '../lib/balance-key-test'
import type { BalanceKey } from '../types'
import { BalanceKeyTestPanel } from './balance-key-test-panel'

type BalanceKeyTestDialogProps = {
  target: BalanceKey | null
  onClose: () => void
}

/**
 * Tests a key that already exists. Only a digest is stored, so the user pastes
 * the key back in; it is checked against the entry's hint first so a test never
 * runs with a different key than the one the user clicked.
 */
export function BalanceKeyTestDialog(props: BalanceKeyTestDialogProps) {
  const { t } = useTranslation()
  const [pastedKey, setPastedKey] = useState('')
  const key = pastedKey.trim()
  const hint = props.target?.token_hint ?? ''
  const matches = key !== '' && balanceKeyMatchesHint(key, hint)

  const close = (open: boolean) => {
    if (open) return
    setPastedKey('')
    props.onClose()
  }

  return (
    <Dialog
      open={props.target !== null}
      onOpenChange={close}
      title={t('Test balance key')}
      description={t(
        'Paste the full key for {{name}}. It is only sent to this site to query the balance.',
        { name: props.target?.name ?? '' }
      )}
      contentClassName='sm:max-w-lg'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <Button type='button' onClick={() => close(false)}>
          {t('Done')}
        </Button>
      }
    >
      <div className='my-4 space-y-4'>
        <div className='space-y-2'>
          <Label htmlFor='balance-key-test-input'>{t('Balance key')}</Label>
          <Input
            id='balance-key-test-input'
            value={pastedKey}
            autoFocus
            autoComplete='off'
            spellCheck={false}
            placeholder={hint}
            className='font-mono text-xs'
            aria-invalid={key !== '' && !matches}
            onChange={(event) => setPastedKey(event.target.value)}
          />
          {key !== '' && !matches ? (
            <p className='text-destructive text-xs'>
              {t('This key does not match {{hint}}.', { hint })}
            </p>
          ) : null}
        </div>
        {matches ? <BalanceKeyTestPanel key={key} balanceKey={key} /> : null}
      </div>
    </Dialog>
  )
}
