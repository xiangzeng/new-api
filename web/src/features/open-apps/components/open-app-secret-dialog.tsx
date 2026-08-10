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
import { AlertTriangle, Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'

import type { OpenAppSecretPayload } from '../types'

interface OpenAppSecretDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Present only right after a create or a secret reset. */
  payload: OpenAppSecretPayload | null
}

/**
 * Shows the clear-text secret once. The server stores only an HMAC digest, so
 * this dialog is the single opportunity to copy it — there is no "show again".
 */
export function OpenAppSecretDialog(props: OpenAppSecretDialogProps) {
  const { t } = useTranslation()

  const copy = async (value: string, label: string) => {
    try {
      await navigator.clipboard.writeText(value)
      toast.success(t('{{label}} copied', { label }))
    } catch {
      toast.error(t('Failed to copy to clipboard'))
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Application credentials')}
      description={t('Send these to the partner over a secure channel.')}
      contentClassName='sm:max-w-lg'
      footer={
        <Button onClick={() => props.onOpenChange(false)}>{t('Done')}</Button>
      }
    >
      <div className='space-y-4'>
        <Alert>
          <AlertTriangle aria-hidden='true' />
          <AlertDescription>
            {t(
              'The secret is shown only once and cannot be recovered. If it is lost, reset it and re-authorize the partner.'
            )}
          </AlertDescription>
        </Alert>

        <div className='space-y-1.5'>
          <Label>{t('App ID')}</Label>
          <div className='flex items-center gap-2'>
            <code className='bg-muted min-w-0 flex-1 truncate rounded-md px-3 py-2 font-mono text-xs'>
              {props.payload?.app.app_id ?? ''}
            </code>
            <Button
              variant='outline'
              size='icon'
              aria-label={t('Copy App ID')}
              onClick={() => copy(props.payload?.app.app_id ?? '', t('App ID'))}
            >
              <Copy />
            </Button>
          </div>
        </div>

        <div className='space-y-1.5'>
          <Label>{t('App Secret')}</Label>
          <div className='flex items-center gap-2'>
            <code className='bg-muted min-w-0 flex-1 truncate rounded-md px-3 py-2 font-mono text-xs'>
              {props.payload?.app_secret ?? ''}
            </code>
            <Button
              variant='outline'
              size='icon'
              aria-label={t('Copy App Secret')}
              onClick={() =>
                copy(props.payload?.app_secret ?? '', t('App Secret'))
              }
            >
              <Copy />
            </Button>
          </div>
        </div>
      </div>
    </Dialog>
  )
}
