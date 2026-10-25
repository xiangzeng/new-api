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
import type { CascadeChannel } from '../types'

/** 渠道限流方式：RPM 水位线与每日额度上限二选一 */
export type CascadeLimitMode = 'rpm' | 'daily_quota'

/** 配了每日额度上限即额度模式，否则按 RPM 水位线（含两者都未配置的不限流） */
export function getCascadeLimitMode(channel: CascadeChannel): CascadeLimitMode {
  return (channel.daily_quota_cap ?? 0) > 0 ? 'daily_quota' : 'rpm'
}

/** 今日额度是否已满：满额渠道在级联选路中等同禁用，次日 0 点自动恢复 */
export function isDailyQuotaReached(channel: CascadeChannel): boolean {
  const quotaCap = channel.daily_quota_cap ?? 0
  return quotaCap > 0 && (channel.today_used_quota ?? 0) >= quotaCap
}
