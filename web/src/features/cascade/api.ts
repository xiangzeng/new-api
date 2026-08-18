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
import { api } from '@/lib/api'

import type {
  CascadeActionResponse,
  CascadeHealthEventsResponse,
  CascadeOverviewResponse,
  CascadePurgeGroupResponse,
} from './types'

export async function getCascadeOverview() {
  const res = await api.get<CascadeOverviewResponse>('/api/cascade/overview')
  return res.data
}

// 保存编排顺序：orders = 各分组内渠道的溢出顺序（影响路由），
// group_sequence = 分组泳道在编排页的展示顺序（纯展示）。两者可单独提交。
export async function saveCascadeOrder(payload: {
  orders?: { group: string; channel_ids: number[] }[]
  group_sequence?: string[]
}) {
  const res = await api.post<CascadeActionResponse>(
    '/api/cascade/order',
    payload
  )
  return res.data
}

// 保存渠道 RPM 水位线：rpm <= 0 表示清除（= 不限流），未提交的渠道保持原值
export async function saveCascadeWatermark(
  watermarks: { channel_id: number; rpm: number }[]
) {
  const res = await api.post<CascadeActionResponse>('/api/cascade/watermark', {
    watermarks,
  })
  return res.data
}

export async function resetCascadeChannelHealth(channelId: number) {
  const res = await api.post<CascadeActionResponse>(
    '/api/cascade/reset_health',
    {
      channel_id: channelId,
    }
  )
  return res.data
}

// 清理孤儿分组：把已失效的组名从所有渠道上摘掉（后端只接受非在役分组）
export async function purgeCascadeGroup(group: string) {
  const res = await api.post<CascadePurgeGroupResponse>(
    '/api/cascade/purge_group',
    { group }
  )
  return res.data
}

export async function getCascadeHealthEvents(channelId: number) {
  const res = await api.get<CascadeHealthEventsResponse>(
    '/api/cascade/health_events',
    { params: { channel_id: channelId } }
  )
  return res.data
}

export async function updateCascadeSettingOption(
  key: string,
  value: string | boolean | number
) {
  const res = await api.put<CascadeActionResponse>('/api/option/', {
    key: `cascade_setting.${key}`,
    value,
  })
  return res.data
}
