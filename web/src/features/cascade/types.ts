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
export type CascadeHealthState = 'healthy' | 'cooling' | 'probing'

export type CascadeChannelHealth = {
  channel_id: number
  state: CascadeHealthState
  consecutive_failures: number
  recovery_successes: number
  tripped_at?: number
  cooldown_remaining?: number
  last_error?: string
  last_error_at?: number
}

export type CascadeChannelMetricsWindow = {
  attempts: number
  faults: number
  error_rate: number
  trips: number
  restores: number
  avg_latency_ms: number
  avg_ttft_ms: number
}

export type CascadeChannelMetrics = {
  '1h': CascadeChannelMetricsWindow
  '24h': CascadeChannelMetricsWindow
}

export type CascadeChannel = {
  id: number
  name: string
  type: number
  status: number
  priority: number
  weight: number
  health?: CascadeChannelHealth
  metrics?: CascadeChannelMetrics
  /** 近 60 秒被选中的次数（滚动窗口，选中即记账） */
  rpm: number
  /** RPM 水位线，0 = 不限流 */
  rpm_watermark: number
}

export type CascadeGroup = {
  name: string
  /** 该分组已从分组倍率配置中删除，但渠道 group 字段仍残留组名 */
  orphan?: boolean
  channels: CascadeChannel[]
}

export type CascadeSetting = {
  enabled: boolean
  failure_threshold: number
  cooldown_seconds: number
  probe_enabled: boolean
  probe_interval_seconds: number
  recovery_success_count: number
  max_attempts_per_request: number
  watermark_enabled: boolean
  incomplete_stream_as_fault: boolean
  extra_fault_status_codes: number[]
  extra_fault_keywords: string[]
  ignore_fault_keywords: string[]
}

export type CascadeOverviewResponse = {
  success: boolean
  message?: string
  data?: {
    groups: CascadeGroup[]
    setting: CascadeSetting
  }
}

export type CascadeActionResponse = {
  success: boolean
  message?: string
}

export type CascadePurgeGroupResponse = {
  success: boolean
  message?: string
  data?: {
    updated: number
    skipped: { id: number; name: string }[]
  }
}

export type CascadeHealthEventType = 'trip' | 'restore' | 'manual_reset'

export type CascadeHealthEvent = {
  id: number
  channel_id: number
  event: CascadeHealthEventType
  reason?: string
  created_at: number
}

export type CascadeHealthEventsResponse = {
  success: boolean
  message?: string
  data?: {
    events: CascadeHealthEvent[]
    trip_count_24h: number
    trip_count_7d: number
  }
}
