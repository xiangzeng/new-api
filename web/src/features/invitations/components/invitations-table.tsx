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
import { getRouteApi } from '@tanstack/react-router'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import { getDefaultTimeRange } from '@/features/usage-logs/lib'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'

import { getInvitationSummaries } from '../api'
import type { InvitationSummary, InvitationTimeRange } from '../types'
import { useInvitationsColumns } from './invitations-columns'
import { InviteesSheet } from './invitees-sheet'

const route = getRouteApi('/_authenticated/invitations/')

function toUnixSeconds(milliseconds: number): number {
  return Math.floor(milliseconds / 1000)
}

export function InvitationsTable() {
  const { t } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const searchParams = route.useSearch()
  const navigate = route.useNavigate()
  const [selectedInviter, setSelectedInviter] =
    useState<InvitationSummary | null>(null)
  const [inviteesOpen, setInviteesOpen] = useState(false)

  // Default to the current day so the period aggregation never scans the whole
  // log table; the user can widen the window from the picker presets.
  const [defaultRange] = useState(getDefaultTimeRange)

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: searchParams,
    navigate,
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
  })

  const startTimeMs = searchParams.startTime ?? defaultRange.start.getTime()
  const endTimeMs = searchParams.endTime ?? defaultRange.end.getTime()
  const timeRange: InvitationTimeRange = useMemo(
    () => ({
      start_timestamp: toUnixSeconds(startTimeMs),
      end_timestamp: toUnixSeconds(endTimeMs),
    }),
    [startTimeMs, endTimeMs]
  )
  const hasCustomTimeRange =
    searchParams.startTime != null || searchParams.endTime != null

  const handleTimeRangeChange = useCallback(
    (range: { start?: Date; end?: Date }) => {
      navigate({
        search: (prev) => ({
          ...prev,
          page: undefined,
          startTime: range.start ? range.start.getTime() : undefined,
          endTime: range.end ? range.end.getTime() : undefined,
        }),
      })
    },
    [navigate]
  )

  const handleReset = useCallback(() => {
    navigate({
      search: (prev) => ({
        ...prev,
        page: undefined,
        filter: undefined,
        startTime: undefined,
        endTime: undefined,
      }),
    })
  }, [navigate])

  const handleViewInvitees = useCallback((summary: InvitationSummary) => {
    setSelectedInviter(summary)
    setInviteesOpen(true)
  }, [])

  const columns = useInvitationsColumns({ onViewInvitees: handleViewInvitees })

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'invitations',
      'summary',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      timeRange.start_timestamp,
      timeRange.end_timestamp,
    ],
    queryFn: async () => {
      const result = await getInvitationSummaries({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        keyword: globalFilter?.trim() ?? '',
        start_timestamp: timeRange.start_timestamp,
        end_timestamp: timeRange.end_timestamp,
      })

      if (!result.success) {
        toast.error(result.message || t('Failed to load invitations'))
        return { items: [], total: 0 }
      }

      return {
        items: result.data?.items ?? [],
        total: result.data?.total ?? 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const { table } = useDataTable({
    data: data?.items ?? [],
    columns,
    columnFilters,
    globalFilter,
    pagination,
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    enableSorting: false,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total ?? 0,
    ensurePageInRange,
  })

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle={t('No Invitations Found')}
        emptyDescription={t(
          'No invitation relationships available. Try adjusting your search or time range.'
        )}
        skeletonKeyPrefix='invitations-skeleton'
        applyHeaderSize
        toolbarProps={{
          searchPlaceholder: t(
            'Filter by inviter ID, username, email or invitation code...'
          ),
          searchDebounceMs: 500,
          additionalSearch: (
            <CompactDateTimeRangePicker
              start={new Date(startTimeMs)}
              end={new Date(endTimeMs)}
              onChange={handleTimeRangeChange}
              className='w-full sm:w-[300px]'
            />
          ),
          hasAdditionalFilters: hasCustomTimeRange,
          onReset: handleReset,
        }}
      />

      <InviteesSheet
        open={inviteesOpen}
        onOpenChange={setInviteesOpen}
        inviter={selectedInviter}
        timeRange={timeRange}
      />
    </>
  )
}
