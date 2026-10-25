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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowRight, RotateCcw, Trash2 } from 'lucide-react'
import { Fragment, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { channelsQueryKeys } from '@/features/channels/lib'
import type { Channel } from '@/features/channels/types'

import {
  purgeCascadeGroup,
  resetCascadeChannelHealth,
  resetCascadeGroupAffinity,
} from '../api'
import type { CascadeChannel, CascadeGroup } from '../types'
import { ChannelCard } from './channel-card'

type CascadeLaneProps = {
  group: CascadeGroup
  recoveryTarget: number
  fullChannelsById: Map<number, Channel>
  /** 展示顺序（服务端顺序与页面级未保存调整合并后），由父级统一维护 */
  ids: number[]
  onReorder: (nextIds: number[]) => void
}

export function CascadeLane({
  group,
  recoveryTarget,
  fullChannelsById,
  ids,
  onReorder,
}: CascadeLaneProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [draggingId, setDraggingId] = useState<number | null>(null)
  const [purgeOpen, setPurgeOpen] = useState(false)
  const [resetAffinityOpen, setResetAffinityOpen] = useState(false)
  const dragIndexRef = useRef<number | null>(null)

  const channelsById = new Map<number, CascadeChannel>(
    group.channels.map((channel) => [channel.id, channel])
  )

  const restoreMutation = useMutation({
    mutationFn: resetCascadeChannelHealth,
    onSuccess: (res) => {
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      toast.success(t('Channel restored'))
      queryClient.invalidateQueries({ queryKey: ['cascade', 'overview'] })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  // 孤儿分组清理：后端会把组名从所有渠道上摘掉并重建 abilities，不可逆
  const purgeMutation = useMutation({
    mutationFn: purgeCascadeGroup,
    onSuccess: (res) => {
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      const skipped = res.data?.skipped ?? []
      if (skipped.length > 0) {
        toast.warning(
          t(
            'Group cleaned up from {{count}} channels; {{skipped}} skipped because it was their only group: {{names}}',
            {
              count: res.data?.updated ?? 0,
              skipped: skipped.length,
              names: skipped
                .map((item) => `#${item.id} ${item.name}`)
                .join(', '),
            }
          )
        )
      } else {
        toast.success(
          t('Group cleaned up from {{count}} channels', {
            count: res.data?.updated ?? 0,
          })
        )
      }
      setPurgeOpen(false)
      queryClient.invalidateQueries({ queryKey: ['cascade', 'overview'] })
      queryClient.invalidateQueries({ queryKey: channelsQueryKeys.all })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  // 按当前顺序重新落位：归属只在首次落位时按编排选、活跃用户不过期，改顺序后已有归属不跟随，
  // 清掉该组归属让下一条请求按当前顺序重新落位（每个活跃用户重建一次上游缓存）
  const resetAffinityMutation = useMutation({
    mutationFn: resetCascadeGroupAffinity,
    onSuccess: (res) => {
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      toast.success(
        t('Cleared channel bindings for {{count}} users', {
          count: res.data?.deleted ?? 0,
        })
      )
      setResetAffinityOpen(false)
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  // 页面上拖动过但还没保存：重新落位按已保存的顺序走，确认弹窗里提示
  const orderUnsaved =
    ids.join(',') !== group.channels.map((channel) => channel.id).join(',')

  // 只挂着这一个分组的渠道摘完会失去全部 ability，后端会跳过——提前在确认弹窗里点名
  const soleGroupChannels = group.channels.filter((channel) => {
    const full = fullChannelsById.get(channel.id)
    if (!full?.group) return false
    return full.group
      .split(',')
      .map((name) => name.trim())
      .filter(Boolean)
      .every((name) => name === group.name)
  })

  const handleDragEnter = (targetIndex: number) => {
    const from = dragIndexRef.current
    if (from === null || from === targetIndex) return
    const next = [...ids]
    const [moved] = next.splice(from, 1)
    next.splice(targetIndex, 0, moved)
    dragIndexRef.current = targetIndex
    onReorder(next)
  }

  return (
    <div className='rounded-xl border p-4'>
      <div className='mb-3 flex flex-wrap items-center gap-2'>
        <span className='font-medium'>{group.name}</span>
        <Badge variant='secondary'>
          {t('{{count}} channels', { count: group.channels.length })}
        </Badge>
        {group.orphan && (
          <>
            <Tooltip>
              <TooltipTrigger render={<Badge variant='destructive' />}>
                {t('Deactivated')}
              </TooltipTrigger>
              <TooltipContent className='max-w-xs'>
                {t(
                  'This group is no longer in the group ratio settings, so users cannot select it. It still shows here because the channels below still carry the group name.'
                )}
              </TooltipContent>
            </Tooltip>
            <Button
              variant='ghost'
              size='sm'
              className='h-6 px-2 text-xs'
              onClick={() => setPurgeOpen(true)}
            >
              <Trash2 className='size-3' />
              {t('Clean up group')}
            </Button>
          </>
        )}
        {!group.orphan && (
          <Button
            variant='ghost'
            size='sm'
            className='ml-auto h-6 px-2 text-xs'
            onClick={() => setResetAffinityOpen(true)}
          >
            <RotateCcw className='size-3' />
            {t('Re-place by current order')}
          </Button>
        )}
      </div>
      <div className='flex items-stretch gap-2 overflow-x-auto pb-2'>
        {ids.map((id, index) => {
          const channel = channelsById.get(id)
          if (!channel) return null
          return (
            <Fragment key={id}>
              {index > 0 && (
                <div className='text-muted-foreground flex shrink-0 flex-col items-center justify-center px-1'>
                  <span className='text-[10px] leading-none'>{t('Spill')}</span>
                  <ArrowRight className='size-4' />
                </div>
              )}
              <ChannelCard
                channel={channel}
                group={group.name}
                fullChannel={fullChannelsById.get(id)}
                index={index}
                recoveryTarget={recoveryTarget}
                isDragging={draggingId === id}
                onDragStart={() => {
                  setDraggingId(id)
                  dragIndexRef.current = index
                }}
                onDragEnter={() => handleDragEnter(index)}
                onDragEnd={() => {
                  setDraggingId(null)
                  dragIndexRef.current = null
                }}
                onRestore={(channelId) => restoreMutation.mutate(channelId)}
                restorePending={restoreMutation.isPending}
              />
            </Fragment>
          )
        })}
      </div>

      <AlertDialog open={resetAffinityOpen} onOpenChange={setResetAffinityOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('Re-place users in "{{name}}" by current order?', {
                name: group.name,
              })}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                "Users keep the channel they were first placed on, so changing the order does not move them. This clears every user binding in this group; each user's next request is placed by the current order. Every active user rebuilds the upstream prompt cache once (billed as cache writes)."
              )}
              {orderUnsaved && (
                <span className='text-destructive mt-2 block'>
                  {t(
                    'The order on this page is not saved yet. Users will be placed by the saved order.'
                  )}
                </span>
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={resetAffinityMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={resetAffinityMutation.isPending}
              onClick={(event) => {
                event.preventDefault()
                resetAffinityMutation.mutate(group.name)
              }}
            >
              {t('Re-place by current order')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={purgeOpen} onOpenChange={setPurgeOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('Clean up group "{{name}}"?', { name: group.name })}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'The group name will be removed from {{count}} channels and their abilities rebuilt. This cannot be undone.',
                { count: group.channels.length }
              )}
              {soleGroupChannels.length > 0 && (
                <span className='text-destructive mt-2 block'>
                  {t(
                    '{{count}} channels will be skipped because this is their only group: {{names}}',
                    {
                      count: soleGroupChannels.length,
                      names: soleGroupChannels
                        .map((channel) => `#${channel.id} ${channel.name}`)
                        .join(', '),
                    }
                  )}
                </span>
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={purgeMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              disabled={purgeMutation.isPending}
              onClick={(event) => {
                event.preventDefault()
                purgeMutation.mutate(group.name)
              }}
            >
              {t('Clean up group')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
