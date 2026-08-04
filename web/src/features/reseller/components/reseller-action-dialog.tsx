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
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { formatQuota } from '@/lib/format'

import {
  changeQuotaPassword,
  commitTransfer,
  convertCommission,
  issueVouchers,
  newIdempotencyKey,
  previewTransfer,
  resetQuotaPassword,
  rotateReceiveAddress,
  setQuotaPassword as createQuotaPassword,
} from '../api'
import type { ResellerTransferPreview } from '../types'

export type ResellerActionKind =
  | 'password-set'
  | 'password-change'
  | 'password-reset'
  | 'rotate'
  | 'transfer'
  | 'convert'
  | 'voucher-single'
  | 'voucher-batch'

interface ResellerActionDialogProps {
  kind: ResellerActionKind
  open: boolean
  onOpenChange: (open: boolean) => void
  onCompleted: () => void | Promise<void>
  availableCommissionQuota?: number
  initialRecipient?: string
}

export function ResellerActionDialog({
  kind,
  open,
  onOpenChange,
  onCompleted,
  availableCommissionQuota = 0,
  initialRecipient = '',
}: ResellerActionDialogProps) {
  const { t } = useTranslation()
  const [quotaPassword, setQuotaPassword] = useState('')
  const [loginPassword, setLoginPassword] = useState('')
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [recipient, setRecipient] = useState(initialRecipient)
  const [amount, setAmount] = useState(1)
  const [count, setCount] = useState(kind === 'voucher-batch' ? 2 : 1)
  const [note, setNote] = useState('')
  const [confirmed, setConfirmed] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [unknownResult, setUnknownResult] = useState(false)
  const [preview, setPreview] = useState<ResellerTransferPreview | null>(null)
  const [idempotencyKey, setIdempotencyKey] = useState(newIdempotencyKey())
  const [issuedCodes, setIssuedCodes] = useState<string[]>([])
  const verification = useSecureVerification({
    onError: () => setSubmitting(false),
  })

  useEffect(() => {
    if (open) return
    setQuotaPassword('')
    setLoginPassword('')
    setCurrentPassword('')
    setNewPassword('')
    setRecipient('')
    setAmount(1)
    setCount(kind === 'voucher-batch' ? 2 : 1)
    setNote('')
    setConfirmed(false)
    setSubmitting(false)
    setUnknownResult(false)
    setPreview(null)
    setIdempotencyKey(newIdempotencyKey())
    setIssuedCodes([])
  }, [kind, open])

  useEffect(() => {
    if (open && kind === 'transfer' && initialRecipient && !preview) {
      setRecipient(initialRecipient)
    }
  }, [initialRecipient, kind, open, preview])

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
      case 'rotate':
        return [
          t('Rotate receive address'),
          t('The old receive address will stop accepting new previews.'),
        ]
      case 'transfer':
        return [
          t('Send quota'),
          t('Preview the recipient before committing this transfer.'),
        ]
      case 'convert':
        return [
          t('Convert all available earnings'),
          t('Move all available commission into your own API wallet.'),
        ]
      case 'voucher-batch':
        return [
          t('Batch issue user codes'),
          t('Issued quota enters escrow immediately and cannot be refunded.'),
        ]
      default:
        return [
          t('Issue one user code'),
          t('Issued quota enters escrow immediately and cannot be refunded.'),
        ]
    }
  }, [kind, t])

  const closeCompleted = async () => {
    toast.success(t('Operation completed'))
    await onCompleted()
    onOpenChange(false)
  }

  const runBootstrap = async (
    scope: 'reseller.security.password' | 'reseller.security.password_reset',
    operation: (proof?: string) => Promise<unknown>
  ) => {
    setSubmitting(true)
    if (loginPassword.trim()) {
      try {
        await operation()
        await closeCompleted()
      } finally {
        setSubmitting(false)
      }
      return
    }
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
        scope,
        title: copy[0],
        description: t(
          'Use an established security method when no login password is available.'
        ),
      }
    )
    setSubmitting(false)
  }

  const submit = async () => {
    setUnknownResult(false)
    if (kind === 'password-set') {
      await runBootstrap('reseller.security.password', (proof) =>
        createQuotaPassword(quotaPassword, loginPassword, proof)
      )
      return
    }
    if (kind === 'password-reset') {
      await runBootstrap('reseller.security.password_reset', (proof) =>
        resetQuotaPassword(newPassword, loginPassword, proof)
      )
      return
    }
    if (kind === 'transfer') {
      setSubmitting(true)
      try {
        const response = await previewTransfer(recipient, amount)
        setPreview(response.data.data as ResellerTransferPreview)
      } finally {
        setSubmitting(false)
      }
      return
    }

    setSubmitting(true)
    try {
      if (kind === 'password-change') {
        await changeQuotaPassword(currentPassword, newPassword)
        await closeCompleted()
        return
      }
      if (kind === 'rotate') {
        await rotateReceiveAddress(quotaPassword)
        await closeCompleted()
        return
      }
      if (kind === 'convert') {
        await convertCommission(
          availableCommissionQuota,
          quotaPassword,
          idempotencyKey
        )
        await closeCompleted()
        return
      }
      const response = await issueVouchers(
        kind === 'voucher-single' ? 1 : count,
        amount,
        note,
        quotaPassword,
        idempotencyKey
      )
      setIssuedCodes(response.data.data.codes as string[])
      toast.success(t('User codes issued'))
      await onCompleted()
    } catch (error: unknown) {
      const status = (error as { response?: { status?: number } })?.response
        ?.status
      if (!status || status >= 500) setUnknownResult(true)
    } finally {
      setSubmitting(false)
    }
  }

  const commit = async () => {
    if (!preview) return
    setSubmitting(true)
    try {
      await commitTransfer(preview, quotaPassword, idempotencyKey)
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
      ? sixDigits.test(quotaPassword)
      : sixDigits.test(newPassword) &&
        (kind !== 'password-change' || sixDigits.test(currentPassword))
  const operationFormValid =
    sixDigits.test(quotaPassword) &&
    (kind !== 'convert' || availableCommissionQuota > 0) &&
    (!kind.startsWith('voucher') ||
      (amount >= 1 &&
        amount <= 2000 &&
        count >= 1 &&
        count <= 50 &&
        note.length <= 255))
  const previewFormValid =
    recipient.trim().length > 0 && amount >= 1 && amount <= 2000
  let formValid = operationFormValid
  if (kind.startsWith('password')) {
    formValid = passwordFormValid
  } else if (kind === 'transfer') {
    formValid = previewFormValid
  }
  let primaryAction: ReactNode = null
  if (!issuedCodes.length) {
    primaryAction = preview ? (
      <Button
        onClick={commit}
        disabled={submitting || !confirmed || !sixDigits.test(quotaPassword)}
      >
        {submitting && <Loader2 className='animate-spin' />}
        {t('Confirm transfer')}
      </Button>
    ) : (
      <Button onClick={submit} disabled={submitting || !formValid}>
        {submitting && <Loader2 className='animate-spin' />}
        {kind === 'transfer' ? t('Preview transfer') : t('Confirm')}
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
          {issuedCodes.length > 0 ? (
            <IssuedCodes codes={issuedCodes} />
          ) : (
            <>
              {unknownResult && (
                <Alert>
                  <AlertTriangle />
                  <AlertTitle>{t('Operation result unknown')}</AlertTitle>
                  <AlertDescription>
                    {t(
                      'Check the relevant history before retrying. This request keeps the same idempotency key.'
                    )}
                  </AlertDescription>
                </Alert>
              )}
              {kind === 'password-set' && (
                <>
                  <PasswordField
                    label={t('New quota password')}
                    value={quotaPassword}
                    onChange={setQuotaPassword}
                  />
                  <LoginPasswordField
                    value={loginPassword}
                    onChange={setLoginPassword}
                  />
                </>
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
                  <LoginPasswordField
                    value={loginPassword}
                    onChange={setLoginPassword}
                  />
                </>
              )}
              {kind === 'transfer' && !preview && (
                <>
                  <Field label={t('Recipient')}>
                    <Input
                      value={recipient}
                      onChange={(event) => setRecipient(event.target.value)}
                      placeholder={t(
                        'Username, 32-character code, or receive link'
                      )}
                      autoComplete='off'
                    />
                  </Field>
                  <AmountField amount={amount} onChange={setAmount} />
                </>
              )}
              {kind === 'transfer' && preview && (
                <>
                  <div className='bg-muted/40 grid grid-cols-2 gap-3 rounded-md border p-3 text-sm'>
                    <div>
                      <div className='text-muted-foreground text-xs'>
                        {t('Recipient')}
                      </div>
                      <div className='mt-1 font-medium'>
                        {preview.recipient_username}
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
                  <label className='flex items-start gap-2 text-sm'>
                    <Checkbox
                      checked={confirmed}
                      onCheckedChange={(checked) =>
                        setConfirmed(checked === true)
                      }
                    />
                    <span>
                      {t(
                        'I confirmed the exact recipient and amount. This transfer cannot be reversed.'
                      )}
                    </span>
                  </label>
                  <PasswordField
                    label={t('Quota password')}
                    value={quotaPassword}
                    onChange={setQuotaPassword}
                  />
                </>
              )}
              {kind === 'convert' && (
                <>
                  <div className='rounded-md border p-3'>
                    <div className='text-muted-foreground text-xs'>
                      {t('Available earnings to convert')}
                    </div>
                    <div className='mt-1 text-lg font-semibold tabular-nums'>
                      {formatQuota(availableCommissionQuota)}
                    </div>
                  </div>
                  <PasswordField
                    label={t('Quota password')}
                    value={quotaPassword}
                    onChange={setQuotaPassword}
                  />
                </>
              )}
              {kind === 'rotate' && (
                <PasswordField
                  label={t('Quota password')}
                  value={quotaPassword}
                  onChange={setQuotaPassword}
                />
              )}
              {kind.startsWith('voucher') && (
                <>
                  <AmountField amount={amount} onChange={setAmount} />
                  {kind === 'voucher-batch' && (
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
                    value={quotaPassword}
                    onChange={setQuotaPassword}
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

function IssuedCodes({ codes }: { codes: string[] }) {
  const { t } = useTranslation()
  return (
    <div className='space-y-2'>
      <Alert>
        <ShieldCheck />
        <AlertTitle>{t('Save these user codes now')}</AlertTitle>
        <AlertDescription>
          {t(
            'Codes are hidden from lists and require the quota password to reveal again.'
          )}
        </AlertDescription>
      </Alert>
      <div className='divide-y rounded-md border font-mono text-xs'>
        {codes.map((code) => (
          <div key={code} className='flex items-center gap-2 px-3 py-2'>
            <span className='min-w-0 flex-1 break-all'>{code}</span>
            <CopyButton value={code} tooltip={t('Copy user code')} />
          </div>
        ))}
      </div>
    </div>
  )
}

function AmountField({
  amount,
  onChange,
}: {
  amount: number
  onChange: (value: number) => void
}) {
  const { t } = useTranslation()
  return (
    <Field label={t('Amount')}>
      <Input
        type='number'
        min={1}
        max={2000}
        value={amount}
        onChange={(event) => onChange(Number(event.target.value))}
      />
    </Field>
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

function LoginPasswordField({
  value,
  onChange,
}: {
  value: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  return (
    <Field label={t('Login password')}>
      <Input
        type='password'
        autoComplete='current-password'
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
      <p className='text-muted-foreground text-xs'>
        {t('Leave blank to use an established security verification method.')}
      </p>
    </Field>
  )
}
