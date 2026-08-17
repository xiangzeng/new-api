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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { getChannels } from '@/features/channels/api'
import { ChannelsDialogs } from '@/features/channels/components/channels-dialogs'
import { ChannelsProvider } from '@/features/channels/components/channels-provider'
import { channelsQueryKeys } from '@/features/channels/lib'
import type { Channel } from '@/features/channels/types'

import { getCascadeOverview, saveCascadeOrder } from './api'
import { CascadeLane } from './components/cascade-lane'
import { CascadeSettingsCard } from './components/settings-card'
import type { CascadeOverviewResponse } from './types'

// 卡片操作弹窗需要全量渠道字段，编排 overview 只带瘦数据，这里一次拉全
const CASCADE_CHANNELS_PAGE_SIZE = 1000

// 本地顺序里剔除已消失的渠道，追加服务端新增的渠道
function mergeOrder(serverIds: number[], localOrder?: number[]): number[] {
  if (!localOrder) return serverIds
  const serverSet = new Set(serverIds)
  const known = new Set(localOrder)
  return [
    ...localOrder.filter((id) => serverSet.has(id)),
    ...serverIds.filter((id) => !known.has(id)),
  ]
}

export function Cascade() {
  return (
    <ChannelsProvider>
      <CascadeContent />
      <ChannelsDialogs />
    </ChannelsProvider>
  )
}

function CascadeContent() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data, isLoading, error } = useQuery({
    queryKey: ['cascade', 'overview'],
    queryFn: getCascadeOverview,
    refetchInterval: 5000,
  })

  const channelsQuery = useQuery({
    queryKey: channelsQueryKeys.list({ view: 'cascade' }),
    queryFn: () => getChannels({ p: 1, page_size: CASCADE_CHANNELS_PAGE_SIZE }),
  })

  const channelItems = channelsQuery.data?.data?.items
  const channelsById = useMemo(() => {
    const map = new Map<number, Channel>()
    for (const item of channelItems ?? []) {
      map.set(item.id, item)
    }
    return map
  }, [channelItems])

  // 所有渠道操作（含弹窗内保存）都会失效 channels 列表 query，
  // 列表刷新后同步刷新编排总览，不等 5s 轮询
  const listUpdatedAt = channelsQuery.dataUpdatedAt
  const skipInitialListUpdate = useRef(true)
  useEffect(() => {
    if (!listUpdatedAt) return
    if (skipInitialListUpdate.current) {
      skipInitialListUpdate.current = false
      return
    }
    queryClient.invalidateQueries({ queryKey: ['cascade', 'overview'] })
  }, [listUpdatedAt, queryClient])

  // groupName -> 未保存的本地顺序；空对象 = 全部跟随服务端
  const [localOrders, setLocalOrders] = useState<Record<string, number[]>>({})

  const overview = data?.data
  const setting = overview?.setting
  const groups = overview?.groups ?? []

  const idsByGroup = new Map<string, number[]>()
  const dirtyGroups: string[] = []
  for (const group of groups) {
    const serverIds = group.channels.map((channel) => channel.id)
    const ids = mergeOrder(serverIds, localOrders[group.name])
    idsByGroup.set(group.name, ids)
    if (ids.join(',') !== serverIds.join(',')) {
      dirtyGroups.push(group.name)
    }
  }
  const dirty = dirtyGroups.length > 0

  const saveMutation = useMutation({
    mutationFn: saveCascadeOrder,
    onSuccess: async (res, orders) => {
      if (!res.success) {
        toast.error(res.message || t('Save failed'))
        return
      }
      toast.success(t('Order saved'))
      // 先取消在途轮询，再把新顺序乐观写入缓存，避免保存后闪回旧顺序。
      // 编排顺序按分组独立存储，直接按保存的列表重排各组；
      // 未入列渠道与后端同规则垫底（优先级降序、id 升序）
      await queryClient.cancelQueries({ queryKey: ['cascade', 'overview'] })
      const orderByGroup = new Map(
        orders.map((order) => [order.group, order.channel_ids])
      )
      queryClient.setQueryData<CascadeOverviewResponse>(
        ['cascade', 'overview'],
        (prev) => {
          if (!prev?.data) return prev
          return {
            ...prev,
            data: {
              ...prev.data,
              groups: prev.data.groups.map((group) => {
                const saved = orderByGroup.get(group.name)
                if (!saved) return group
                const pos = new Map(saved.map((id, index) => [id, index]))
                return {
                  ...group,
                  channels: [...group.channels].sort((a, b) => {
                    const pa = pos.get(a.id)
                    const pb = pos.get(b.id)
                    if (pa !== undefined && pb !== undefined) return pa - pb
                    if (pa !== undefined || pb !== undefined) {
                      return pa !== undefined ? -1 : 1
                    }
                    return a.priority !== b.priority
                      ? b.priority - a.priority
                      : a.id - b.id
                  }),
                }
              }),
            },
          }
        }
      )
      setLocalOrders({})
      queryClient.invalidateQueries({ queryKey: ['cascade', 'overview'] })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Save failed'))
    },
  })

  // 编排顺序按分组独立存储，只提交有改动的泳道，跨组互不影响
  const handleSaveAll = () => {
    saveMutation.mutate(
      dirtyGroups.map((name) => ({
        group: name,
        channel_ids: idsByGroup.get(name) ?? [],
      }))
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex min-w-0 items-center gap-2'>
          <span className='truncate'>{t('Channel Cascade')}</span>
          {setting && (
            <Badge variant={setting.enabled ? 'default' : 'outline'}>
              {setting.enabled ? t('Enabled') : t('Not enabled')}
            </Badge>
          )}
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Traffic flows left to right within each group. When a channel returns a fault error, the request spills to the next one and the channel is temporarily marked unavailable; tripped channels are probed periodically and restored after consecutive successes. Drag cards to reorder, then save.'
            )}
          </p>

          {dirty && (
            <div className='bg-card sticky top-2 z-10 flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3 shadow-sm'>
              <span className='text-sm font-medium'>
                {t('Unsaved order changes')}
              </span>
              <div className='flex items-center gap-2'>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => setLocalOrders({})}
                  disabled={saveMutation.isPending}
                >
                  {t('Cancel')}
                </Button>
                <Button
                  size='sm'
                  onClick={handleSaveAll}
                  disabled={saveMutation.isPending}
                >
                  {t('Save Order')}
                </Button>
              </div>
            </div>
          )}

          {isLoading && (
            <div className='flex justify-center py-12'>
              <Spinner />
            </div>
          )}
          {error !== null && !isLoading && (
            <p className='text-destructive text-sm'>
              {error instanceof Error ? error.message : t('Load failed')}
            </p>
          )}
          {!isLoading && !error && groups.length === 0 && (
            <p className='text-muted-foreground py-12 text-center text-sm'>
              {t('No channels yet')}
            </p>
          )}

          {groups.map((group) => (
            <CascadeLane
              key={group.name}
              group={group}
              recoveryTarget={setting?.recovery_success_count ?? 3}
              fullChannelsById={channelsById}
              ids={idsByGroup.get(group.name) ?? []}
              onReorder={(nextIds) =>
                setLocalOrders((prev) => ({ ...prev, [group.name]: nextIds }))
              }
            />
          ))}

          {setting && <CascadeSettingsCard setting={setting} />}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
