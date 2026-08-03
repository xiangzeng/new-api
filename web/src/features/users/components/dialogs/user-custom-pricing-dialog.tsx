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
import { Loader2, SlidersHorizontal } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
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
import { getFlowQuotaDates } from '@/features/dashboard/api'
import {
  aggregateFlowGroupUsage,
  type FlowGroupUsageRow,
} from '@/features/dashboard/lib'
import { cn } from '@/lib/utils'

import {
  deleteUserCustomPricing,
  getUserCustomPricing,
  updateUserCustomPricing,
} from '../../api'
import type {
  User,
  UserCustomPricingDetail,
  UserCustomPricingPayload,
} from '../../types'
import { CustomPricingUsageSection } from './custom-pricing-usage-section'
import { CustomPricingVisibilitySection } from './custom-pricing-visibility-section'

/** Lookback for ranking which groups the user actually consumes. */
const USAGE_LOOKBACK_SECONDS = 7 * 24 * 60 * 60

type CustomPricingUser = Pick<User, 'id' | 'username' | 'display_name'>

interface UserCustomPricingDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: CustomPricingUser | null
  onSuccess?: () => void
}

type GroupRow = {
  name: string
  defaultRatio: number
  configured: boolean
  ratio: number | null
}

function buildGroupRows(detail: UserCustomPricingDetail | null): GroupRow[] {
  if (!detail) return []
  return Object.entries(detail.groups)
    .map(([name, info]) => ({
      name,
      defaultRatio: info.default_ratio,
      configured: info.configured,
      ratio: info.ratio,
    }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

export function UserCustomPricingDialog(props: UserCustomPricingDialogProps) {
  const { t } = useTranslation()
  const [detail, setDetail] = useState<UserCustomPricingDetail | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [disabling, setDisabling] = useState(false)
  const [usageGroups, setUsageGroups] = useState<FlowGroupUsageRow[]>([])
  const [usageLoading, setUsageLoading] = useState(false)
  const [usageExpanded, setUsageExpanded] = useState(false)

  const userId = props.user?.id
  const username = props.user?.username

  const fetchDetail = useCallback(async () => {
    if (userId == null) return
    setLoading(true)
    try {
      const result = await getUserCustomPricing(userId)
      if (result.success && result.data) {
        setDetail(result.data)
      } else {
        toast.error(result.message || t('Failed to load custom pricing'))
      }
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to load custom pricing')
      )
    } finally {
      setLoading(false)
    }
  }, [userId, t])

  const fetchUsage = useCallback(async () => {
    if (!username) {
      setUsageGroups([])
      return
    }
    setUsageLoading(true)
    try {
      const end = Math.floor(Date.now() / 1000)
      const start = end - USAGE_LOOKBACK_SECONDS
      const result = await getFlowQuotaDates(
        {
          start_timestamp: start,
          end_timestamp: end,
          username,
        },
        true
      )
      if (result.success) {
        setUsageGroups(aggregateFlowGroupUsage(result.data).groups)
      } else {
        setUsageGroups([])
      }
    } catch {
      setUsageGroups([])
    } finally {
      setUsageLoading(false)
    }
  }, [username])

  useEffect(() => {
    if (!props.open || userId == null) {
      setDetail(null)
      setSaving(false)
      setDisabling(false)
      setUsageGroups([])
      setUsageExpanded(false)
      return
    }
    setUsageExpanded(false)
    void fetchDetail()
    void fetchUsage()
  }, [props.open, userId, fetchDetail, fetchUsage])

  const rows = useMemo(() => buildGroupRows(detail), [detail])
  const configuredCount = rows.filter((row) => row.configured).length

  const allGroupNames = useMemo(() => {
    if (!detail) return []
    const names = detail.all_groups
      ? Object.keys(detail.all_groups)
      : Object.keys(detail.groups)
    return names.sort((a, b) => a.localeCompare(b))
  }, [detail])

  const extraGroups = detail?.extra_groups ?? {}
  const hideGroups = detail?.hide_groups ?? []

  const updateGroup = (
    groupName: string,
    updater: (
      row: UserCustomPricingDetail['groups'][string]
    ) => UserCustomPricingDetail['groups'][string]
  ) => {
    setDetail((current) => {
      if (!current) return current
      const group = current.groups[groupName]
      if (!group) return current
      return {
        ...current,
        enabled: true,
        groups: {
          ...current.groups,
          [groupName]: updater(group),
        },
      }
    })
  }

  const toggleGroup = (groupName: string, configured: boolean) => {
    updateGroup(groupName, (group) => ({
      ...group,
      configured,
      ratio: configured ? (group.ratio ?? group.default_ratio) : null,
    }))
  }

  const updateRatio = (groupName: string, value: string) => {
    const ratio = value.trim() === '' ? null : Number(value)
    updateGroup(groupName, (group) => ({
      ...group,
      configured: true,
      ratio,
    }))
  }

  const applyDefaultRatios = () => {
    setDetail((current) => {
      if (!current) return current
      const groups: UserCustomPricingDetail['groups'] = {}
      for (const [name, group] of Object.entries(current.groups)) {
        groups[name] = {
          ...group,
          configured: true,
          ratio: group.ratio ?? group.default_ratio,
        }
      }
      return { ...current, enabled: true, groups }
    })
  }

  const buildPayload = (): UserCustomPricingPayload | null => {
    if (!detail) return null
    const groups: UserCustomPricingPayload['groups'] = {}
    for (const [name, group] of Object.entries(detail.groups)) {
      if (!group.configured) continue
      if (group.ratio == null) {
        toast.error(t('Invalid ratio for {{group}}', { group: name }))
        return null
      }
      const ratio = Number(group.ratio)
      if (!Number.isFinite(ratio) || ratio < 0) {
        toast.error(t('Invalid ratio for {{group}}', { group: name }))
        return null
      }
      groups[name] = { ratio }
    }
    return {
      enabled: true,
      groups,
      ...(Object.keys(extraGroups).length > 0
        ? { extra_groups: extraGroups }
        : {}),
      ...(hideGroups.length > 0 ? { hide_groups: hideGroups } : {}),
    }
  }

  const handleSave = async () => {
    if (!props.user) return
    const payload = buildPayload()
    if (!payload) return

    setSaving(true)
    try {
      const result = await updateUserCustomPricing(props.user.id, payload)
      if (result.success) {
        toast.success(t('Custom pricing saved'))
        props.onSuccess?.()
        props.onOpenChange(false)
      } else {
        toast.error(result.message || t('Failed to save custom pricing'))
      }
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to save custom pricing')
      )
    } finally {
      setSaving(false)
    }
  }

  const handleDisable = async () => {
    if (!props.user) return

    setDisabling(true)
    try {
      const result = await deleteUserCustomPricing(props.user.id)
      if (result.success) {
        toast.success(t('Custom pricing disabled'))
        props.onSuccess?.()
        props.onOpenChange(false)
      } else {
        toast.error(result.message || t('Failed to disable custom pricing'))
      }
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to disable custom pricing')
      )
    } finally {
      setDisabling(false)
    }
  }

  const busy = loading || saving || disabling

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={
        <>
          <SlidersHorizontal className='h-5 w-5' />
          {t('Custom Pricing')}
        </>
      }
      description={t('Set per-user group pricing ratios')}
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
          <Button
            variant='destructive'
            onClick={handleDisable}
            disabled={busy || !props.user}
          >
            {disabling ? t('Processing...') : t('Disable')}
          </Button>
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
              <div className='flex items-center gap-2'>
                <StatusBadge
                  label={`${configuredCount}/${rows.length}`}
                  variant={configuredCount > 0 ? 'success' : 'neutral'}
                  copyable={false}
                />
                <Button
                  variant='outline'
                  size='sm'
                  onClick={applyDefaultRatios}
                  disabled={!detail || busy}
                >
                  {t('Apply Defaults')}
                </Button>
              </div>
            </div>
            <Separator className='my-3' />
            <p className='text-muted-foreground text-xs'>
              {t(
                'Configured groups use this user-specific ratio. Unconfigured groups fall back to the system default ratio.'
              )}
            </p>
          </div>

          <CustomPricingUsageSection
            groups={usageGroups}
            loading={usageLoading}
            expanded={usageExpanded}
            onExpandedChange={setUsageExpanded}
          />

          <div className='border-border overflow-hidden rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Group')}</TableHead>
                  <TableHead>{t('Default Ratio')}</TableHead>
                  <TableHead>{t('Configured')}</TableHead>
                  <TableHead className='w-[160px]'>
                    {t('Custom Ratio')}
                  </TableHead>
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
                      className={cn(!row.configured && 'bg-muted/20')}
                    >
                      <TableCell className='font-medium'>{row.name}</TableCell>
                      <TableCell>{row.defaultRatio}</TableCell>
                      <TableCell>
                        <Switch
                          checked={row.configured}
                          onCheckedChange={(checked) =>
                            toggleGroup(row.name, !!checked)
                          }
                          disabled={busy}
                        />
                      </TableCell>
                      <TableCell>
                        <Input
                          type='number'
                          min={0}
                          step={0.1}
                          disabled={!row.configured || busy}
                          value={
                            row.configured && row.ratio != null
                              ? String(row.ratio)
                              : ''
                          }
                          onChange={(event) =>
                            updateRatio(row.name, event.target.value)
                          }
                        />
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>

          <CustomPricingVisibilitySection
            allGroups={allGroupNames}
            extraGroups={extraGroups}
            hideGroups={hideGroups}
            onExtraGroupsChange={(next) =>
              setDetail((current) =>
                current ? { ...current, extra_groups: next } : current
              )
            }
            onHideGroupsChange={(next) =>
              setDetail((current) =>
                current ? { ...current, hide_groups: next } : current
              )
            }
            disabled={busy}
          />
        </div>
      )}
    </Dialog>
  )
}
