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
import type { FlowQuotaDataItem } from '../types'

export interface FlowGroupUsageRow {
  name: string
  quota: number
  count: number
  /** Fraction of the window's total quota consumed by this group (0-1) */
  share: number
}

export interface FlowGroupUsageSummary {
  totalQuota: number
  totalCount: number
  /** One row per group, ranked by consumption descending */
  groups: FlowGroupUsageRow[]
}

export const EMPTY_FLOW_GROUP_USAGE: FlowGroupUsageSummary = {
  totalQuota: 0,
  totalCount: 0,
  groups: [],
}

/**
 * Collapse per-model flow rows into totals plus one ranked row per user group.
 *
 * Used wherever an admin needs to see which groups a single user actually
 * consumes — the usage-logs user detail dialog and the custom-pricing editor.
 */
export function aggregateFlowGroupUsage(
  rows: FlowQuotaDataItem[] | undefined | null
): FlowGroupUsageSummary {
  if (!rows?.length) {
    return EMPTY_FLOW_GROUP_USAGE
  }
  const totals = new Map<string, { quota: number; count: number }>()
  let totalQuota = 0
  let totalCount = 0
  for (const row of rows) {
    const name = (row.use_group || '').trim() || 'unknown'
    const quota = Number(row.quota) || 0
    const count = Number(row.count) || 0
    const current = totals.get(name) || { quota: 0, count: 0 }
    current.quota += quota
    current.count += count
    totals.set(name, current)
    totalQuota += quota
    totalCount += count
  }
  const groups = [...totals.entries()]
    .map(([name, value]) => ({
      name,
      quota: value.quota,
      count: value.count,
      share: totalQuota > 0 ? value.quota / totalQuota : 0,
    }))
    .sort((a, b) => b.quota - a.quota || a.name.localeCompare(b.name))
  return { totalQuota, totalCount, groups }
}
