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
import { useQuery } from '@tanstack/react-query'
import { Loader2, Plus, Search } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

import { searchUsers } from '../../users/api'
import type { CustomPricingUserTarget } from './custom-pricing-columns'

const SEARCH_DEBOUNCE_MS = 250

interface AddCustomPricingUserDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Users that already have custom pricing enabled — filtered out of results */
  enabledUserIds: Set<number>
  onEnable: (user: CustomPricingUserTarget) => void
  enablePending: boolean
}

export function AddCustomPricingUserDialog(
  props: AddCustomPricingUserDialogProps
) {
  const { t } = useTranslation()
  const [searchTerm, setSearchTerm] = useState('')
  const [debouncedSearchTerm, setDebouncedSearchTerm] = useState('')
  const [selectedUser, setSelectedUser] =
    useState<CustomPricingUserTarget | null>(null)

  useEffect(() => {
    const timeout = setTimeout(() => {
      setDebouncedSearchTerm(searchTerm.trim())
    }, SEARCH_DEBOUNCE_MS)
    return () => clearTimeout(timeout)
  }, [searchTerm])

  useEffect(() => {
    if (!props.open) {
      setSearchTerm('')
      setDebouncedSearchTerm('')
      setSelectedUser(null)
    }
  }, [props.open])

  const searchUsersQuery = useQuery({
    queryKey: ['custom-pricing', 'search', debouncedSearchTerm],
    enabled: props.open && debouncedSearchTerm.length > 0,
    queryFn: async () => {
      const result = await searchUsers({
        keyword: debouncedSearchTerm,
        page_size: 20,
      })
      if (!result.success) {
        throw new Error(result.message || t('Failed to search users'))
      }
      return result.data?.items ?? []
    },
  })

  const searchResults = (searchUsersQuery.data ?? []).filter(
    (user) => !props.enabledUserIds.has(user.id)
  )

  let searchContent = (
    <div className='space-y-1'>
      {searchResults.map((user) => {
        const selected = selectedUser?.id === user.id
        return (
          <button
            key={user.id}
            type='button'
            className={cn(
              'hover:bg-muted focus-visible:border-ring focus-visible:ring-ring/50 flex w-full items-center justify-between gap-3 rounded-md border border-transparent px-2.5 py-2 text-left outline-none focus-visible:ring-3',
              selected && 'border-primary bg-primary/5'
            )}
            onClick={() =>
              setSelectedUser({
                id: user.id,
                username: user.username,
                display_name: user.display_name,
              })
            }
          >
            <span className='min-w-0'>
              <span className='block truncate text-sm font-medium'>
                {user.display_name || user.username}
              </span>
              <span className='text-muted-foreground block truncate text-xs'>
                {user.username} · ID {user.id}
              </span>
            </span>
            <StatusBadge
              label={user.group}
              autoColor={user.group}
              copyable={false}
            />
          </button>
        )
      })}
    </div>
  )
  if (searchUsersQuery.isFetching) {
    searchContent = <LoadingState inline message={t('Searching...')} />
  } else if (debouncedSearchTerm.length === 0) {
    searchContent = (
      <div className='text-muted-foreground flex min-h-[200px] items-center justify-center text-sm'>
        {t('Search users to add custom pricing')}
      </div>
    )
  } else if (searchUsersQuery.isError) {
    searchContent = (
      <div className='text-destructive flex min-h-[200px] items-center justify-center text-sm'>
        {t('Failed to search users')}
      </div>
    )
  } else if (searchResults.length === 0) {
    searchContent = (
      <div className='text-muted-foreground flex min-h-[200px] items-center justify-center text-sm'>
        {t('No matching users')}
      </div>
    )
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={
        <span className='flex items-center gap-2'>
          <Search className='size-5' />
          {t('Add Custom Pricing User')}
        </span>
      }
      description={t('Select a user to enable custom pricing.')}
      contentClassName='sm:max-w-xl'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={props.enablePending}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={() => selectedUser && props.onEnable(selectedUser)}
            disabled={!selectedUser || props.enablePending}
          >
            {props.enablePending ? (
              <Loader2 className='animate-spin' />
            ) : (
              <Plus />
            )}
            {t('Enable')}
          </Button>
        </>
      }
    >
      <div className='space-y-3'>
        <Input
          value={searchTerm}
          onChange={(event) => {
            setSearchTerm(event.target.value)
            setSelectedUser(null)
          }}
          placeholder={t('Search by ID, username, display name, or email')}
        />

        <div className='border-border bg-muted/20 min-h-[220px] rounded-lg border p-2'>
          {searchContent}
        </div>
      </div>
    </Dialog>
  )
}
