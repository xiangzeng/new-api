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
import { useTranslation } from 'react-i18next'

import { formatMs } from '../lib/format'
import type { CascadeChannelMetricsWindow } from '../types'

// 单个时间窗的指标明细行，卡片只显示紧凑摘要，明细统一在健康时间线弹窗里看
export function MetricsDetailLine({
  label,
  window,
}: {
  label: string
  window: CascadeChannelMetricsWindow
}) {
  const { t } = useTranslation()
  return (
    <div>
      <span className='font-medium'>{label}</span>
      {': '}
      {t('Attempts')} {window.attempts} · {t('Faults')} {window.faults} ·{' '}
      {t('Trips')} {window.trips} · {t('Restores')} {window.restores}
      {window.avg_latency_ms > 0 &&
        ` · ${t('Avg latency')} ${formatMs(window.avg_latency_ms)}`}
      {window.avg_ttft_ms > 0 &&
        ` · ${t('Avg TTFT')} ${formatMs(window.avg_ttft_ms)}`}
    </div>
  )
}
