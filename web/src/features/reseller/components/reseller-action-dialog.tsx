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
import { AlertTriangle, Loader2, ShieldCheck } from 'lucide-react'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'

import {
  changeQuotaPassword,
  commitTransfer,
  convertCommission,
  issueVouchers,
  newIdempotencyKey,
  previewTransfer,
  resetQuotaPassword,
  setQuotaPassword,
} from '../api'
import type { ResellerSecurityScope } from '../types'

export type ResellerActionKind =
  | 'password-set'
  | 'password-change'
  | 'password-reset'
  | 'transfer'
  | 'convert'
  | 'voucher'

interface ResellerActionDialogProps {
  kind: ResellerActionKind
  open: boolean
  onOpenChange: (open: boolean) => void
  onCompleted: () => void | Promise<void>
}

interface TransferPreview {
  nonce: string
  receiver: { user_id: number; username: string }
  amount: number
  expires_at: number
}

const scopeForKind = (kind: ResellerActionKind): ResellerSecurityScope => {
  if (kind === 'password-reset') return 'reseller.security.password_reset'
  if (kind.startsWith('password')) return 'reseller.security.password'
  if (kind === 'transfer') return 'reseller.transfer'
  if (kind === 'convert') return 'reseller.commission.convert'
  return 'reseller.voucher.issue'
}

export function ResellerActionDialog({
  kind,
  open,
  onOpenChange,
  onCompleted,
}: ResellerActionDialogProps) {
  const { t } = useTranslation()
  const [password, setPassword] = useState('')
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [receivePublicId, setReceivePublicId] = useState('')
  const [amount, setAmount] = useState(1)
  const [count, setCount] = useState(1)
  const [note, setNote] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [unknownResult, setUnknownResult] = useState(false)
  const [preview, setPreview] = useState<TransferPreview | null>(null)
  const [previewProof, setPreviewProof] = useState<string>()
  const [idempotencyKey, setIdempotencyKey] = useState(newIdempotencyKey())
  const [issuedCodes, setIssuedCodes] = useState<string[]>([])

  const verification = useSecureVerification({
    onError: () => setSubmitting(false),
  })

  useEffect(() => {
    if (open) return
    setPassword('')
    setCurrentPassword('')
    setNewPassword('')
    setReceivePublicId('')
    setAmount(1)
    setCount(1)
    setNote('')
    setSubmitting(false)
    setUnknownResult(false)
    setPreview(null)
    setPreviewProof(undefined)
    setIdempotencyKey(newIdempotencyKey())
    setIssuedCodes([])
  }, [open])

  const copy = useMemo(() => {
    switch (kind) {
      case 'password-set':
        return [
          t('Set quota password'),
          t('Create a separate 6-digit password for reseller funds.'),
        ]
      case 'password-change':
        return [
          t('Change quota password'),
          t('Enter the current password before choosing a new one.'),
        ]
      case 'password-reset':
        return [
          t('Reset quota password'),
          t(
            'Outbound transfers and voucher issuance will be frozen for 24 hours.'
          ),
        ]
      case 'transfer':
        return [
          t('Send quota'),
          t('Preview the recipient before committing this transfer.'),
        ]
      case 'convert':
        return [
          t('Convert earnings'),
          t('Move available commission into your own API wallet.'),
        ]
      default:
        return [
          t('Issue user codes'),
          t('Issued quota enters escrow immediately and cannot be refunded.'),
        ]
    }
  }, [kind, t])

  const closeCompleted = async () => {
    toast.success(t('Operation completed'))
    await onCompleted()
    onOpenChange(false)
  }

  const runVerified = async (
    operation: (proof?: string) => Promise<unknown>
  ) => {
    setSubmitting(true)
    await verification.startVerification(
      async (proof) => {
        try {
          const result = await operation(proof)
          await closeCompleted()
          return result
        } finally {
          setSubmitting(false)
        }
      },
      {
        scope: scopeForKind(kind),
        title: copy[0],
        description: t(
          'Confirm your identity to continue this sensitive operation.'
        ),
      }
    )
    setSubmitting(false)
  }

  const submit = async () => {
    setUnknownResult(false)
    if (kind === 'password-set') {
      await runVerified((proof) => setQuotaPassword(password, proof))
      return
    }
    if (kind === 'password-change') {
      await runVerified((proof) =>
        changeQuotaPassword(currentPassword, newPassword, proof)
      )
      return
    }
    if (kind === 'password-reset') {
      await runVerified((proof) => resetQuotaPassword(newPassword, proof))
      return
    }
    if (kind === 'convert') {
      await runVerified((proof) =>
        convertCommission(amount, password, idempotencyKey, proof)
      )
      return
    }
    if (kind === 'voucher') {
      setSubmitting(true)
      await verification.startVerification(
        async (proof) => {
          try {
            const response = await issueVouchers(
              count,
              amount,
              note,
              password,
              idempotencyKey,
              proof
            )
            setIssuedCodes(response.data.data.codes as string[])
            toast.success(t('User codes issued'))
            await onCompleted()
            return response
          } finally {
            setSubmitting(false)
          }
        },
        {
          scope: 'reseller.voucher.issue',
          title: copy[0],
          description: t(
            'Confirm your identity to continue this sensitive operation.'
          ),
        }
      )
      setSubmitting(false)
      return
    }

    setSubmitting(true)
    await verification.startVerification(
      async (proof) => {
        try {
          const response = await previewTransfer(receivePublicId, amount, proof)
          setPreview(response.data.data as TransferPreview)
          setPreviewProof(proof)
          return response
        } finally {
          setSubmitting(false)
        }
      },
      {
        scope: 'reseller.transfer',
        title: t('Verify transfer'),
        description: t(
          'Confirm your identity before previewing the recipient.'
        ),
      }
    )
    setSubmitting(false)
  }

  const commit = async () => {
    if (!preview) return
    setSubmitting(true)
    try {
      await commitTransfer(
        preview.nonce,
        password,
        idempotencyKey,
        previewProof
      )
      await closeCompleted()
    } catch (error: unknown) {
      const status = (error as { response?: { status?: number } })?.response
        ?.status
      if (!status || status >= 500) setUnknownResult(true)
    } finally {
      setSubmitting(false)
    }
  }

  const sixDigits = /^\d{6}$/
  const passwordFormValid =
    kind === 'password-set'
      ? sixDigits.test(password)
      : sixDigits.test(newPassword) &&
        (kind !== 'password-change' || sixDigits.test(currentPassword))
  const fundsFormValid =
    amount >= 1 &&
    amount <= 2000 &&
    sixDigits.test(password) &&
    (kind !== 'transfer' || receivePublicId.trim().length > 0) &&
    (kind !== 'voucher' || (count >= 1 && count <= 50 && note.length <= 255))
  const canSubmit = kind.startsWith('password')
    ? passwordFormValid
    : fundsFormValid
  let primaryAction: ReactNode = null
  if (!issuedCodes.length) {
    primaryAction = preview ? (
      <Button
        onClick={commit}
        disabled={submitting || !sixDigits.test(password)}
      >
        {submitting && <Loader2 className='animate-spin' />}
        {t('Confirm transfer')}
      </Button>
    ) : (
      <Button onClick={submit} disabled={submitting || !canSubmit}>
        {submitting && <Loader2 className='animate-spin' />}
        {kind === 'transfer'
          ? t('Preview transfer')
          : t('Continue to verification')}
      </Button>
    )
  }

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={onOpenChange}
        title={copy[0]}
        description={copy[1]}
        contentClassName='sm:max-w-lg'
        footer={
          <>
            <Button
              variant='outline'
              onClick={() => onOpenChange(false)}
              disabled={submitting}
            >
              {issuedCodes.length ? t('Close') : t('Cancel')}
            </Button>
            {primaryAction}
          </>
        }
      >
        <div className='space-y-4'>
          {issuedCodes.length > 0 && (
            <div className='space-y-2'>
              <Alert>
                <ShieldCheck />
                <AlertTitle>{t('Save these user codes now')}</AlertTitle>
                <AlertDescription>
                  {t(
                    'Codes are hidden from lists. They can only be revealed again after security verification.'
                  )}
                </AlertDescription>
              </Alert>
              <div className='divide-y rounded-md border font-mono text-xs'>
                {issuedCodes.map((code) => (
                  <div key={code} className='flex items-center gap-2 px-3 py-2'>
                    <span className='min-w-0 flex-1 break-all'>{code}</span>
                    <CopyButton value={code} tooltip={t('Copy user code')} />
                  </div>
                ))}
              </div>
            </div>
          )}
          {issuedCodes.length === 0 && (
            <>
              {unknownResult && (
                <Alert>
                  <AlertTriangle />
                  <AlertTitle>{t('Transfer result unknown')}</AlertTitle>
                  <AlertDescription>
                    {t(
                      'Check transfer history before retrying. This request keeps the same idempotency key.'
                    )}
                  </AlertDescription>
                </Alert>
              )}

              {kind === 'password-set' && (
                <PasswordField
                  label={t('New quota password')}
                  value={password}
                  onChange={setPassword}
                />
              )}
              {kind === 'password-change' && (
                <>
                  <PasswordField
                    label={t('Current quota password')}
                    value={currentPassword}
                    onChange={setCurrentPassword}
                  />
                  <PasswordField
                    label={t('New quota password')}
                    value={newPassword}
                    onChange={setNewPassword}
                  />
                </>
              )}
              {kind === 'password-reset' && (
                <>
                  <Alert>
                    <ShieldCheck />
                    <AlertTitle>{t('24-hour outbound freeze')}</AlertTitle>
                    <AlertDescription>
                      {t(
                        'Receiving quota and converting earnings remain available during the freeze.'
                      )}
                    </AlertDescription>
                  </Alert>
                  <PasswordField
                    label={t('New quota password')}
                    value={newPassword}
                    onChange={setNewPassword}
                  />
                </>
              )}

              {!kind.startsWith('password') && (
                <>
                  {kind === 'transfer' && !preview && (
                    <Field label={t('Recipient address')}>
                      <Input
                        value={receivePublicId}
                        onChange={(event) =>
                          setReceivePublicId(event.target.value)
                        }
                        autoComplete='off'
                      />
                    </Field>
                  )}
                  {preview && (
                    <div className='bg-muted/40 grid grid-cols-2 gap-3 rounded-md border p-3 text-sm'>
                      <div>
                        <div className='text-muted-foreground text-xs'>
                          {t('Recipient')}
                        </div>
                        <div className='mt-1 font-medium'>
                          {preview.receiver.username ||
                            preview.receiver.user_id}
                        </div>
                      </div>
                      <div>
                        <div className='text-muted-foreground text-xs'>
                          {t('Amount')}
                        </div>
                        <div className='mt-1 font-medium tabular-nums'>
                          {preview.amount}
                        </div>
                      </div>
                    </div>
                  )}
                  {!preview && (
                    <Field label={t('Amount')}>
                      <Input
                        type='number'
                        min={1}
                        max={2000}
                        value={amount}
                        onChange={(event) =>
                          setAmount(Number(event.target.value))
                        }
                      />
                    </Field>
                  )}
                  {kind === 'voucher' && (
                    <>
                      <Field label={t('Number of codes')}>
                        <Input
                          type='number'
                          min={1}
                          max={50}
                          value={count}
                          onChange={(event) =>
                            setCount(Number(event.target.value))
                          }
                        />
                      </Field>
                      <Field label={t('Batch note')}>
                        <Textarea
                          maxLength={255}
                          value={note}
                          onChange={(event) => setNote(event.target.value)}
                        />
                      </Field>
                    </>
                  )}
                  <PasswordField
                    label={t('Quota password')}
                    value={password}
                    onChange={setPassword}
                  />
                </>
              )}
            </>
          )}
        </div>
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

function Field({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className='space-y-1.5'>
      <Label>{label}</Label>
      {children}
    </div>
  )
}

function PasswordField({
  label,
  value,
  onChange,
}: {
  label: string
  value: string
  onChange: (value: string) => void
}) {
  return (
    <Field label={label}>
      <Input
        type='password'
        inputMode='numeric'
        autoComplete='off'
        maxLength={6}
        value={value}
        onChange={(event) => onChange(event.target.value.replaceAll(/\D/g, ''))}
      />
    </Field>
  )
}
