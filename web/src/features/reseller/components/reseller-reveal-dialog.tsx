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
import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'

import { revealVoucher, revealVoucherBatch } from '../api'

interface ResellerRevealDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  publicId: string
  batch: boolean
}

export function ResellerRevealDialog({
  open,
  onOpenChange,
  publicId,
  batch,
}: ResellerRevealDialogProps) {
  const { t } = useTranslation()
  const [password, setPassword] = useState('')
  const [codes, setCodes] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const verification = useSecureVerification({
    onError: () => setLoading(false),
  })

  useEffect(() => {
    if (open) return
    setPassword('')
    setCodes([])
    setLoading(false)
  }, [open])

  const submit = async () => {
    setLoading(true)
    await verification.startVerification(
      async (proof) => {
        try {
          const response = batch
            ? await revealVoucherBatch(publicId, password, proof)
            : await revealVoucher(publicId, password, proof)
          const data = response.data.data as { code?: string; codes?: string[] }
          setCodes(data.codes || (data.code ? [data.code] : []))
          return response
        } finally {
          setLoading(false)
        }
      },
      {
        scope: 'reseller.voucher.reveal',
        title: t('Reveal user codes'),
        description: t('Confirm your identity before decrypting stored codes.'),
      }
    )
    setLoading(false)
  }

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={onOpenChange}
        title={t('Reveal user codes')}
        description={t(
          'The quota password and an additional verification method are both required.'
        )}
        contentClassName='sm:max-w-lg'
        footer={
          <>
            <Button
              variant='outline'
              onClick={() => onOpenChange(false)}
              disabled={loading}
            >
              {codes.length ? t('Close') : t('Cancel')}
            </Button>
            {!codes.length && (
              <Button
                onClick={submit}
                disabled={loading || !/^\d{6}$/.test(password)}
              >
                {loading && <Loader2 className='animate-spin' />}
                {t('Continue to verification')}
              </Button>
            )}
          </>
        }
      >
        {codes.length ? (
          <div className='divide-y rounded-md border font-mono text-xs'>
            {codes.map((code) => (
              <div key={code} className='flex items-center gap-2 px-3 py-2'>
                <span className='min-w-0 flex-1 break-all'>{code}</span>
                <CopyButton value={code} tooltip={t('Copy user code')} />
              </div>
            ))}
          </div>
        ) : (
          <div className='space-y-1.5'>
            <Label htmlFor='reseller-reveal-password'>
              {t('Quota password')}
            </Label>
            <Input
              id='reseller-reveal-password'
              type='password'
              inputMode='numeric'
              autoComplete='off'
              maxLength={6}
              value={password}
              onChange={(event) =>
                setPassword(event.target.value.replaceAll(/\D/g, ''))
              }
            />
          </div>
        )}
      </Dialog>
      <SecureVerificationDialog
        open={verification.open}
        onOpenChange={(next) => {
          if (!next) verification.cancel()
        }}
        methods={verification.methods}
        state={verification.state}
        onVerify={async (method, code) => {
          await verification.executeVerification(method, code)
        }}
        onCancel={verification.cancel}
        onCodeChange={verification.setCode}
        onMethodChange={verification.switchMethod}
      />
    </>
  )
}
