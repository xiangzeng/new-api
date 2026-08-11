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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Plus, Wallet } from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import dayjs from '@/lib/dayjs'

import { createBalanceKey, getBalanceKeys, revokeBalanceKey } from '../api'
import type { BalanceKey } from '../types'

const BALANCE_KEYS_QUERY_KEY = ['profile', 'balance-keys'] as const

// Stands in for the real key in the always-visible example. The clear-text key
// is never stored, so the card cannot fill in a key the user already created.
const BALANCE_KEY_PLACEHOLDER = 'obk_YOUR_KEY'

/**
 * The exact command a user can paste into a terminal. The origin comes from the
 * browser so a site behind a custom domain shows its own address rather than a
 * documentation placeholder nobody remembers to replace.
 */
function balanceQueryCommand(key: string) {
  const origin = typeof window === 'undefined' ? '' : window.location.origin
  return `curl -H "Authorization: Bearer ${key}" \\\n  ${origin}/api/open/v1/balance`
}

/**
 * Issues and manages read-only balance keys. A key reads this account's balance
 * and nothing else, so it is what a user puts in their own script or app instead
 * of an API key that can spend quota or an access token that can change the
 * account. The keys are long lived, which is only acceptable because their owner
 * can see when each was last used and revoke it here.
 */
export function BalanceKeysCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [revokeTarget, setRevokeTarget] = useState<BalanceKey | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [newKeyName, setNewKeyName] = useState('')
  const [issuedKey, setIssuedKey] = useState<string | null>(null)

  const keysQuery = useQuery({
    queryKey: BALANCE_KEYS_QUERY_KEY,
    queryFn: async () => {
      const result = await getBalanceKeys()
      if (!result.success) {
        throw new Error(result.message || t('Failed to load balance keys'))
      }
      return result.data ?? []
    },
  })

  const createMutation = useMutation({
    mutationFn: async (name: string) => {
      const result = await createBalanceKey(name)
      if (!result.success || !result.data) {
        throw new Error(result.message || t('Failed to create balance key'))
      }
      return result.data
    },
    onSuccess: async (created) => {
      setIssuedKey(created.key)
      setNewKeyName('')
      await queryClient.invalidateQueries({ queryKey: BALANCE_KEYS_QUERY_KEY })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to create balance key')
      )
    },
  })

  const revokeMutation = useMutation({
    mutationFn: async (key: BalanceKey) => {
      const result = await revokeBalanceKey(key.id)
      if (!result.success) {
        throw new Error(result.message || t('Failed to revoke balance key'))
      }
    },
    onSuccess: async () => {
      toast.success(t('Balance key revoked'))
      setRevokeTarget(null)
      await queryClient.invalidateQueries({ queryKey: BALANCE_KEYS_QUERY_KEY })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to revoke balance key')
      )
    },
  })

  const formatTimestamp = useCallback(
    (timestamp: number) =>
      timestamp > 0
        ? dayjs.unix(timestamp).format('YYYY-MM-DD HH:mm')
        : t('Never'),
    [t]
  )

  const closeCreateDialog = (open: boolean) => {
    if (createMutation.isPending) return
    if (!open) {
      setIssuedKey(null)
      setNewKeyName('')
    }
    setCreateOpen(open)
  }

  const keys = keysQuery.data ?? []

  return (
    <>
      <Card>
        <CardHeader>
          <div className='flex items-center gap-3'>
            <IconBadge>
              <Wallet aria-hidden='true' />
            </IconBadge>
            <div className='min-w-0'>
              <CardTitle>{t('Balance keys')}</CardTitle>
              <CardDescription>
                {t(
                  'Read your own balance from your own program. A balance key can only read the balance — it cannot call models, spend quota, or change your account.'
                )}
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent className='space-y-3'>
          {keysQuery.isLoading ? (
            <>
              <Skeleton className='h-14 w-full' />
              <Skeleton className='h-14 w-full' />
            </>
          ) : (
            keys.map((key) => (
              <div
                key={key.id}
                className='flex items-center justify-between gap-3 rounded-md border px-3 py-2.5'
              >
                <div className='min-w-0'>
                  <div className='truncate text-sm font-medium'>{key.name}</div>
                  <div className='text-muted-foreground truncate font-mono text-xs'>
                    {key.token_hint}
                  </div>
                  <div className='text-muted-foreground truncate text-xs'>
                    {t('Created {{created}} · Last used {{used}}', {
                      created: formatTimestamp(key.created_time),
                      used: formatTimestamp(key.last_used_time),
                    })}
                  </div>
                </div>
                <Button
                  variant='outline'
                  size='sm'
                  disabled={revokeMutation.isPending}
                  onClick={() => setRevokeTarget(key)}
                >
                  {t('Revoke')}
                </Button>
              </div>
            ))
          )}
          <Button
            variant='outline'
            className='w-full gap-2'
            onClick={() => setCreateOpen(true)}
          >
            <Plus className='h-4 w-4' aria-hidden='true' />
            {t('Create balance key')}
          </Button>

          <div className='space-y-2 rounded-md border border-dashed p-3'>
            <div className='flex items-center justify-between gap-2'>
              <span className='text-sm font-medium'>{t('How to use')}</span>
              <CopyButton
                value={balanceQueryCommand(BALANCE_KEY_PLACEHOLDER)}
                variant='ghost'
                className='size-8'
                iconClassName='size-4'
                tooltip={t('Copy command')}
                aria-label={t('Copy command')}
              />
            </div>
            <pre className='text-muted-foreground overflow-x-auto font-mono text-xs'>
              {balanceQueryCommand(BALANCE_KEY_PLACEHOLDER)}
            </pre>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Replace obk_YOUR_KEY with your own key. The response carries quota and used_quota in raw units, which never change with the site display setting, plus balance and used converted to the site currency.'
              )}
            </p>
          </div>
        </CardContent>
      </Card>

      <Dialog
        open={createOpen}
        onOpenChange={closeCreateDialog}
        title={issuedKey ? t('Balance key created') : t('Create balance key')}
        description={
          issuedKey
            ? t(
                'Copy it now. Only a digest is stored, so it cannot be shown again.'
              )
            : t('Name it after the program that will use it.')
        }
        contentClassName='sm:max-w-md'
        contentHeight='auto'
        bodyClassName='space-y-4'
        footer={
          issuedKey ? (
            <Button type='button' onClick={() => closeCreateDialog(false)}>
              {t('Done')}
            </Button>
          ) : (
            <>
              <Button
                type='button'
                variant='outline'
                onClick={() => closeCreateDialog(false)}
                disabled={createMutation.isPending}
              >
                {t('Cancel')}
              </Button>
              <Button
                type='button'
                className='gap-2'
                disabled={createMutation.isPending}
                onClick={() => createMutation.mutate(newKeyName)}
              >
                {createMutation.isPending ? (
                  <Loader2
                    className='h-4 w-4 animate-spin'
                    aria-hidden='true'
                  />
                ) : null}
                {t('Create')}
              </Button>
            </>
          )
        }
      >
        <div className='my-4 space-y-2'>
          {issuedKey ? (
            <>
              <Label htmlFor='balance-key'>{t('Balance key')}</Label>
              <div className='flex gap-2'>
                <Input
                  id='balance-key'
                  type='text'
                  value={issuedKey}
                  readOnly
                  className='font-mono text-xs'
                />
                <CopyButton
                  value={issuedKey}
                  variant='outline'
                  className='size-9'
                  iconClassName='size-4'
                  tooltip={t('Copy key')}
                  aria-label={t('Copy key')}
                />
              </div>
              <div className='flex items-center justify-between gap-2 pt-2'>
                <span className='text-sm font-medium'>{t('How to use')}</span>
                <CopyButton
                  value={balanceQueryCommand(issuedKey)}
                  variant='ghost'
                  className='size-8'
                  iconClassName='size-4'
                  tooltip={t('Copy command')}
                  aria-label={t('Copy command')}
                />
              </div>
              <pre className='text-muted-foreground overflow-x-auto font-mono text-xs'>
                {balanceQueryCommand(issuedKey)}
              </pre>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'This command already carries the key above, so it runs as is. The same example, with a placeholder, stays on the card after you close this dialog.'
                )}
              </p>
            </>
          ) : (
            <>
              <Label htmlFor='balance-key-name'>{t('Name')}</Label>
              <Input
                id='balance-key-name'
                value={newKeyName}
                autoFocus
                placeholder={t('Phone widget')}
                onChange={(event) => setNewKeyName(event.target.value)}
              />
            </>
          )}
        </div>
      </Dialog>

      <AlertDialog
        open={revokeTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRevokeTarget(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Revoke balance key')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                '{{name}} will stop working immediately. Any program still using it will fail to read your balance.',
                { name: revokeTarget?.name ?? '' }
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={revokeMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={revokeMutation.isPending}
              onClick={() =>
                revokeTarget && revokeMutation.mutate(revokeTarget)
              }
            >
              {revokeMutation.isPending ? (
                <Loader2 className='animate-spin' />
              ) : null}
              {t('Revoke')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
