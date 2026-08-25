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
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { searchUsers } from '@/features/users/api'

import { openResellerCenter } from '../api'
import type { OpenResellerResult } from '../types'

const SEARCH_DEBOUNCE_MS = 300
const SEARCH_PAGE_SIZE = 8

/**
 * Opens the reseller center on someone's behalf.
 *
 * The roster only lists accounts that already run a center, so a reseller who
 * never switched it on cannot be found there — the search here runs against all
 * users instead. Whether the target is already a reseller is not checked before
 * the call: the endpoint is idempotent and reports it back, which costs one
 * round trip instead of a second lookup per keystroke.
 */
export function OpenResellerDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onOpened: (result: OpenResellerResult) => void
}) {
  const { t } = useTranslation()
  const [keyword, setKeyword] = useState('')
  const [debouncedKeyword, setDebouncedKeyword] = useState('')

  useEffect(() => {
    const timer = setTimeout(
      () => setDebouncedKeyword(keyword.trim()),
      SEARCH_DEBOUNCE_MS
    )
    return () => clearTimeout(timer)
  }, [keyword])

  useEffect(() => {
    if (!props.open) {
      setKeyword('')
      setDebouncedKeyword('')
    }
  }, [props.open])

  const searchQuery = useQuery({
    queryKey: ['reseller-admin', 'open-search', debouncedKeyword],
    queryFn: async () => {
      const result = await searchUsers({
        keyword: debouncedKeyword,
        p: 1,
        page_size: SEARCH_PAGE_SIZE,
      })
      if (!result.success) {
        throw new Error(result.message || t('Failed to search users'))
      }
      return result.data?.items ?? []
    },
    enabled: props.open && debouncedKeyword.length > 0,
  })

  const openMutation = useMutation({
    mutationFn: async (userId: number) => {
      const result = await openResellerCenter({ reseller_id: userId })
      if (!result.success) {
        throw new Error(
          result.message || t('Failed to open the reseller center')
        )
      }
      return result.data
    },
    onSuccess: (result) => {
      toast.success(
        result.created
          ? t('Reseller center opened')
          : t('This account already runs a reseller center')
      )
      props.onOpenChange(false)
      props.onOpened(result)
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to open the reseller center')
      ),
  })

  const results = searchQuery.data ?? []

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Open a reseller center')}</DialogTitle>
          <DialogDescription>
            {t(
              'The account becomes a reseller immediately and can then be given direct customers. It gets no customers and no pricing from this action.'
            )}
          </DialogDescription>
        </DialogHeader>

        <Input
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
          placeholder={t('Search by username, display name or email')}
          aria-label={t('Search by username, display name or email')}
        />

        <div className='max-h-72 overflow-y-auto rounded-md border'>
          {debouncedKeyword.length === 0 ? (
            <p className='text-muted-foreground p-4 text-sm'>
              {t('Type to search for an account.')}
            </p>
          ) : null}
          {debouncedKeyword.length > 0 && searchQuery.isPending ? (
            <p className='text-muted-foreground p-4 text-sm'>
              {t('Searching...')}
            </p>
          ) : null}
          {debouncedKeyword.length > 0 &&
          !searchQuery.isPending &&
          results.length === 0 ? (
            <p className='text-muted-foreground p-4 text-sm'>
              {t('No matching account.')}
            </p>
          ) : null}
          <ul>
            {results.map((user) => (
              <li key={user.id} className='border-b last:border-b-0'>
                <div className='flex items-center justify-between gap-3 px-3 py-2'>
                  <div className='min-w-0'>
                    <div className='truncate font-medium'>
                      {user.display_name || user.username}
                    </div>
                    <div className='text-muted-foreground truncate text-xs'>
                      #{user.id} · {user.username}
                    </div>
                  </div>
                  <Button
                    size='sm'
                    disabled={openMutation.isPending}
                    onClick={() => openMutation.mutate(user.id)}
                  >
                    {t('Open')}
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        </div>

        <DialogFooter>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
