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
import { toIntlLocale } from '@/i18n/languages'
import {
  formatTimestampRelative,
  formatTimestampToDate,
  parseTimestampFromInput,
} from '@/lib/format'

/** Preset block durations; `keep` leaves an existing block's expiry unchanged. */
export type GroupBlockDuration =
  | 'keep'
  | '1h'
  | '6h'
  | '24h'
  | '3d'
  | '7d'
  | 'permanent'
  | 'custom'

const PRESET_SECONDS: Partial<Record<GroupBlockDuration, number>> = {
  '1h': 60 * 60,
  '6h': 6 * 60 * 60,
  '24h': 24 * 60 * 60,
  '3d': 3 * 24 * 60 * 60,
  '7d': 7 * 24 * 60 * 60,
}

export const DEFAULT_GROUP_BLOCK_DURATION: GroupBlockDuration = '24h'

/**
 * Resolve the unix-second expiry for a duration choice.
 * Returns 0 for permanent and null when the choice is invalid
 * (missing / past custom time, or `keep` without an existing block).
 */
export function resolveGroupBlockExpiresAt(
  duration: GroupBlockDuration,
  options: { nowSec: number; keepExpiresAt?: number; customInput?: string }
): number | null {
  if (duration === 'permanent') return 0
  if (duration === 'keep') {
    return options.keepExpiresAt ?? null
  }
  if (duration === 'custom') {
    if (!options.customInput) return null
    const ts = parseTimestampFromInput(options.customInput)
    return Number.isFinite(ts) && ts > options.nowSec ? ts : null
  }
  const seconds = PRESET_SECONDS[duration]
  return seconds ? options.nowSec + seconds : null
}

/**
 * Relative remaining time, e.g. "in 5 hours" / "5小时后".
 * `language` is the i18next code (`zhCN`…), normalized for Intl here.
 */
export function formatGroupBlockRemaining(
  expiresAt: number,
  language: string
): string {
  return formatTimestampRelative(expiresAt, 'seconds', toIntlLocale(language))
}

/** Absolute expiry time for tooltips (YYYY-MM-DD HH:mm:ss). */
export function formatGroupBlockExpiry(expiresAt: number): string {
  return formatTimestampToDate(expiresAt)
}
