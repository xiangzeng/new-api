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
import type { FlowQuotaDataItem } from '@/features/dashboard/types'
import dayjs from '@/lib/dayjs'

/**
 * Breakdown dimensions available on the user usage detail page.
 *
 * `token` and `node` are only populated for root accounts — the admin variant
 * of `/api/data/flow` does not select those columns.
 */
export const USER_USAGE_DIMENSIONS = [
  'group',
  'model',
  'channel',
  'token',
  'node',
] as const

export type UserUsageDimension = (typeof USER_USAGE_DIMENSIONS)[number]

/** Rows whose dimension value is missing collapse into this synthetic bucket. */
export const USER_USAGE_UNKNOWN_KEY = '__unknown__'

export type UserUsageDimensionFilters = Partial<
  Record<UserUsageDimension, string>
>

export interface UserUsageDimensionValue {
  /** Stable identity used for filtering and as the React key. */
  key: string
  /** Display name; empty when the source row carried no value. */
  name: string
}

export interface UserUsageRow extends UserUsageDimensionValue {
  quota: number
  count: number
  tokens: number
  /** Fraction of the range's total quota consumed by this row (0-1) */
  share: number
}

export interface UserUsageSummary {
  totalQuota: number
  totalCount: number
  totalTokens: number
  /** One row per dimension value, ranked by consumption descending */
  rows: UserUsageRow[]
}

export const EMPTY_USER_USAGE_SUMMARY: UserUsageSummary = {
  totalQuota: 0,
  totalCount: 0,
  totalTokens: 0,
  rows: [],
}

/** Table row after the caller resolved a translated label for unknown values. */
export interface UserUsageTableRow extends UserUsageRow {
  label: string
}

export type UserUsageSortKey = 'label' | 'quota' | 'count' | 'tokens'
export type UserUsageSortOrder = 'asc' | 'desc'

/** Unix timestamps in seconds, matching the `/api/data/flow` contract. */
export interface UserUsageRange {
  start: number
  end: number
}

function namedDimensionValue(
  name: string | undefined
): UserUsageDimensionValue {
  const trimmed = (name ?? '').trim()
  if (!trimmed) return { key: USER_USAGE_UNKNOWN_KEY, name: '' }
  return { key: trimmed, name: trimmed }
}

function identifiedDimensionValue(
  id: number | undefined,
  name: string | undefined
): UserUsageDimensionValue {
  if (!id) return { key: USER_USAGE_UNKNOWN_KEY, name: '' }
  const trimmed = (name ?? '').trim()
  return { key: String(id), name: trimmed || `#${id}` }
}

export function getUserUsageDimensionValue(
  row: FlowQuotaDataItem,
  dimension: UserUsageDimension
): UserUsageDimensionValue {
  switch (dimension) {
    case 'group':
      return namedDimensionValue(row.use_group)
    case 'model':
      return namedDimensionValue(row.model_name)
    case 'channel':
      return identifiedDimensionValue(row.channel_id, row.channel_name)
    case 'token':
      return identifiedDimensionValue(row.token_id, row.token_name)
    case 'node':
      return namedDimensionValue(row.node_name)
  }
}

/**
 * Keep only the flow rows matching every active dimension filter.
 *
 * Each flow row carries all dimensions at once (group + model + channel …),
 * so filtering by one dimension narrows every other breakdown accordingly.
 */
export function filterUserUsageRows(
  rows: FlowQuotaDataItem[] | undefined | null,
  filters: UserUsageDimensionFilters
): FlowQuotaDataItem[] {
  if (!rows?.length) return []
  const active = USER_USAGE_DIMENSIONS.filter((dimension) =>
    Boolean(filters[dimension])
  )
  if (active.length === 0) return rows
  return rows.filter((row) =>
    active.every(
      (dimension) =>
        getUserUsageDimensionValue(row, dimension).key === filters[dimension]
    )
  )
}

/** Collapse per-model flow rows into totals plus one ranked row per dimension value. */
export function aggregateUserUsage(
  rows: FlowQuotaDataItem[] | undefined | null,
  dimension: UserUsageDimension
): UserUsageSummary {
  if (!rows?.length) return EMPTY_USER_USAGE_SUMMARY
  const totals = new Map<
    string,
    { name: string; quota: number; count: number; tokens: number }
  >()
  let totalQuota = 0
  let totalCount = 0
  let totalTokens = 0
  for (const row of rows) {
    const value = getUserUsageDimensionValue(row, dimension)
    const quota = Number(row.quota) || 0
    const count = Number(row.count) || 0
    const tokens = Number(row.token_used) || 0
    const current = totals.get(value.key) ?? {
      name: value.name,
      quota: 0,
      count: 0,
      tokens: 0,
    }
    current.quota += quota
    current.count += count
    current.tokens += tokens
    totals.set(value.key, current)
    totalQuota += quota
    totalCount += count
    totalTokens += tokens
  }
  const aggregated = [...totals.entries()]
    .map(([key, value]) => ({
      key,
      name: value.name,
      quota: value.quota,
      count: value.count,
      tokens: value.tokens,
      share: totalQuota > 0 ? value.quota / totalQuota : 0,
    }))
    .sort((a, b) => b.quota - a.quota || a.name.localeCompare(b.name))
  return { totalQuota, totalCount, totalTokens, rows: aggregated }
}

/** Dimension values present in the range, ranked by consumption descending. */
export function collectUserUsageOptions(
  rows: FlowQuotaDataItem[] | undefined | null,
  dimension: UserUsageDimension
): UserUsageDimensionValue[] {
  return aggregateUserUsage(rows, dimension).rows.map((row) => ({
    key: row.key,
    name: row.name,
  }))
}

export function sortUserUsageRows(
  rows: UserUsageTableRow[],
  sortKey: UserUsageSortKey,
  sortOrder: UserUsageSortOrder
): UserUsageTableRow[] {
  const direction = sortOrder === 'asc' ? 1 : -1
  return [...rows].sort((a, b) => {
    if (sortKey === 'label') return a.label.localeCompare(b.label) * direction
    return (a[sortKey] - b[sortKey]) * direction
  })
}

/** Rolling 30-day window used when the URL carries no explicit range. */
export function getDefaultUserUsageRange(
  now: Date = new Date()
): UserUsageRange {
  const today = dayjs(now)
  return {
    start: today.subtract(29, 'day').startOf('day').unix(),
    end: today.endOf('day').unix(),
  }
}

/** Full calendar month range for a `YYYY-MM` value, or null when unparseable. */
export function getUserUsageMonthRange(month: string): UserUsageRange | null {
  if (!/^\d{4}-\d{2}$/.test(month)) return null
  const parsed = dayjs(`${month}-01`)
  if (!parsed.isValid()) return null
  return {
    start: parsed.startOf('month').unix(),
    end: parsed.endOf('month').unix(),
  }
}

/** Most recent `count` months, newest first, as `YYYY-MM` values. */
export function getUserUsageMonthOptions(
  count: number,
  now: Date = new Date()
): string[] {
  const current = dayjs(now).startOf('month')
  const months: string[] = []
  for (let index = 0; index < count; index += 1) {
    months.push(current.subtract(index, 'month').format('YYYY-MM'))
  }
  return months
}

/** `YYYY-MM` when the range covers exactly one calendar month, otherwise null. */
export function matchUserUsageMonth(range: UserUsageRange): string | null {
  const start = dayjs.unix(range.start)
  if (!start.isValid()) return null
  if (start.startOf('month').unix() !== range.start) return null
  if (start.endOf('month').unix() !== range.end) return null
  return start.format('YYYY-MM')
}
