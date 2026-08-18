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
import type { ChannelDailyUsage } from '../types'

export type RecentUsageDay = {
  date: string
  labelKey: 'Today' | 'Yesterday' | 'Day before yesterday'
}

export type RecentUsageDateRange = {
  startDate: string
  endDate: string
  days: RecentUsageDay[]
}

export type RecentUsageDisplayRow = RecentUsageDay &
  Pick<ChannelDailyUsage, 'quota_used' | 'request_count' | 'token_used'>

function formatLocalDate(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

export function buildRecentUsageDateRange(
  anchor = new Date(),
  dayCount = 3
): RecentUsageDateRange {
  const normalizedDayCount = Math.max(1, Math.floor(dayCount))
  const anchorDay = new Date(
    anchor.getFullYear(),
    anchor.getMonth(),
    anchor.getDate()
  )
  const days: RecentUsageDay[] = []

  for (let offset = 0; offset < normalizedDayCount; offset++) {
    const date = new Date(anchorDay)
    date.setDate(anchorDay.getDate() - offset)
    let labelKey: RecentUsageDay['labelKey'] = 'Day before yesterday'
    if (offset === 0) {
      labelKey = 'Today'
    } else if (offset === 1) {
      labelKey = 'Yesterday'
    }
    days.push({ date: formatLocalDate(date), labelKey })
  }

  return {
    startDate: days.at(-1)?.date ?? formatLocalDate(anchorDay),
    endDate: days[0]?.date ?? formatLocalDate(anchorDay),
    days,
  }
}

export function mergeRecentUsageDays(
  days: RecentUsageDay[],
  rows: ChannelDailyUsage[] | undefined
): RecentUsageDisplayRow[] {
  const rowByDate = new Map((rows ?? []).map((row) => [row.date, row]))

  return days.map((day) => {
    const row = rowByDate.get(day.date)
    return {
      ...day,
      quota_used: row?.quota_used ?? 0,
      request_count: row?.request_count ?? 0,
      token_used: row?.token_used ?? 0,
    }
  })
}
