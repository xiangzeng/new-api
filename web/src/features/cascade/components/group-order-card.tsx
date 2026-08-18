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
import { GripVertical } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { cn } from '@/lib/utils'

import type { CascadeGroup } from '../types'

type GroupOrderCardProps = {
  /** 在役分组，按当前展示顺序（含未保存的本地调整） */
  groups: CascadeGroup[]
  /** 是否存在失效分组（这类分组永远沉底，不进本表） */
  hasOrphan: boolean
  onReorder: (names: string[]) => void
}

// 分组泳道展示顺序：一行一个分组，拖左侧手柄改上下顺序。
// 只影响本页展示，不动组内渠道的级联溢出顺序。
export function GroupOrderCard({
  groups,
  hasOrphan,
  onReorder,
}: GroupOrderCardProps) {
  const { t } = useTranslation()
  const [draggingName, setDraggingName] = useState<string | null>(null)
  const dragIndexRef = useRef<number | null>(null)

  if (groups.length < 2) return null

  const handleDragEnter = (targetIndex: number) => {
    const from = dragIndexRef.current
    if (from === null || from === targetIndex) return
    const next = groups.map((group) => group.name)
    const [moved] = next.splice(from, 1)
    next.splice(targetIndex, 0, moved)
    dragIndexRef.current = targetIndex
    onReorder(next)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Group display order')}</CardTitle>
        <CardDescription>
          {t(
            'Drag a row to reorder how group lanes are listed on this page. Display only — it does not change the cascade order inside a group.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className='divide-y rounded-lg border'>
          {groups.map((group, index) => (
            <div
              key={group.name}
              draggable
              onDragStart={() => {
                setDraggingName(group.name)
                dragIndexRef.current = index
              }}
              onDragEnter={() => handleDragEnter(index)}
              onDragOver={(e) => e.preventDefault()}
              onDragEnd={() => {
                setDraggingName(null)
                dragIndexRef.current = null
              }}
              className={cn(
                'hover:bg-accent/40 flex cursor-grab items-center gap-3 px-3 py-2.5 transition-opacity',
                draggingName === group.name && 'opacity-40'
              )}
            >
              <GripVertical className='text-muted-foreground size-4 shrink-0' />
              <span className='text-muted-foreground w-5 shrink-0 text-xs tabular-nums'>
                {index + 1}
              </span>
              <span className='min-w-0 flex-1 truncate text-sm font-medium'>
                {group.name}
              </span>
              <Badge variant='secondary' className='shrink-0'>
                {t('{{count}} channels', { count: group.channels.length })}
              </Badge>
            </div>
          ))}
        </div>
        {hasOrphan && (
          <p className='text-muted-foreground mt-2 text-xs'>
            {t(
              'Deactivated groups always sink to the bottom, so they are not listed here.'
            )}
          </p>
        )}
      </CardContent>
    </Card>
  )
}
