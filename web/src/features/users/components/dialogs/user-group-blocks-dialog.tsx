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
import { Ban, Loader2 } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTimestampForInput } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  deleteUserGroupBlocks,
  getUserGroupBlocks,
  updateUserGroupBlocks,
} from '../../api'
import {
  DEFAULT_GROUP_BLOCK_DURATION,
  formatGroupBlockExpiry,
  formatGroupBlockRemaining,
  resolveGroupBlockExpiresAt,
  type GroupBlockDuration,
} from '../../lib'
import type {
  User,
  UserGroupBlock,
  UserGroupBlocksDetail,
  UserGroupBlocksPayload,
} from '../../types'

type GroupBlockUser = Pick<User, 'id' | 'username' | 'display_name'>

interface UserGroupBlocksDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: GroupBlockUser | null
  onSuccess?: () => void
}

type GroupRow = {
  name: string
  desc: string
  blocked: boolean
  /** Active block loaded from the server, if any */
  existing?: UserGroupBlock
  duration: GroupBlockDuration
  customInput: string
}

function buildRows(detail: UserGroupBlocksDetail | null): GroupRow[] {
  if (!detail) return []
  const blocks = new Map(detail.blocks.map((block) => [block.group, block]))
  return detail.groups.map((group) => {
    const existing = blocks.get(group.name)
    return {
      name: group.name,
      desc: group.desc,
      blocked: !!existing,
      existing,
      duration: existing ? 'keep' : DEFAULT_GROUP_BLOCK_DURATION,
      customInput: '',
    }
  })
}

export function UserGroupBlocksDialog(props: UserGroupBlocksDialogProps) {
  const { t, i18n } = useTranslation()
  const [detail, setDetail] = useState<UserGroupBlocksDetail | null>(null)
  const [rows, setRows] = useState<GroupRow[]>([])
  const [note, setNote] = useState('')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [clearing, setClearing] = useState(false)

  const userId = props.user?.id

  const fetchDetail = useCallback(async () => {
    if (userId == null) return
    setLoading(true)
    try {
      const result = await getUserGroupBlocks(userId)
      if (result.success && result.data) {
        setDetail(result.data)
        setRows(buildRows(result.data))
      } else {
        toast.error(result.message || t('Failed to load group restrictions'))
      }
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to load group restrictions')
      )
    } finally {
      setLoading(false)
    }
  }, [userId, t])

  useEffect(() => {
    if (!props.open || userId == null) {
      setDetail(null)
      setRows([])
      setSaving(false)
      setClearing(false)
      return
    }
    setNote('')
    void fetchDetail()
  }, [props.open, userId, fetchDetail])

  const updateRow = (name: string, patch: Partial<GroupRow>) => {
    setRows((current) =>
      current.map((row) => (row.name === name ? { ...row, ...patch } : row))
    )
  }

  const toggleRow = (row: GroupRow, blocked: boolean) => {
    updateRow(row.name, {
      blocked,
      duration: row.existing ? 'keep' : DEFAULT_GROUP_BLOCK_DURATION,
      customInput: '',
    })
  }

  const changeDuration = (row: GroupRow, duration: GroupBlockDuration) => {
    updateRow(row.name, {
      duration,
      // Prefill custom input one day ahead so the picker is never empty.
      customInput:
        duration === 'custom' && !row.customInput
          ? formatTimestampForInput(Math.floor(Date.now() / 1000) + 86400)
          : row.customInput,
    })
  }

  const describeExpiry = (row: GroupRow): string => {
    const expiresAt = resolveGroupBlockExpiresAt(row.duration, {
      nowSec: Math.floor(Date.now() / 1000),
      keepExpiresAt: row.existing?.expires_at,
      customInput: row.customInput,
    })
    if (expiresAt == null) return t('Invalid time')
    if (expiresAt === 0) return t('Permanent')
    return `${formatGroupBlockExpiry(expiresAt)} (${formatGroupBlockRemaining(
      expiresAt,
      i18n.language
    )})`
  }

  const buildPayload = (): UserGroupBlocksPayload | null => {
    const nowSec = Math.floor(Date.now() / 1000)
    const trimmedNote = note.trim()
    const blocks: UserGroupBlocksPayload['blocks'] = []
    for (const row of rows) {
      if (!row.blocked) continue
      const expiresAt = resolveGroupBlockExpiresAt(row.duration, {
        nowSec,
        keepExpiresAt: row.existing?.expires_at,
        customInput: row.customInput,
      })
      if (expiresAt == null) {
        toast.error(
          t('Invalid restriction time for {{group}}', { group: row.name })
        )
        return null
      }
      // Unchanged blocks keep their original note; new / changed ones take this round's note.
      const blockNote =
        row.duration === 'keep' ? (row.existing?.note ?? '') : trimmedNote
      blocks.push({
        group: row.name,
        expires_at: expiresAt,
        ...(blockNote ? { note: blockNote } : {}),
      })
    }
    return { blocks }
  }

  const handleSave = async () => {
    if (!props.user) return
    const payload = buildPayload()
    if (!payload) return

    setSaving(true)
    try {
      const result = await updateUserGroupBlocks(props.user.id, payload)
      if (result.success) {
        toast.success(t('Group restrictions saved'))
        props.onSuccess?.()
        props.onOpenChange(false)
      } else {
        toast.error(result.message || t('Failed to save group restrictions'))
      }
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to save group restrictions')
      )
    } finally {
      setSaving(false)
    }
  }

  const handleClear = async () => {
    if (!props.user) return

    setClearing(true)
    try {
      const result = await deleteUserGroupBlocks(props.user.id)
      if (result.success) {
        toast.success(t('All group restrictions lifted'))
        props.onSuccess?.()
        props.onOpenChange(false)
      } else {
        toast.error(result.message || t('Failed to lift group restrictions'))
      }
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to lift group restrictions')
      )
    } finally {
      setClearing(false)
    }
  }

  const busy = loading || saving || clearing
  const blockedCount = rows.filter((row) => row.blocked).length
  const hasExisting = (detail?.blocks.length ?? 0) > 0

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={
        <>
          <Ban className='h-5 w-5' />
          {t('Group Restrictions')}
        </>
      }
      description={t(
        'Temporarily stop this user from using specific groups. Restrictions lift automatically when they expire.'
      )}
      contentClassName='sm:max-w-3xl'
      titleClassName='flex items-center gap-2'
      contentHeight='min(72vh, 720px)'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={busy}
          >
            {t('Cancel')}
          </Button>
          {hasExisting && (
            <Button
              variant='destructive'
              onClick={handleClear}
              disabled={busy || !props.user}
            >
              {clearing ? t('Processing...') : t('Lift All')}
            </Button>
          )}
          <Button onClick={handleSave} disabled={busy || !props.user}>
            {saving ? t('Saving...') : t('Save')}
          </Button>
        </>
      }
    >
      {loading ? (
        <div className='flex items-center justify-center py-10'>
          <Loader2 className='text-muted-foreground h-6 w-6 animate-spin' />
        </div>
      ) : (
        <div className='space-y-4'>
          <div className='border-border bg-muted/30 rounded-lg border p-3'>
            <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
              <div className='min-w-0'>
                <p className='text-sm font-medium'>
                  {props.user?.display_name || props.user?.username || '-'}
                </p>
                <p className='text-muted-foreground text-xs'>
                  {props.user
                    ? `${props.user.username} · ID ${props.user.id}`
                    : '-'}
                </p>
              </div>
              <StatusBadge
                label={`${blockedCount}/${rows.length}`}
                variant={blockedCount > 0 ? 'danger' : 'neutral'}
                copyable={false}
              />
            </div>
            <Separator className='my-3' />
            <p className='text-muted-foreground text-xs'>
              {t(
                'Requests to a restricted group are rejected with a notice that includes the expected recovery time. The auto group skips restricted groups; other groups are unaffected.'
              )}
            </p>
          </div>

          <div className='space-y-2'>
            <label htmlFor='group-block-note' className='text-sm font-medium'>
              {t('Internal note')}
            </label>
            <Input
              id='group-block-note'
              value={note}
              onChange={(event) => setNote(event.target.value)}
              disabled={busy}
              maxLength={255}
              placeholder={t('e.g. suspected distillation')}
            />
            <p className='text-muted-foreground text-xs'>
              {t(
                'Visible to admins only. Applied to groups newly restricted or changed in this save.'
              )}
            </p>
          </div>

          <div className='border-border overflow-hidden rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Group')}</TableHead>
                  <TableHead className='w-[90px]'>{t('Restrict')}</TableHead>
                  <TableHead className='w-[190px]'>
                    {t('Restriction Duration')}
                  </TableHead>
                  <TableHead>{t('Expires At')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={4}
                      className='text-muted-foreground py-8 text-center'
                    >
                      {t('No groups available')}
                    </TableCell>
                  </TableRow>
                ) : (
                  rows.map((row) => (
                    <TableRow
                      key={row.name}
                      className={cn(!row.blocked && 'bg-muted/20')}
                    >
                      <TableCell>
                        <div className='font-medium'>{row.name}</div>
                        {row.existing?.note ? (
                          <div className='text-muted-foreground max-w-[220px] truncate text-xs'>
                            {row.existing.note}
                          </div>
                        ) : null}
                      </TableCell>
                      <TableCell>
                        <Switch
                          checked={row.blocked}
                          onCheckedChange={(checked) =>
                            toggleRow(row, !!checked)
                          }
                          disabled={busy}
                        />
                      </TableCell>
                      <TableCell>
                        <div className='space-y-1.5'>
                          <NativeSelect
                            size='sm'
                            className='w-full'
                            value={row.duration}
                            disabled={!row.blocked || busy}
                            onChange={(event) =>
                              changeDuration(
                                row,
                                event.target.value as GroupBlockDuration
                              )
                            }
                          >
                            {row.existing && (
                              <NativeSelectOption value='keep'>
                                {t('Keep current')}
                              </NativeSelectOption>
                            )}
                            <NativeSelectOption value='1h'>
                              {t('1 hour')}
                            </NativeSelectOption>
                            <NativeSelectOption value='6h'>
                              {t('6 hours')}
                            </NativeSelectOption>
                            <NativeSelectOption value='24h'>
                              {t('24 hours')}
                            </NativeSelectOption>
                            <NativeSelectOption value='3d'>
                              {t('3 days')}
                            </NativeSelectOption>
                            <NativeSelectOption value='7d'>
                              {t('7 days')}
                            </NativeSelectOption>
                            <NativeSelectOption value='permanent'>
                              {t('Permanent')}
                            </NativeSelectOption>
                            <NativeSelectOption value='custom'>
                              {t('Custom')}
                            </NativeSelectOption>
                          </NativeSelect>
                          {row.blocked && row.duration === 'custom' && (
                            <Input
                              type='datetime-local'
                              className='h-7'
                              value={row.customInput}
                              disabled={busy}
                              onChange={(event) =>
                                updateRow(row.name, {
                                  customInput: event.target.value,
                                })
                              }
                            />
                          )}
                        </div>
                      </TableCell>
                      <TableCell className='text-muted-foreground text-xs tabular-nums'>
                        {row.blocked ? describeExpiry(row) : '-'}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        </div>
      )}
    </Dialog>
  )
}
