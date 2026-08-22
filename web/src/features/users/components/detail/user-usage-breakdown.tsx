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
import { ArrowDown, ArrowUp, Loader2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatCompactNumber, formatQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import { USER_USAGE_DIMENSION_META } from '../../constants'
import {
  USER_USAGE_UNKNOWN_KEY,
  sortUserUsageRows,
  type UserUsageDimension,
  type UserUsageSortKey,
  type UserUsageSortOrder,
  type UserUsageSummary,
  type UserUsageTableRow,
} from '../../lib/user-usage'

interface UserUsageBreakdownProps {
  dimension: UserUsageDimension
  dimensions: UserUsageDimension[]
  onDimensionChange: (dimension: UserUsageDimension) => void
  summary: UserUsageSummary
  keyword: string
  loading: boolean
}

function SortableHeader(props: {
  label: string
  sortKey: UserUsageSortKey
  activeKey: UserUsageSortKey
  order: UserUsageSortOrder
  align?: 'left' | 'right'
  onSort: (sortKey: UserUsageSortKey) => void
}) {
  const isActive = props.activeKey === props.sortKey
  const Icon = props.order === 'asc' ? ArrowUp : ArrowDown

  return (
    <button
      type='button'
      onClick={() => props.onSort(props.sortKey)}
      aria-pressed={isActive}
      className={cn(
        'hover:text-foreground flex w-full items-center gap-1 transition-colors',
        props.align === 'right' && 'justify-end',
        isActive && 'text-foreground font-medium'
      )}
    >
      {props.label}
      {isActive && <Icon className='size-3' aria-hidden='true' />}
    </button>
  )
}

export function UserUsageBreakdown(props: UserUsageBreakdownProps) {
  const { t } = useTranslation()
  const [sortKey, setSortKey] = useState<UserUsageSortKey>('quota')
  const [sortOrder, setSortOrder] = useState<UserUsageSortOrder>('desc')

  const rows = useMemo(() => {
    const labelled: UserUsageTableRow[] = props.summary.rows.map((row) => ({
      ...row,
      label: row.key === USER_USAGE_UNKNOWN_KEY ? t('Unknown') : row.name,
    }))
    const keyword = props.keyword.trim().toLowerCase()
    const matched = keyword
      ? labelled.filter((row) => row.label.toLowerCase().includes(keyword))
      : labelled
    return sortUserUsageRows(matched, sortKey, sortOrder)
  }, [props.keyword, props.summary.rows, sortKey, sortOrder, t])

  const handleSort = (nextKey: UserUsageSortKey) => {
    if (nextKey === sortKey) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc')
      return
    }
    setSortKey(nextKey)
    setSortOrder(nextKey === 'label' ? 'asc' : 'desc')
  }

  const columns = [
    {
      id: 'name',
      header: (
        <SortableHeader
          label={t(USER_USAGE_DIMENSION_META[props.dimension].labelKey)}
          sortKey='label'
          activeKey={sortKey}
          order={sortOrder}
          onSort={handleSort}
        />
      ),
      cell: (row: UserUsageTableRow) => (
        <span className='font-medium break-all'>{row.label}</span>
      ),
    },
    {
      id: 'quota',
      className: 'w-[120px] text-right',
      cellClassName: 'text-right font-semibold tabular-nums',
      header: (
        <SortableHeader
          label={t('Used Quota')}
          sortKey='quota'
          activeKey={sortKey}
          order={sortOrder}
          align='right'
          onSort={handleSort}
        />
      ),
      cell: (row: UserUsageTableRow) => formatQuota(row.quota),
    },
    {
      id: 'count',
      className: 'w-[100px] text-right',
      cellClassName: 'text-muted-foreground text-right tabular-nums',
      header: (
        <SortableHeader
          label={t('Requests')}
          sortKey='count'
          activeKey={sortKey}
          order={sortOrder}
          align='right'
          onSort={handleSort}
        />
      ),
      cell: (row: UserUsageTableRow) => formatCompactNumber(row.count),
    },
    {
      id: 'tokens',
      className: 'w-[100px] text-right',
      cellClassName: 'text-muted-foreground text-right tabular-nums',
      header: (
        <SortableHeader
          label={t('Tokens')}
          sortKey='tokens'
          activeKey={sortKey}
          order={sortOrder}
          align='right'
          onSort={handleSort}
        />
      ),
      cell: (row: UserUsageTableRow) => formatCompactNumber(row.tokens),
    },
    {
      id: 'share',
      className: 'w-[80px] text-right',
      cellClassName: 'text-right tabular-nums',
      header: <span className='block text-right'>{t('Share')}</span>,
      cell: (row: UserUsageTableRow) => `${Math.round(row.share * 1000) / 10}%`,
    },
  ]

  return (
    <div className='space-y-3'>
      <Tabs
        value={props.dimension}
        onValueChange={(value) =>
          props.onDimensionChange(value as UserUsageDimension)
        }
      >
        <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
          {props.dimensions.map((dimension) => (
            <TabsTrigger key={dimension} value={dimension}>
              {t(USER_USAGE_DIMENSION_META[dimension].labelKey)}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      {props.loading ? (
        <div className='flex items-center justify-center rounded-lg border py-10'>
          <Loader2 className='text-muted-foreground size-5 animate-spin' />
        </div>
      ) : (
        <StaticDataTable
          columns={columns}
          data={rows}
          getRowKey={(row) => row.key}
          emptyContent={t('No usage data in the selected period')}
        />
      )}
    </div>
  )
}
