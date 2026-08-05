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
import dayjs from 'dayjs'
import { Store, Loader2, Unlink } from 'lucide-react'
import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'

import {
  getUserResellerBinding,
  bindUserToReseller,
  unbindUserFromReseller,
} from '../../api'
import type { UserResellerBinding } from '../../types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number | null
  onSuccess?: () => void
}

const REGISTRATION_SOURCE_LABELS: Record<string, string> = {
  primary: 'Primary site',
  reseller: 'Reseller invitation',
  admin: 'Admin binding',
  legacy_unknown: 'Legacy',
}

const RESELLER_ERROR_MESSAGES: Record<string, string> = {
  RESELLER_CUSTOMER_BOUND:
    'This user already belongs to another reseller. Unbind it first.',
  RESELLER_SELF_BINDING: 'A reseller cannot be bound as its own customer',
  RESELLER_FORBIDDEN: 'The selected reseller is disabled or frozen',
  RESELLER_NOT_FOUND: 'No matching reseller or user was found',
}

function resellerErrorKey(error: unknown): string {
  const response = (
    error as {
      response?: { data?: { message?: string; data?: { code?: string } } }
    }
  )?.response
  const code = response?.data?.data?.code
  if (code && RESELLER_ERROR_MESSAGES[code]) {
    return RESELLER_ERROR_MESSAGES[code]
  }
  return response?.data?.message || 'Request failed'
}

export function UserResellerBindingDialog(props: Props) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [unbindOpen, setUnbindOpen] = useState(false)
  const [binding, setBinding] = useState<UserResellerBinding | null>(null)
  const [resellerInput, setResellerInput] = useState('')

  const fetchBinding = useCallback(async () => {
    if (!props.userId) return
    setLoading(true)
    try {
      const res = await getUserResellerBinding(props.userId)
      setBinding(res.data ?? null)
    } catch (error) {
      toast.error(t(resellerErrorKey(error)))
    } finally {
      setLoading(false)
    }
  }, [props.userId, t])

  useEffect(() => {
    if (props.open) {
      setResellerInput('')
      void fetchBinding()
    }
  }, [props.open, fetchBinding])

  const handleBind = async () => {
    const identity = resellerInput.trim()
    if (!props.userId || !identity) return
    const payload = /^\d+$/.test(identity)
      ? { reseller_id: Number(identity) }
      : { reseller_username: identity }
    setSaving(true)
    try {
      await bindUserToReseller(props.userId, payload)
      toast.success(t('Customer bound to the reseller'))
      setResellerInput('')
      await fetchBinding()
      props.onSuccess?.()
    } catch (error) {
      toast.error(t(resellerErrorKey(error)))
    } finally {
      setSaving(false)
    }
  }

  const handleUnbind = async () => {
    if (!props.userId) return
    setSaving(true)
    try {
      await unbindUserFromReseller(props.userId)
      toast.success(t('Customer released from the reseller'))
      await fetchBinding()
      props.onSuccess?.()
    } catch (error) {
      toast.error(t(resellerErrorKey(error)))
    } finally {
      setSaving(false)
      setUnbindOpen(false)
    }
  }

  return (
    <>
      <Dialog
        open={props.open}
        onOpenChange={props.onOpenChange}
        title={
          <>
            <Store className='h-5 w-5' />
            {t('Reseller Binding')}
          </>
        }
        description={t('Manage the direct reseller this user belongs to')}
        contentClassName='sm:max-w-lg'
        titleClassName='flex items-center gap-2'
        contentHeight='auto'
        bodyClassName='space-y-4'
      >
        {loading || !binding ? (
          <div className='flex items-center justify-center py-8'>
            <Loader2 className='text-muted-foreground h-6 w-6 animate-spin' />
          </div>
        ) : (
          <div className='space-y-4'>
            <p className='text-muted-foreground text-sm'>
              {binding.customer_username} (ID: {binding.customer_id})
            </p>

            {binding.is_reseller && (
              <p className='text-muted-foreground text-xs'>
                {t(
                  'This user runs a reseller center with {{count}} direct customers',
                  { count: binding.own_customer_count }
                )}
              </p>
            )}

            <Separator />

            {binding.bound ? (
              <div className='space-y-3'>
                <div className='flex items-center justify-between gap-2'>
                  <div className='min-w-0'>
                    <div className='flex items-center gap-1.5'>
                      <span className='text-sm font-medium'>
                        {binding.reseller_username}
                      </span>
                      <StatusBadge
                        variant='neutral'
                        label={t(
                          REGISTRATION_SOURCE_LABELS[
                            binding.registration_source
                          ] || binding.registration_source
                        )}
                        copyable={false}
                        size='sm'
                      />
                    </div>
                    <p className='text-muted-foreground text-xs'>
                      {t('Reseller ID')}: {binding.reseller_id} ·{' '}
                      {t('Bound at')}:{' '}
                      {dayjs.unix(binding.bound_at).format('YYYY-MM-DD HH:mm')}
                    </p>
                  </div>
                  <Button
                    variant='outline'
                    size='sm'
                    className='text-destructive hover:text-destructive shrink-0 gap-1.5'
                    onClick={() => setUnbindOpen(true)}
                  >
                    <Unlink className='h-3.5 w-3.5' />
                    {t('Unbind')}
                  </Button>
                </div>
                <p className='text-muted-foreground text-xs'>
                  {t('Current price multiplier')}:{' '}
                  {(binding.current_multiplier_bps / 10000).toFixed(4)}x
                </p>
              </div>
            ) : (
              <div className='space-y-2'>
                <Label htmlFor='reseller-identity'>
                  {t('Reseller username or ID')}
                </Label>
                <div className='flex items-center gap-2'>
                  <Input
                    id='reseller-identity'
                    value={resellerInput}
                    onChange={(event) => setResellerInput(event.target.value)}
                    placeholder={t('Enter the reseller username or user ID')}
                  />
                  <Button
                    onClick={handleBind}
                    disabled={saving || !resellerInput.trim()}
                  >
                    {saving && <Loader2 className='h-3.5 w-3.5 animate-spin' />}
                    {t('Bind')}
                  </Button>
                </div>
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'The reseller center is opened automatically if the target user has not opened it yet.'
                  )}
                </p>
              </div>
            )}
          </div>
        )}
      </Dialog>

      <ConfirmDialog
        open={unbindOpen}
        onOpenChange={setUnbindOpen}
        title={t('Confirm Unbind')}
        desc={t(
          'Releasing this customer also deletes its customer-level price multiplier. Earnings already recorded for the reseller are kept.'
        )}
        confirmText={t('Unbind')}
        destructive
        handleConfirm={handleUnbind}
        isLoading={saving}
      />
    </>
  )
}
