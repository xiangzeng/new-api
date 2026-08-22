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
import { ArrowLeft, ScrollText } from 'lucide-react'
import { useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { getFlowQuotaDates } from '@/features/dashboard/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getUser } from '../../api'
import {
  aggregateUserUsage,
  collectUserUsageOptions,
  filterUserUsageRows,
  getDefaultUserUsageRange,
  type UserUsageDimension,
  type UserUsageDimensionFilters,
  type UserUsageDimensionValue,
  type UserUsageRange,
} from '../../lib/user-usage'
import { UserUsageBreakdown } from './user-usage-breakdown'
import { UserUsageFilterBar } from './user-usage-filter-bar'
import { UserUsageSummaryPanel } from './user-usage-summary'

const route = getRouteApi('/_authenticated/users/$userId')

/** Admin flow rows carry no token/node columns; only root sees those breakdowns. */
const ADMIN_DIMENSIONS: UserUsageDimension[] = ['group', 'model', 'channel']
const ROOT_DIMENSIONS: UserUsageDimension[] = [
  'group',
  'model',
  'channel',
  'token',
  'node',
]

export function UserDetail() {
  const { t } = useTranslation()
  const params = route.useParams()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const role = useAuthStore((state) => state.auth.user?.role ?? ROLE.USER)

  const userId = Number(params.userId)
  const dimensions =
    role >= ROLE.SUPER_ADMIN ? ROOT_DIMENSIONS : ADMIN_DIMENSIONS
  const dimension =
    search.dim && dimensions.includes(search.dim) ? search.dim : 'group'

  const fallbackRange = useMemo(() => getDefaultUserUsageRange(), [])
  const rangeStart = search.start ?? fallbackRange.start
  const rangeEnd = search.end ?? fallbackRange.end
  const range: UserUsageRange = useMemo(
    () => ({ start: rangeStart, end: rangeEnd }),
    [rangeEnd, rangeStart]
  )

  const filters: UserUsageDimensionFilters = useMemo(
    () => ({
      group: search.group,
      model: search.model,
      channel: search.channel,
      token: search.token,
      node: search.node,
    }),
    [search.channel, search.group, search.model, search.node, search.token]
  )
  const keyword = search.q ?? ''
  const hasActiveFilters =
    Boolean(keyword) || dimensions.some((item) => Boolean(filters[item]))

  const userQuery = useQuery({
    queryKey: ['user-detail', userId],
    queryFn: () => getUser(userId),
    enabled: Number.isInteger(userId) && userId > 0,
  })
  const user = userQuery.data?.success ? userQuery.data.data : undefined
  const username = user?.username

  const usageQuery = useQuery({
    queryKey: ['user-usage-flow', username, range.start, range.end],
    queryFn: async () => {
      const result = await getFlowQuotaDates(
        {
          start_timestamp: range.start,
          end_timestamp: range.end,
          username,
        },
        true
      )
      if (!result.success) {
        throw new Error(result.message || t('Failed to load usage data'))
      }
      return result.data ?? []
    },
    enabled: Boolean(username),
  })

  const flowRows = useMemo(() => usageQuery.data ?? [], [usageQuery.data])
  const scopedRows = useMemo(
    () => filterUserUsageRows(flowRows, filters),
    [filters, flowRows]
  )
  const summary = useMemo(
    () => aggregateUserUsage(scopedRows, dimension),
    [dimension, scopedRows]
  )
  const options = useMemo(() => {
    const collected: Partial<
      Record<UserUsageDimension, UserUsageDimensionValue[]>
    > = {}
    for (const item of dimensions) {
      collected[item] = collectUserUsageOptions(flowRows, item)
    }
    return collected
  }, [dimensions, flowRows])

  const updateSearch = useCallback(
    (next: Record<string, unknown>) => {
      void navigate({
        search: (prev) => ({ ...prev, ...next }),
        replace: true,
      })
    },
    [navigate]
  )

  const handleReset = useCallback(() => {
    updateSearch({
      group: undefined,
      model: undefined,
      channel: undefined,
      token: undefined,
      node: undefined,
      q: undefined,
    })
  }, [updateSearch])

  const usageLoading = usageQuery.isPending && Boolean(username)
  const profileMissing = !userQuery.isPending && !user

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {username ? `${username} · ${t('Usage details')}` : t('Usage details')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => void navigate({ to: '/users' })}
        >
          <ArrowLeft className='size-4' />
          {t('Back to users')}
        </Button>
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={!username}
          onClick={() =>
            void navigate({
              to: '/usage-logs/$section',
              params: { section: 'common' },
              search: {
                page: 1,
                username,
                startTime: range.start * 1000,
                endTime: range.end * 1000,
              },
            })
          }
        >
          <ScrollText className='size-4' />
          {t('View user logs')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {profileMissing ? (
          <div className='text-muted-foreground py-12 text-center text-sm'>
            {t('Failed to fetch user information')}
          </div>
        ) : (
          <div className='space-y-4'>
            <UserUsageSummaryPanel
              user={user}
              summary={summary}
              profileLoading={userQuery.isPending}
              usageLoading={usageLoading}
            />

            <UserUsageFilterBar
              range={range}
              onRangeChange={(next) =>
                updateSearch({ start: next.start, end: next.end })
              }
              dimensions={dimensions}
              options={options}
              filters={filters}
              onFilterChange={(item, value) =>
                updateSearch({ [item]: value || undefined })
              }
              keyword={keyword}
              onKeywordChange={(value) =>
                updateSearch({ q: value || undefined })
              }
              onReset={handleReset}
              hasActiveFilters={hasActiveFilters}
            />

            <UserUsageBreakdown
              dimension={dimension}
              dimensions={dimensions}
              onDimensionChange={(next) => updateSearch({ dim: next })}
              summary={summary}
              keyword={keyword}
              loading={usageLoading}
            />

            <p className='text-muted-foreground text-xs'>
              {t(
                'Counted from grouped usage records aggregated hourly, so requests without a group are excluded and the last few minutes may be missing.'
              )}
            </p>
          </div>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
