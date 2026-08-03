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
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { useDebounce } from '@/hooks'
import { formatQuota } from '@/lib/format'

import { getInvitationInvitees } from '../api'
import type { InvitationSummary, InvitationTimeRange } from '../types'
import { InvitationUserCell, PeriodUsageCell } from './invitation-cells'

const INVITEES_PAGE_SIZE = 10
const SEARCH_DEBOUNCE_MS = 400

/** Mirrors the backend user status codes (`model.User.Status`). */
const INVITEE_STATUS_DISABLED = 2

interface InviteesSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  inviter: InvitationSummary | null
  timeRange: InvitationTimeRange
}

export function InviteesSheet(props: InviteesSheetProps) {
  const { t } = useTranslation()
  const [keyword, setKeyword] = useState('')
  const [page, setPage] = useState(1)
  const debouncedKeyword = useDebounce(keyword, SEARCH_DEBOUNCE_MS)
  const inviterId = props.inviter?.inviter_id ?? 0

  // A different inviter (or a new search term) always restarts at page one.
  useEffect(() => {
    setPage(1)
  }, [inviterId, debouncedKeyword])

  useEffect(() => {
    if (!props.open) {
      setKeyword('')
    }
  }, [props.open])

  const inviteesQuery = useQuery({
    queryKey: [
      'invitations',
      'invitees',
      inviterId,
      page,
      debouncedKeyword,
      props.timeRange.start_timestamp,
      props.timeRange.end_timestamp,
    ],
    enabled: props.open && inviterId > 0,
    queryFn: async () => {
      const result = await getInvitationInvitees(inviterId, {
        p: page,
        page_size: INVITEES_PAGE_SIZE,
        keyword: debouncedKeyword.trim(),
        start_timestamp: props.timeRange.start_timestamp,
        end_timestamp: props.timeRange.end_timestamp,
      })
      if (!result.success) {
        throw new Error(result.message || t('Failed to load invitees'))
      }
      return {
        items: result.data?.items ?? [],
        total: result.data?.total ?? 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const invitees = inviteesQuery.data?.items ?? []
  const total = inviteesQuery.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / INVITEES_PAGE_SIZE))

  let emptyContent = t('No invitees')
  if (inviteesQuery.isLoading) {
    emptyContent = t('Loading...')
  } else if (inviteesQuery.isError) {
    emptyContent = t('Failed to load invitees')
  }

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-3xl')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Invitees')}</SheetTitle>
          <SheetDescription>
            {props.inviter?.inviter_username || '-'} (ID:{' '}
            {props.inviter?.inviter_id ?? '-'})
          </SheetDescription>
        </SheetHeader>

        <div className={sideDrawerFormClassName()}>
          <Input
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder={t('Search by ID, username, display name, or email')}
          />

          <StaticDataTable
            data={inviteesQuery.isLoading ? [] : invitees}
            getRowKey={(invitee) => invitee.invitee_id}
            emptyClassName='text-muted-foreground py-8'
            emptyContent={emptyContent}
            columns={[
              {
                id: 'invitee',
                header: t('Invitee'),
                cell: (invitee) => (
                  <InvitationUserCell
                    id={invitee.invitee_id}
                    username={invitee.username}
                    displayName={invitee.display_name}
                    email={invitee.email}
                    deleted={invitee.is_deleted}
                  />
                ),
              },
              {
                id: 'status',
                header: t('Status'),
                className: 'w-[110px]',
                cell: (invitee) =>
                  invitee.status === INVITEE_STATUS_DISABLED ? (
                    <StatusBadge
                      label={t('Disabled')}
                      variant='neutral'
                      copyable={false}
                    />
                  ) : (
                    <StatusBadge
                      label={t('Enabled')}
                      variant='success'
                      copyable={false}
                    />
                  ),
              },
              {
                id: 'group',
                header: t('Group'),
                className: 'w-[130px]',
                cell: (invitee) => (
                  <StatusBadge
                    label={invitee.group || '-'}
                    autoColor={invitee.group}
                    copyable={false}
                  />
                ),
              },
              {
                id: 'quota',
                header: t('Balance'),
                className: 'w-[120px]',
                cell: (invitee) => (
                  <span className='tabular-nums'>
                    {formatQuota(invitee.quota)}
                  </span>
                ),
              },
              {
                id: 'used_quota',
                header: t('Used Quota'),
                className: 'w-[150px]',
                cell: (invitee) => (
                  <span className='tabular-nums'>
                    {formatQuota(invitee.used_quota)}
                  </span>
                ),
              },
              {
                id: 'period_quota',
                header: t('Usage In Range'),
                className: 'w-[160px]',
                cell: (invitee) => <PeriodUsageCell stats={invitee} />,
              },
            ]}
          />

          <div className='flex items-center justify-between gap-2'>
            <span className='text-muted-foreground text-sm'>
              {t('Total:')} {total.toLocaleString()}
            </span>
            <div className='flex items-center gap-2'>
              <Button
                variant='outline'
                size='sm'
                disabled={page <= 1 || inviteesQuery.isFetching}
                onClick={() => setPage((current) => Math.max(1, current - 1))}
              >
                <ChevronLeft />
                <span className='sr-only'>{t('Go to previous page')}</span>
              </Button>
              <span className='text-muted-foreground text-sm tabular-nums'>
                {page} / {totalPages}
              </span>
              <Button
                variant='outline'
                size='sm'
                disabled={page >= totalPages || inviteesQuery.isFetching}
                onClick={() =>
                  setPage((current) => Math.min(totalPages, current + 1))
                }
              >
                <ChevronRight />
                <span className='sr-only'>{t('Go to next page')}</span>
              </Button>
            </div>
          </div>
        </div>
      </SheetContent>
    </Sheet>
  )
}
