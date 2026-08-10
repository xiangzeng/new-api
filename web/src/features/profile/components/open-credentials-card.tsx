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
import { Loader2, Plug } from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { Skeleton } from '@/components/ui/skeleton'
import dayjs from '@/lib/dayjs'

import { getOpenCredentials, revokeOpenCredential } from '../api'
import type { OpenCredentialGrant } from '../types'

const OPEN_CREDENTIALS_QUERY_KEY = ['profile', 'open-credentials'] as const

/**
 * Lists the third-party sites this user let query their balance, and lets them
 * end any of those grants. The credentials are long lived, so this card is what
 * keeps that acceptable: the person who granted access can always see and
 * revoke it.
 */
export function OpenCredentialsCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [revokeTarget, setRevokeTarget] = useState<OpenCredentialGrant | null>(
    null
  )

  const grantsQuery = useQuery({
    queryKey: OPEN_CREDENTIALS_QUERY_KEY,
    queryFn: async () => {
      const result = await getOpenCredentials()
      if (!result.success) {
        throw new Error(result.message || t('Failed to load authorizations'))
      }
      return result.data ?? []
    },
  })

  const revokeMutation = useMutation({
    mutationFn: async (grant: OpenCredentialGrant) => {
      const result = await revokeOpenCredential(grant.id)
      if (!result.success) {
        throw new Error(result.message || t('Failed to revoke authorization'))
      }
    },
    onSuccess: async () => {
      toast.success(t('Authorization revoked'))
      setRevokeTarget(null)
      await queryClient.invalidateQueries({
        queryKey: OPEN_CREDENTIALS_QUERY_KEY,
      })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to revoke authorization')
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

  const grants = grantsQuery.data ?? []

  // A user with no third-party grants has nothing to manage here; hiding the
  // card keeps the profile page from growing an empty section for everyone.
  if (!grantsQuery.isLoading && grants.length === 0) {
    return null
  }

  return (
    <>
      <Card>
        <CardHeader>
          <div className='flex items-center gap-3'>
            <IconBadge>
              <Plug aria-hidden='true' />
            </IconBadge>
            <div className='min-w-0'>
              <CardTitle>{t('Third-party balance access')}</CardTitle>
              <CardDescription>
                {t(
                  'Sites you allowed to read your balance. They cannot spend quota or change your account.'
                )}
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent className='space-y-3'>
          {grantsQuery.isLoading ? (
            <>
              <Skeleton className='h-14 w-full' />
              <Skeleton className='h-14 w-full' />
            </>
          ) : (
            grants.map((grant) => (
              <div
                key={grant.id}
                className='flex items-center justify-between gap-3 rounded-md border px-3 py-2.5'
              >
                <div className='min-w-0'>
                  <div className='truncate text-sm font-medium'>
                    {grant.app_name || grant.app_id}
                  </div>
                  <div className='text-muted-foreground truncate text-xs'>
                    {t('Granted {{granted}} · Last used {{used}}', {
                      granted: formatTimestamp(grant.created_time),
                      used: formatTimestamp(grant.last_used_time),
                    })}
                  </div>
                </div>
                <Button
                  variant='outline'
                  size='sm'
                  disabled={revokeMutation.isPending}
                  onClick={() => setRevokeTarget(grant)}
                >
                  {t('Revoke')}
                </Button>
              </div>
            ))
          )}
        </CardContent>
      </Card>

      <AlertDialog
        open={revokeTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRevokeTarget(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Revoke authorization')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                '{{name}} will no longer be able to read your balance. You can authorize it again from their site.',
                { name: revokeTarget?.app_name || revokeTarget?.app_id || '' }
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
